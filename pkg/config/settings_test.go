package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ptone/scion-agent/pkg/api"
)

func TestLoadSettings(t *testing.T) {
	// Create temporary directories for global and grove settings
	tmpDir := t.TempDir()

	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)
	os.Setenv("HOME", tmpDir)

	groveDir := filepath.Join(tmpDir, "my-grove")
	groveScionDir := filepath.Join(groveDir, ".scion")
	if err := os.MkdirAll(groveScionDir, 0755); err != nil {
		t.Fatal(err)
	}

	// 1. Test defaults
	s, err := LoadSettings(groveScionDir)
	if err != nil {
		t.Fatalf("LoadSettings failed: %v", err)
	}
	if s.ActiveProfile != "local" {
		t.Errorf("expected active profile 'local', got '%s'", s.ActiveProfile)
	}
	if s.DefaultTemplate != "gemini" {
		t.Errorf("expected default template 'gemini', got '%s'", s.DefaultTemplate)
	}

	// 2. Test Global overrides
	globalSettings := `{
		"active_profile": "prod",
		"default_template": "claude",
		"runtimes": {
			"kubernetes": { "namespace": "scion-global" }
		},
		"profiles": {
			"prod": { "runtime": "kubernetes", "tmux": false }
		}
	}`
	globalScionDir := filepath.Join(tmpDir, ".scion")
	if err := os.MkdirAll(globalScionDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(globalScionDir, "settings.json"), []byte(globalSettings), 0644); err != nil {
		t.Fatal(err)
	}

	s, err = LoadSettings(groveScionDir)
	if err != nil {
		t.Fatalf("LoadSettings failed: %v", err)
	}
	if s.ActiveProfile != "prod" {
		t.Errorf("expected global override active_profile 'prod', got '%s'", s.ActiveProfile)
	}
	if s.DefaultTemplate != "claude" {
		t.Errorf("expected global override template 'claude', got '%s'", s.DefaultTemplate)
	}
	if s.Runtimes["kubernetes"].Namespace != "scion-global" {
		t.Errorf("expected global override runtime namespace 'scion-global', got '%s'", s.Runtimes["kubernetes"].Namespace)
	}

	// 3. Test Grove overrides
	groveSettings := `{
		"active_profile": "local-dev",
		"profiles": {
			"local-dev": { "runtime": "local", "tmux": true }
		}
	}`
	if err := os.WriteFile(filepath.Join(groveScionDir, "settings.json"), []byte(groveSettings), 0644); err != nil {
		t.Fatal(err)
	}

	s, err = LoadSettings(groveScionDir)
	if err != nil {
		t.Fatalf("LoadSettings failed: %v", err)
	}
	if s.ActiveProfile != "local-dev" {
		t.Errorf("expected grove override active_profile 'local-dev', got '%s'", s.ActiveProfile)
	}
	// Template should still be claude from global
	if s.DefaultTemplate != "claude" {
		t.Errorf("expected inherited global template 'claude', got '%s'", s.DefaultTemplate)
	}
}

func TestUpdateSetting(t *testing.T) {
	tmpDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)
	os.Setenv("HOME", tmpDir)

	groveDir := filepath.Join(tmpDir, "my-grove")
	groveScionDir := filepath.Join(groveDir, ".scion")
	if err := os.MkdirAll(groveScionDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Update local setting
	err := UpdateSetting(groveScionDir, "active_profile", "kubernetes", false)
	if err != nil {
		t.Fatalf("UpdateSetting failed: %v", err)
	}

	// Verify file content
	content, err := os.ReadFile(filepath.Join(groveScionDir, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), `"active_profile": "kubernetes"`) {
		t.Errorf("expected file to contain active_profile: kubernetes, got %s", string(content))
	}

	// Update default_template
	err = UpdateSetting(groveScionDir, "default_template", "my-template", false)
	if err != nil {
		t.Fatalf("UpdateSetting default_template failed: %v", err)
	}
	content, _ = os.ReadFile(filepath.Join(groveScionDir, "settings.json"))
	if !strings.Contains(string(content), `"default_template": "my-template"`) {
		t.Errorf("expected file to contain default_template: my-template, got %s", string(content))
	}
}

func TestResolve(t *testing.T) {
	s := &Settings{
		ActiveProfile: "local",
		Runtimes: map[string]RuntimeConfig{
			"docker": {Host: "unix:///var/run/docker.sock"},
		},
		Harnesses: map[string]HarnessConfig{
			"gemini": {Image: "gemini:latest", User: "root"},
		},
		Profiles: map[string]ProfileConfig{
			"local": {
				Runtime: "docker",
				HarnessOverrides: map[string]HarnessOverride{
					"gemini": {Image: "gemini:dev"},
				},
			},
		},
	}

	runtimeCfg, name, err := s.ResolveRuntime("")
	if err != nil {
		t.Fatal(err)
	}
	if name != "docker" {
		t.Errorf("expected runtime name docker, got %s", name)
	}
	if runtimeCfg.Host != "unix:///var/run/docker.sock" {
		t.Errorf("expected host unix:///var/run/docker.sock, got %s", runtimeCfg.Host)
	}

	h, err := s.ResolveHarness("", "gemini")
	if err != nil {
		t.Fatal(err)
	}
	if h.Image != "gemini:dev" {
		t.Errorf("expected image gemini:dev, got %s", h.Image)
	}
}

