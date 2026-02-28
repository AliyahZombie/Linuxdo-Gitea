// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"context"
	"net/http"
	"testing"

	repo_model "code.gitea.io/gitea/models/repo"
	user_model "code.gitea.io/gitea/models/user"
	"code.gitea.io/gitea/modules/optional"
	api "code.gitea.io/gitea/modules/structs"
	"code.gitea.io/gitea/tests"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setUserTrustLevel(t testing.TB, ctx context.Context, userID int64, trustLevel int) {
	t.Helper()
	require.NoError(t, user_model.UpdateUserCols(ctx, &user_model.User{ID: userID, DiscourseTrustLevel: trustLevel}, "discourse_trust_level"))
}

func setRepoMinTrustLevel(t testing.TB, ctx context.Context, repoID int64, minTrustLevel int) {
	t.Helper()
	require.NoError(t, repo_model.UpdateRepositoryColsNoAutoTime(ctx, &repo_model.Repository{ID: repoID, MinTrustLevel: minTrustLevel}, "min_trust_level"))
}

func containsRepoByFullName(results api.SearchResults, fullName string) bool {
	for _, repo := range results.Data {
		if repo.FullName == fullName {
			return true
		}
	}
	return false
}

func createViewerUser(t testing.TB, ctx context.Context, name string) *user_model.User {
	t.Helper()

	user := &user_model.User{
		Name:   name,
		Email:  name + "@example.com",
		Passwd: userPassword,
	}
	meta := &user_model.Meta{InitialIP: "127.0.0.1", InitialUserAgent: "integration"}
	require.NoError(t, user_model.CreateUser(ctx, user, meta, &user_model.CreateUserOverwriteOptions{IsActive: optional.Some(true)}))
	return user
}

func TestTrustLevelVisibility_WebAndSearch(t *testing.T) {
	defer tests.PrepareTestEnv(t)()
	ctx := t.Context()

	repo, err := repo_model.GetRepositoryByOwnerAndName(ctx, "user2", "repo1")
	require.NoError(t, err)
	setRepoMinTrustLevel(t, ctx, repo.ID, 2)
	repo, err = repo_model.GetRepositoryByID(ctx, repo.ID)
	require.NoError(t, err)
	require.Equal(t, 2, repo.MinTrustLevel)
	t.Cleanup(func() {
		resetCtx := context.Background()
		setRepoMinTrustLevel(t, resetCtx, repo.ID, 0)
	})

	MakeRequest(t, NewRequest(t, "GET", "/user2/repo1"), http.StatusNotFound)

	exploreResp := MakeRequest(t, NewRequest(t, "GET", "/explore/repos?q=repo1"), http.StatusOK)
	assert.NotContains(t, exploreResp.Body.String(), "/user2/repo1")

	user := createViewerUser(t, ctx, "trustviewerweb")
	setUserTrustLevel(t, ctx, user.ID, 1)
	t.Cleanup(func() {
		resetCtx := context.Background()
		setUserTrustLevel(t, resetCtx, user.ID, 0)
	})
	loginUser(t, user.Name).MakeRequest(t, NewRequest(t, "GET", "/user2/repo1"), http.StatusNotFound)

	setUserTrustLevel(t, ctx, user.ID, 2)
	loginUser(t, user.Name).MakeRequest(t, NewRequest(t, "GET", "/user2/repo1"), http.StatusOK)
}

func TestTrustLevelVisibility_API(t *testing.T) {
	defer tests.PrepareTestEnv(t)()
	ctx := t.Context()

	repo, err := repo_model.GetRepositoryByOwnerAndName(ctx, "user2", "repo1")
	require.NoError(t, err)
	setRepoMinTrustLevel(t, ctx, repo.ID, 2)
	repo, err = repo_model.GetRepositoryByID(ctx, repo.ID)
	require.NoError(t, err)
	require.Equal(t, 2, repo.MinTrustLevel)
	t.Cleanup(func() {
		resetCtx := context.Background()
		setRepoMinTrustLevel(t, resetCtx, repo.ID, 0)
	})

	MakeRequest(t, NewRequest(t, "GET", "/api/v1/repos/user2/repo1"), http.StatusNotFound)

	resp := MakeRequest(t, NewRequest(t, "GET", "/api/v1/repos/search?q=repo1&private=false"), http.StatusOK)
	var results api.SearchResults
	DecodeJSON(t, resp, &results)
	assert.False(t, containsRepoByFullName(results, "user2/repo1"))

	user := createViewerUser(t, ctx, "trustviewerapi")
	setUserTrustLevel(t, ctx, user.ID, 1)
	t.Cleanup(func() {
		resetCtx := context.Background()
		setUserTrustLevel(t, resetCtx, user.ID, 0)
	})

	req := NewRequest(t, "GET", "/api/v1/repos/user2/repo1")
	req.SetBasicAuth(user.Name, userPassword)
	MakeRequest(t, req, http.StatusNotFound)

	req = NewRequest(t, "GET", "/api/v1/repos/search?q=repo1&private=false")
	req.SetBasicAuth(user.Name, userPassword)
	resp = MakeRequest(t, req, http.StatusOK)
	results = api.SearchResults{}
	DecodeJSON(t, resp, &results)
	assert.False(t, containsRepoByFullName(results, "user2/repo1"))

	setUserTrustLevel(t, ctx, user.ID, 2)
	req = NewRequest(t, "GET", "/api/v1/repos/user2/repo1")
	req.SetBasicAuth(user.Name, userPassword)
	resp = MakeRequest(t, req, http.StatusOK)
	var repoAPI api.Repository
	DecodeJSON(t, resp, &repoAPI)
	assert.Equal(t, "user2/repo1", repoAPI.FullName)
	assert.Equal(t, 2, repoAPI.MinTrustLevel)

	req = NewRequest(t, "GET", "/api/v1/repos/search?q=repo1&private=false")
	req.SetBasicAuth(user.Name, userPassword)
	resp = MakeRequest(t, req, http.StatusOK)
	results = api.SearchResults{}
	DecodeJSON(t, resp, &results)
	assert.True(t, containsRepoByFullName(results, "user2/repo1"))
}
