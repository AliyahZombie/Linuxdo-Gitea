// Copyright 2025 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package auth

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	asymkey_model "code.gitea.io/gitea/models/asymkey"
	"code.gitea.io/gitea/models/auth"
	user_model "code.gitea.io/gitea/models/user"
	"code.gitea.io/gitea/modules/log"
	"code.gitea.io/gitea/modules/timeutil"
	"code.gitea.io/gitea/modules/util"
	asymkey_service "code.gitea.io/gitea/services/asymkey"
	"code.gitea.io/gitea/services/auth/source/oauth2"
	"code.gitea.io/gitea/services/context"

	"github.com/markbates/goth"
)

func oauth2IsLinuxDoConnectOIDC(source *oauth2.Source) bool {
	if source == nil {
		return false
	}
	if source.Provider != "openidConnect" {
		return false
	}

	normalizeURL := func(s string) string {
		s = strings.TrimSpace(s)
		return strings.TrimRight(s, "/")
	}

	discoveryURL := normalizeURL(source.OpenIDConnectAutoDiscoveryURL)
	expectedURL := normalizeURL("https://connect.linux.do/.well-known/openid-configuration")

	return discoveryURL == expectedURL
}

func oauth2ParseDiscourseTrustLevel(raw any) (trustLevel int, ok bool) {
	clamp := func(v int) int {
		if v < 0 {
			return 0
		}
		if v > 4 {
			return 4
		}
		return v
	}

	clampInt64 := func(v int64) int {
		if v < 0 {
			return 0
		}
		if v > 4 {
			return 4
		}
		return int(v)
	}

	clampUint64 := func(v uint64) int {
		if v > 4 {
			return 4
		}
		return int(v)
	}

	parseFloat := func(v float64) (int, bool) {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return 0, false
		}
		if v < 0 {
			return 0, false
		}
		if v > 4 {
			return 4, true
		}
		if v != float64(int64(v)) {
			return 0, false
		}
		return int(v), true
	}

	switch v := raw.(type) {
	case nil:
		return 0, false
	case int:
		return clamp(v), true
	case int8:
		return clamp(int(v)), true
	case int16:
		return clamp(int(v)), true
	case int32:
		return clampInt64(int64(v)), true
	case int64:
		return clampInt64(v), true
	case uint:
		return clampUint64(uint64(v)), true
	case uint8:
		return clampUint64(uint64(v)), true
	case uint16:
		return clampUint64(uint64(v)), true
	case uint32:
		return clampUint64(uint64(v)), true
	case uint64:
		return clampUint64(v), true
	case float32:
		return parseFloat(float64(v))
	case float64:
		return parseFloat(v)
	case json.Number:
		if i64, err := v.Int64(); err == nil {
			return clampInt64(i64), true
		}
		if f64, err := v.Float64(); err == nil {
			return parseFloat(f64)
		}
		return 0, false
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			return 0, false
		}
		return clamp(parsed), true
	default:
		return 0, false
	}
}

func oauth2SignInSync(ctx *context.Context, authSourceID int64, u *user_model.User, gothUser goth.User) {
	oauth2UpdateAvatarIfNeed(ctx, gothUser.AvatarURL, u)

	authSource, err := auth.GetSourceByID(ctx, authSourceID)
	if err != nil {
		ctx.ServerError("GetSourceByID", err)
		return
	}
	oauth2Source, _ := authSource.Cfg.(*oauth2.Source)
	if !authSource.IsOAuth2() || oauth2Source == nil {
		ctx.ServerError("oauth2SignInSync", fmt.Errorf("source %s is not an OAuth2 source", gothUser.Provider))
		return
	}

	if oauth2IsLinuxDoConnectOIDC(oauth2Source) {
		raw, exists := gothUser.RawData["trust_level"]
		trustLevel, ok := oauth2ParseDiscourseTrustLevel(raw)
		if exists && !ok {
			log.Error("Unable to parse OAuth2 user discourse trust level %s: invalid trust_level claim type: %T", gothUser.Provider, raw)
		}
		updatedUnix := timeutil.TimeStampNow()

		u.DiscourseTrustLevel = trustLevel
		u.DiscourseTrustLevelUpdatedUnix = updatedUnix

		if err := user_model.UpdateUserCols(ctx, u, "discourse_trust_level", "discourse_trust_level_updated_unix"); err != nil {
			log.Error("Unable to sync OAuth2 user discourse trust level %s: %v", gothUser.Provider, err)
		}
	}

	// sync full name
	fullNameKey := util.IfZero(oauth2Source.FullNameClaimName, "name")
	fullName, _ := gothUser.RawData[fullNameKey].(string)
	fullName = util.IfZero(fullName, gothUser.Name)

	// need to update if the user has no full name set
	shouldUpdateFullName := u.FullName == ""
	// force to update if the attribute is set
	shouldUpdateFullName = shouldUpdateFullName || oauth2Source.FullNameClaimName != ""
	// only update if the full name is different
	shouldUpdateFullName = shouldUpdateFullName && u.FullName != fullName
	if shouldUpdateFullName {
		u.FullName = fullName
		if err := user_model.UpdateUserCols(ctx, u, "full_name"); err != nil {
			log.Error("Unable to sync OAuth2 user full name %s: %v", gothUser.Provider, err)
		}
	}

	err = oauth2UpdateSSHPubIfNeed(ctx, authSource, &gothUser, u)
	if err != nil {
		log.Error("Unable to sync OAuth2 SSH public key %s: %v", gothUser.Provider, err)
	}
}

func oauth2SyncGetSSHKeys(source *oauth2.Source, gothUser *goth.User) ([]string, error) {
	value, exists := gothUser.RawData[source.SSHPublicKeyClaimName]
	if !exists {
		return []string{}, nil
	}
	rawSlice, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("invalid SSH public key value type: %T", value)
	}

	sshKeys := make([]string, 0, len(rawSlice))
	for _, v := range rawSlice {
		str, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("invalid SSH public key value item type: %T", v)
		}
		sshKeys = append(sshKeys, str)
	}
	return sshKeys, nil
}

func oauth2UpdateSSHPubIfNeed(ctx *context.Context, authSource *auth.Source, gothUser *goth.User, user *user_model.User) error {
	oauth2Source, _ := authSource.Cfg.(*oauth2.Source)
	if oauth2Source == nil || oauth2Source.SSHPublicKeyClaimName == "" {
		return nil
	}
	sshKeys, err := oauth2SyncGetSSHKeys(oauth2Source, gothUser)
	if err != nil {
		return err
	}
	if !asymkey_model.SynchronizePublicKeys(ctx, user, authSource, sshKeys, false) {
		return nil
	}
	return asymkey_service.RewriteAllPublicKeys(ctx)
}
