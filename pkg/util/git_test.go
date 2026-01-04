package util

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func setupGitRepo(t *testing.T) string {
	dir := t.TempDir()
	
	// Initialize git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to init git repo: %v", err)
	}

	// Config user for commits
	configCmds := [][]string{
		{"config", "user.email", "you@example.com"},
		{"config", "user.name", "Your Name"},
		{"commit", "--allow-empty", "-m", "root commit"},
	}

	for _, args := range configCmds {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if err := cmd.Run(); err != nil {
			t.Fatalf("failed to run git %v: %v", args, err)
		}
	}

	return dir
}

func TestGitUtils(t *testing.T) {
	// Need to be inside the repo for most tests
	repoDir := setupGitRepo(t)
	
	// Save current working dir to restore later
	originalWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(originalWd)

	if err := os.Chdir(repoDir); err != nil {
		t.Fatal(err)
	}

	t.Run("IsGitRepo", func(t *testing.T) {
		if !IsGitRepo() {
			t.Error("expected true, got false")
		}
	})

	t.Run("RepoRoot", func(t *testing.T) {
		root, err := RepoRoot()
		if err != nil {
			t.Errorf("RepoRoot failed: %v", err)
		}
		// RepoRoot usually returns path with symlinks resolved, matching t.TempDir behavior
		// On macOS t.TempDir might be in /var/folders/... which is a symlink to /private/var/folders/...
		// We resolve both to compare safely.
		evalRoot, _ := filepath.EvalSymlinks(root)
		evalRepoDir, _ := filepath.EvalSymlinks(repoDir)
		
		if evalRoot != evalRepoDir {
			t.Errorf("expected root %q, got %q", evalRepoDir, evalRoot)
		}
	})

	t.Run("IsIgnored", func(t *testing.T) {
		ignoreFile := filepath.Join(repoDir, ".gitignore")
		if err := os.WriteFile(ignoreFile, []byte("ignored.txt"), 0644); err != nil {
			t.Fatal(err)
		}
		
		if !IsIgnored("ignored.txt") {
			t.Error("expected ignored.txt to be ignored")
		}
		
		if IsIgnored("not-ignored.txt") {
			t.Error("expected not-ignored.txt to NOT be ignored")
		}
	})

	t.Run("Worktrees", func(t *testing.T) {
		worktreePath := filepath.Join(repoDir, "wt-test")
		branchName := "test-branch"

		// Create
		if err := CreateWorktree(worktreePath, branchName); err != nil {
			t.Fatalf("CreateWorktree failed: %v", err)
		}

		if _, err := os.Stat(worktreePath); os.IsNotExist(err) {
			t.Errorf("worktree dir does not exist")
		}

		// Remove
			if err := RemoveWorktree(worktreePath, false); err != nil {
				t.Fatalf("RemoveWorktree failed: %v", err)
			}		
		// Wait/Check? git worktree remove deletes the directory usually.
		if _, err := os.Stat(worktreePath); !os.IsNotExist(err) {
			t.Errorf("worktree dir still exists after removal")
		}
	})

	t.Run("CompareGitVersion", func(t *testing.T) {
		tests := []struct {
			version string
			major   int
			minor   int
			wantErr bool
		}{
			{"2.47.0", 2, 47, false},
			{"2.48.0", 2, 47, false},
			{"3.0.0", 2, 47, false},
			{"2.46.9", 2, 47, true},
			{"1.9.0", 2, 47, true},
			{"2.47.1.windows.1", 2, 47, false},
			{"invalid", 2, 47, true},
		}

		for _, tt := range tests {
			err := CompareGitVersion(tt.version, tt.major, tt.minor)
			if (err != nil) != tt.wantErr {
				t.Errorf("CompareGitVersion(%q, %d, %d) error = %v, wantErr %v", tt.version, tt.major, tt.minor, err, tt.wantErr)
			}
		}
	})
}
