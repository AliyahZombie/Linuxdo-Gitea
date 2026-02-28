// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package v1_26

import (
	"xorm.io/xorm"
)

func AddDiscourseTrustVisibility(x *xorm.Engine) error {
	type Repository struct {
		ID            int64 `xorm:"pk autoincr"`
		MinTrustLevel int   `xorm:"NOT NULL DEFAULT 0 INDEX"`
	}

	type User struct {
		ID                             int64 `xorm:"pk autoincr"`
		DiscourseTrustLevel            int   `xorm:"NOT NULL DEFAULT 0"`
		DiscourseTrustLevelUpdatedUnix int64 `xorm:"NOT NULL DEFAULT 0"`
	}

	_, err := x.SyncWithOptions(xorm.SyncOptions{
		IgnoreDropIndices: true,
		IgnoreConstrains:  true,
	}, new(Repository), new(User))
	return err
}
