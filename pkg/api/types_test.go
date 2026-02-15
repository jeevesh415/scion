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

package api

import (
	"strings"
	"testing"
)

func TestVolumeMountValidate(t *testing.T) {
	tests := []struct {
		name    string
		vol     VolumeMount
		wantErr string
	}{
		{
			name: "valid local volume",
			vol: VolumeMount{
				Source: "/host/path",
				Target: "/container/path",
			},
			wantErr: "",
		},
		{
			name: "valid local volume with explicit type",
			vol: VolumeMount{
				Source: "/host/path",
				Target: "/container/path",
				Type:   "local",
			},
			wantErr: "",
		},
		{
			name: "valid gcs volume",
			vol: VolumeMount{
				Target: "/container/path",
				Type:   "gcs",
				Bucket: "my-bucket",
				Prefix: "some/prefix",
			},
			wantErr: "",
		},
		{
			name: "missing target",
			vol: VolumeMount{
				Source: "/host/path",
			},
			wantErr: "missing required field: target",
		},
		{
			name: "missing source for local volume",
			vol: VolumeMount{
				Target: "/container/path",
			},
			wantErr: "missing required field: source",
		},
		{
			name: "missing source for explicit local type",
			vol: VolumeMount{
				Target: "/container/path",
				Type:   "local",
			},
			wantErr: "missing required field: source",
		},
		{
			name: "invalid type",
			vol: VolumeMount{
				Source: "/host/path",
				Target: "/container/path",
				Type:   "nfs",
			},
			wantErr: "invalid type",
		},
		{
			name: "gcs without bucket",
			vol: VolumeMount{
				Target: "/container/path",
				Type:   "gcs",
			},
			wantErr: "missing required field: bucket",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.vol.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("Validate() unexpected error: %v", err)
				}
			} else {
				if err == nil {
					t.Errorf("Validate() expected error containing %q, got nil", tt.wantErr)
				} else if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("Validate() error = %q, want containing %q", err.Error(), tt.wantErr)
				}
			}
		})
	}
}

func TestValidateVolumes(t *testing.T) {
	t.Run("nil slice is valid", func(t *testing.T) {
		if err := ValidateVolumes(nil); err != nil {
			t.Errorf("ValidateVolumes(nil) unexpected error: %v", err)
		}
	})

	t.Run("empty slice is valid", func(t *testing.T) {
		if err := ValidateVolumes([]VolumeMount{}); err != nil {
			t.Errorf("ValidateVolumes([]) unexpected error: %v", err)
		}
	})

	t.Run("all valid volumes", func(t *testing.T) {
		vols := []VolumeMount{
			{Source: "/a", Target: "/b"},
			{Target: "/c", Type: "gcs", Bucket: "bkt"},
		}
		if err := ValidateVolumes(vols); err != nil {
			t.Errorf("ValidateVolumes() unexpected error: %v", err)
		}
	})

	t.Run("error includes index", func(t *testing.T) {
		vols := []VolumeMount{
			{Source: "/a", Target: "/b"},
			{Source: "/c"}, // missing target
		}
		err := ValidateVolumes(vols)
		if err == nil {
			t.Fatal("ValidateVolumes() expected error, got nil")
		}
		if !strings.Contains(err.Error(), "volumes[1]") {
			t.Errorf("ValidateVolumes() error = %q, want containing 'volumes[1]'", err.Error())
		}
	})
}

func TestValidateServices(t *testing.T) {
	tests := []struct {
		name     string
		services []ServiceSpec
		wantErr  string
	}{
		{
			name:     "nil slice is valid",
			services: nil,
			wantErr:  "",
		},
		{
			name:     "empty slice is valid",
			services: []ServiceSpec{},
			wantErr:  "",
		},
		{
			name: "valid service minimal",
			services: []ServiceSpec{
				{Name: "svc1", Command: []string{"sleep", "10"}},
			},
			wantErr: "",
		},
		{
			name: "valid service with all fields",
			services: []ServiceSpec{
				{
					Name:    "chrome-mcp",
					Command: []string{"npx", "@anthropic-ai/chrome-devtools-mcp@latest"},
					Restart: "on-failure",
					Env:     map[string]string{"CHROME_PATH": "/usr/bin/chromium"},
					ReadyCheck: &ReadyCheck{
						Type:    "tcp",
						Target:  "localhost:9222",
						Timeout: "30s",
					},
				},
			},
			wantErr: "",
		},
		{
			name: "valid multiple services",
			services: []ServiceSpec{
				{Name: "svc1", Command: []string{"cmd1"}, Restart: "always"},
				{Name: "svc2", Command: []string{"cmd2"}, Restart: "no"},
			},
			wantErr: "",
		},
		{
			name: "missing name",
			services: []ServiceSpec{
				{Command: []string{"cmd"}},
			},
			wantErr: "missing required field: name",
		},
		{
			name: "missing command",
			services: []ServiceSpec{
				{Name: "svc1"},
			},
			wantErr: "missing required field: command",
		},
		{
			name: "empty command slice",
			services: []ServiceSpec{
				{Name: "svc1", Command: []string{}},
			},
			wantErr: "missing required field: command",
		},
		{
			name: "invalid restart policy",
			services: []ServiceSpec{
				{Name: "svc1", Command: []string{"cmd"}, Restart: "never"},
			},
			wantErr: "invalid restart policy",
		},
		{
			name: "duplicate names",
			services: []ServiceSpec{
				{Name: "svc1", Command: []string{"cmd1"}},
				{Name: "svc1", Command: []string{"cmd2"}},
			},
			wantErr: "duplicate service name",
		},
		{
			name: "invalid ready_check type",
			services: []ServiceSpec{
				{
					Name:    "svc1",
					Command: []string{"cmd"},
					ReadyCheck: &ReadyCheck{
						Type:    "grpc",
						Target:  "localhost:50051",
						Timeout: "10s",
					},
				},
			},
			wantErr: "invalid ready_check type",
		},
		{
			name: "ready_check missing target",
			services: []ServiceSpec{
				{
					Name:    "svc1",
					Command: []string{"cmd"},
					ReadyCheck: &ReadyCheck{
						Type:    "tcp",
						Timeout: "10s",
					},
				},
			},
			wantErr: "ready_check missing required field: target",
		},
		{
			name: "ready_check missing timeout",
			services: []ServiceSpec{
				{
					Name:    "svc1",
					Command: []string{"cmd"},
					ReadyCheck: &ReadyCheck{
						Type:   "http",
						Target: "http://localhost:8080/health",
					},
				},
			},
			wantErr: "ready_check missing required field: timeout",
		},
		{
			name: "valid ready_check types",
			services: []ServiceSpec{
				{
					Name: "svc-tcp", Command: []string{"cmd"},
					ReadyCheck: &ReadyCheck{Type: "tcp", Target: "localhost:8080", Timeout: "5s"},
				},
				{
					Name: "svc-http", Command: []string{"cmd"},
					ReadyCheck: &ReadyCheck{Type: "http", Target: "http://localhost:8080/health", Timeout: "10s"},
				},
				{
					Name: "svc-delay", Command: []string{"cmd"},
					ReadyCheck: &ReadyCheck{Type: "delay", Target: "3s", Timeout: "5s"},
				},
			},
			wantErr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateServices(tt.services)
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("ValidateServices() unexpected error: %v", err)
				}
			} else {
				if err == nil {
					t.Errorf("ValidateServices() expected error containing %q, got nil", tt.wantErr)
				} else if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("ValidateServices() error = %q, want containing %q", err.Error(), tt.wantErr)
				}
			}
		})
	}
}
