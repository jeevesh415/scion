// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build !no_sqlite

package hub

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/ptone/scion-agent/pkg/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHubNativeGrovePath(t *testing.T) {
	path, err := hubNativeGrovePath("my-test-grove")
	require.NoError(t, err)

	homeDir, err := os.UserHomeDir()
	require.NoError(t, err)

	expected := filepath.Join(homeDir, ".scion", "groves", "my-test-grove")
	assert.Equal(t, expected, path)
}

func TestCreateGrove_HubNative_NoGitRemote(t *testing.T) {
	srv, _ := testServer(t)

	body := CreateGroveRequest{
		Name: "Hub Native Grove",
	}

	rec := doRequest(t, srv, http.MethodPost, "/api/v1/groves", body)
	require.Equal(t, http.StatusCreated, rec.Code, "body: %s", rec.Body.String())

	var grove store.Grove
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&grove))

	assert.Equal(t, "Hub Native Grove", grove.Name)
	assert.Equal(t, "hub-native-grove", grove.Slug)
	assert.Empty(t, grove.GitRemote, "hub-native grove should have no git remote")

	// Verify the filesystem was initialized
	workspacePath, err := hubNativeGrovePath(grove.Slug)
	require.NoError(t, err)

	scionDir := filepath.Join(workspacePath, ".scion")
	settingsPath := filepath.Join(scionDir, "settings.yaml")

	_, err = os.Stat(settingsPath)
	assert.NoError(t, err, "settings.yaml should exist for hub-native grove")

	// Cleanup
	t.Cleanup(func() {
		os.RemoveAll(workspacePath)
	})
}

func TestCreateGrove_GitBacked_NoFilesystemInit(t *testing.T) {
	srv, _ := testServer(t)

	body := CreateGroveRequest{
		Name:      "Git Grove",
		GitRemote: "github.com/test/repo",
	}

	rec := doRequest(t, srv, http.MethodPost, "/api/v1/groves", body)
	require.Equal(t, http.StatusCreated, rec.Code, "body: %s", rec.Body.String())

	var grove store.Grove
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&grove))

	assert.Equal(t, "github.com/test/repo", grove.GitRemote)

	// Verify no filesystem was created for git-backed grove
	workspacePath, err := hubNativeGrovePath(grove.Slug)
	require.NoError(t, err)

	_, err = os.Stat(workspacePath)
	assert.True(t, os.IsNotExist(err), "no workspace directory should be created for git-backed groves")
}

func TestPopulateAgentConfig_HubNativeGrove_SetsWorkspace(t *testing.T) {
	srv, _ := testServer(t)

	grove := &store.Grove{
		ID:   "grove-hub-native",
		Name: "Hub Native",
		Slug: "hub-native",
		// No GitRemote — hub-native grove
	}

	agent := &store.Agent{
		ID:            "agent-test",
		AppliedConfig: &store.AgentAppliedConfig{},
	}

	srv.populateAgentConfig(agent, grove, nil)

	expectedPath, err := hubNativeGrovePath("hub-native")
	require.NoError(t, err)
	assert.Equal(t, expectedPath, agent.AppliedConfig.Workspace,
		"Workspace should be set for hub-native groves")
	assert.Nil(t, agent.AppliedConfig.GitClone,
		"GitClone should not be set for hub-native groves")
}

func TestPopulateAgentConfig_GitGrove_NoWorkspace(t *testing.T) {
	srv, _ := testServer(t)

	grove := &store.Grove{
		ID:        "grove-git",
		Name:      "Git Grove",
		Slug:      "git-grove",
		GitRemote: "github.com/test/repo",
	}

	agent := &store.Agent{
		ID:            "agent-test",
		AppliedConfig: &store.AgentAppliedConfig{},
	}

	srv.populateAgentConfig(agent, grove, nil)

	assert.Empty(t, agent.AppliedConfig.Workspace,
		"Workspace should not be set for git-backed groves")
	assert.NotNil(t, agent.AppliedConfig.GitClone,
		"GitClone should be set for git-backed groves")
}
