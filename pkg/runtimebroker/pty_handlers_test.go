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

package runtimebroker

import (
	"context"
	"testing"
	"time"
)

func TestWaitForTmuxSession_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := waitForTmuxSession(ctx, "false", "nonexistent-container", "", nil, nil)
	if err == nil {
		t.Fatal("expected error when context is cancelled")
	}
}

func TestWaitForTmuxSession_TimesOut(t *testing.T) {
	// Use a very short timeout to test the timeout path quickly.
	// "false" always exits with code 1, simulating tmux has-session failure.
	ctx, cancel := context.WithTimeout(context.Background(), 600*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := waitForTmuxSession(ctx, "false", "nonexistent-container", "", nil, nil)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected timeout error")
	}
	if elapsed < 500*time.Millisecond {
		t.Errorf("expected to wait at least 500ms before timing out, got %v", elapsed)
	}
}

func TestWaitForTmuxSession_SucceedsImmediately(t *testing.T) {
	// "true" always exits with code 0, simulating tmux has-session success.
	// We pass extra args that "true" ignores.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	start := time.Now()
	err := waitForTmuxSession(ctx, "true", "any-container", "", nil, nil)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	// First poll is at 500ms, so it should complete around that time
	if elapsed > 2*time.Second {
		t.Errorf("expected quick completion, took %v", elapsed)
	}
}
