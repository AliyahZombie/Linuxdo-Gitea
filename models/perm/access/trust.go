// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package access

import (
	repo_model "code.gitea.io/gitea/models/repo"
	user_model "code.gitea.io/gitea/models/user"
)

func viewerDiscourseTrustLevel(user *user_model.User) int {
	if user == nil || user.ID <= 0 {
		return 0
	}
	trustLevel := user.DiscourseTrustLevel
	if trustLevel < 0 {
		return 0
	}
	if trustLevel > 4 {
		return 4
	}
	return trustLevel
}

func repoRequiredTrustLevel(repo *repo_model.Repository) int {
	if repo == nil {
		return 0
	}
	minTrustLevel := repo.MinTrustLevel
	if minTrustLevel < 0 {
		return 0
	}
	if minTrustLevel > 4 {
		return 4
	}
	return minTrustLevel
}

func trustSatisfied(repo *repo_model.Repository, user *user_model.User) bool {
	minTrustLevel := repoRequiredTrustLevel(repo)
	if minTrustLevel <= 0 {
		return true
	}
	return viewerDiscourseTrustLevel(user) >= minTrustLevel
}
