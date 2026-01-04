package util

import (
	"fmt"
	"os/exec"
	"strings"
)

// IsGitRepo returns true if the current working directory is inside a git repository.
func IsGitRepo() bool {
	cmd := exec.Command("git", "rev-parse", "--is-inside-work-tree")
	err := cmd.Run()
	return err == nil
}

// GetGitVersion returns the git version string.
func GetGitVersion() (string, error) {
	cmd := exec.Command("git", "--version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}
	// Output is usually "git version 2.47.0"
	return strings.TrimPrefix(strings.TrimSpace(string(output)), "git version "), nil
}

// CheckGitVersion returns an error if the git version is less than 2.47.0.
func CheckGitVersion() error {
	version, err := GetGitVersion()
	if err != nil {
		return fmt.Errorf("failed to get git version: %w", err)
	}

	if err := CompareGitVersion(version, 2, 47); err != nil {
		return fmt.Errorf("git version 2.47.0 or newer is required; scion requires worktree support with relative paths (found %s)", version)
	}

	return nil
}

// CompareGitVersion returns an error if the version string is less than major.minor
func CompareGitVersion(version string, minMajor, minMinor int) error {
	// Simple version comparison
	// Format is expected to start with major.minor
	parts := strings.Split(version, ".")
	if len(parts) < 2 {
		return fmt.Errorf("unexpected git version format: %s", version)
	}

	var major, minor int
	fmt.Sscanf(parts[0], "%d", &major)
	fmt.Sscanf(parts[1], "%d", &minor)

	if major < minMajor || (major == minMajor && minor < minMinor) {
		return fmt.Errorf("version %s is less than %d.%d", version, minMajor, minMinor)
	}

	return nil
}

// RepoRoot returns the absolute path to the root of the git repository.
func RepoRoot() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// IsIgnored returns true if the given path is ignored by git.
func IsIgnored(path string) bool {
	cmd := exec.Command("git", "check-ignore", "-q", path)
	err := cmd.Run()
	return err == nil
}

// CreateWorktree creates a new git worktree at the specified path with a new branch.
func CreateWorktree(path, branch string) error {
	// git worktree add --relative-paths -b <branch> <path>
	cmd := exec.Command("git", "worktree", "add", "--relative-paths", "-b", branch, path)
	if output, err := cmd.CombinedOutput(); err != nil {
		// If branch already exists, try to just add it
		if strings.Contains(string(output), "already exists") {
			cmd = exec.Command("git", "worktree", "add", "--relative-paths", path, branch)
			return cmd.Run()
		}
		return err
	}
	return nil
}

// RemoveWorktree removes a git worktree at the specified path.
func RemoveWorktree(path string, deleteBranch bool) error {
	var branchName string
	var repoRoot string

	if deleteBranch {
		// Get repo root
		cmd := exec.Command("git", "-C", path, "rev-parse", "--show-toplevel")
		output, err := cmd.Output()
		if err == nil {
			repoRoot = strings.TrimSpace(string(output))
		}

		// Get branch name
		cmd = exec.Command("git", "-C", path, "branch", "--show-current")
		output, err = cmd.Output()
		if err == nil {
			branchName = strings.TrimSpace(string(output))
		}
	}

	// Try to remove it by running git from within the worktree itself.
	// We use "." as the path to remove the worktree we are "in" via -C.
	cmd := exec.Command("git", "-C", path, "worktree", "remove", ".", "--force")
	if err := cmd.Run(); err != nil {
		return err
	}

	if deleteBranch && branchName != "" && repoRoot != "" {
		cmd := exec.Command("git", "-C", repoRoot, "branch", "-D", branchName)
		return cmd.Run()
	}
	return nil
}