func TestEnvMerging(t *testing.T) {

		s := &Settings{

			Harnesses: map[string]HarnessConfig{

				"gemini": {

					Env: map[string]string{

						"H1": "V1",

						"H2": "V2",

					},

				},

			},

			Profiles: map[string]ProfileConfig{

				"dev": {

					Env: map[string]string{

						"H2": "P2", // Overrides harness

						"P1": "PV1",

					},

					HarnessOverrides: map[string]HarnessOverride{

						"gemini": {

							Env: map[string]string{

								"P1": "PH1", // Overrides profile

								"O1": "OV1",

							},

						},

					},

				},

			},

		}

	

		h, err := s.ResolveHarness("dev", "gemini")

		if err != nil {

			t.Fatal(err)

		}

	

		expected := map[string]string{

			"H1": "V1",  // From harness base

			"H2": "P2",  // Harness base, overridden by profile root

			"P1": "PH1", // Profile root, overridden by harness override

			"O1": "OV1", // From harness override

		}

	

		if len(h.Env) != len(expected) {

			t.Errorf("expected %d env vars, got %d", len(expected), len(h.Env))

		}

	

		for k, v := range expected {

			if h.Env[k] != v {

				t.Errorf("expected %s=%s, got %s", k, v, h.Env[k])

			}

		}

	}

	

	func TestMergeSettingsEnv(t *testing.T) {

		base := &Settings{

			Harnesses: map[string]HarnessConfig{

				"gemini": {

					Env: map[string]string{"A": "1", "B": "2"},

				},

			},

		}

		overrideJSON := `{

			"harnesses": {

				"gemini": {

					"env": {"B": "3", "C": "4"}

				}

			}

		}`

	

		err := MergeSettings(base, []byte(overrideJSON))

		if err != nil {

			t.Fatal(err)

		}

	

		env := base.Harnesses["gemini"].Env

		if env["A"] != "1" || env["B"] != "3" || env["C"] != "4" {
			t.Errorf("unexpected env after merge: %v", env)
		}
	}

func TestMergeSettingsAuthSelectedType(t *testing.T) {
	base := &Settings{
		Harnesses: map[string]HarnessConfig{
			"gemini": {
				AuthSelectedType: "gemini-api-key",
			},
		},
	}

	overrideJSON := `{
		"harnesses": {
			"gemini": {
				"auth_selectedType": "vertex-ai"
			}
		}
	}`

	err := MergeSettings(base, []byte(overrideJSON))
	if err != nil {
		t.Fatal(err)
	}

	if base.Harnesses["gemini"].AuthSelectedType != "vertex-ai" {
		t.Errorf("expected AuthSelectedType to be vertex-ai, got %s", base.Harnesses["gemini"].AuthSelectedType)
	}
}

func TestRuntimeEnvMerging(t *testing.T) {
	s := &Settings{
		Runtimes: map[string]RuntimeConfig{
			"docker": {
				Env: map[string]string{
					"R1": "V1",
					"R2": "V2",
				},
			},
		},
		Profiles: map[string]ProfileConfig{
			"dev": {
				Runtime: "docker",
				Env: map[string]string{
					"R2": "P2", // Overrides runtime
					"P1": "PV1",
				},
			},
		},
	}

	r, _, err := s.ResolveRuntime("dev")
	if err != nil {
		t.Fatal(err)
	}

	expected := map[string]string{
		"R1": "V1",
		"R2": "P2",
		"P1": "PV1",
	}

	if len(r.Env) != len(expected) {
		t.Errorf("expected %d env vars, got %d", len(expected), len(r.Env))
	}

	for k, v := range expected {
		if r.Env[k] != v {
			t.Errorf("expected %s=%s, got %s", k, v, r.Env[k])
		}
	}
}

func TestVolumeMerging(t *testing.T) {
	s := &Settings{
		Harnesses: map[string]HarnessConfig{
			"gemini": {
				Volumes: []api.VolumeMount{
					{Source: "/host/1", Target: "/container/1"},
				},
			},
		},
		Profiles: map[string]ProfileConfig{
			"dev": {
				Volumes: []api.VolumeMount{
					{Source: "/host/2", Target: "/container/2"},
				},
				HarnessOverrides: map[string]HarnessOverride{
					"gemini": {
						Volumes: []api.VolumeMount{
							{Source: "/host/3", Target: "/container/3"},
						},
					},
				},
			},
		},
	}

	h, err := s.ResolveHarness("dev", "gemini")
	if err != nil {
		t.Fatal(err)
	}

	if len(h.Volumes) != 3 {
		t.Errorf("expected 3 volumes, got %d", len(h.Volumes))
	}

	// Check for existence of all expected volumes
	found := make(map[string]bool)
	for _, v := range h.Volumes {
		found[v.Source] = true
	}

	if !found["/host/1"] || !found["/host/2"] || !found["/host/3"] {
		t.Errorf("missing expected volumes: got %v", h.Volumes)
	}
}

	