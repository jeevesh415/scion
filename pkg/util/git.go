package util

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
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
		outputStr := string(output)
		// If branch already exists, try to just add it
		if strings.Contains(outputStr, "already exists") {
			cmd = exec.Command("git", "worktree", "add", "--relative-paths", path, branch)
			if output, err := cmd.CombinedOutput(); err != nil {
				outputStr = string(output)
				if strings.Contains(outputStr, "already checked out") {
					return fmt.Errorf("branch '%s' is already checked out in another worktree", branch)
				}
				return fmt.Errorf("failed to create worktree: %s", strings.TrimSpace(outputStr))
			}
			return nil
		}
		return fmt.Errorf("failed to create worktree: %s", strings.TrimSpace(outputStr))
	}
	return nil
}

// RemoveWorktree removes a git worktree at the specified path.
func RemoveWorktree(path string, deleteBranch bool) (bool, error) {
	var branchName string
	var repoRoot string
	branchDeleted := false

	if deleteBranch {
		// Get the common git dir (main repo's .git dir)
		cmd := exec.Command("git", "-C", path, "rev-parse", "--git-common-dir")
		output, err := cmd.Output()
		if err == nil {
			commonDir := strings.TrimSpace(string(output))
			if !filepath.IsAbs(commonDir) {
				// If relative, it's relative to the worktree root
				commonDir = filepath.Join(path, commonDir)
			}
			repoRoot = filepath.Dir(commonDir)
		}

		// Get branch name
		cmd = exec.Command("git", "-C", path, "branch", "--show-current")
		output, err = cmd.Output()
		if err == nil {
			branchName = strings.TrimSpace(string(output))
		}
	}

	// Remove the worktree. 
	// We run this from the system root or anywhere to ensure we're not "in" the dir
	cmd := exec.Command("git", "worktree", "remove", path, "--force")
	if err := cmd.Run(); err != nil {
		return false, err
	}

	if deleteBranch && branchName != "" && repoRoot != "" {
		// Now delete the branch from the main repo
		cmd := exec.Command("git", "-C", repoRoot, "branch", "-D", branchName)
		if err := cmd.Run(); err == nil {
			branchDeleted = true
		}
	}
	return branchDeleted, nil
}

// PruneWorktrees prunes worktree information for worktrees that no longer exist.
func PruneWorktrees() error {
	cmd := exec.Command("git", "worktree", "prune")
	return cmd.Run()
}

// FindWorktreeByBranch returns the absolute path of the worktree checked out to the specified branch.
// It returns an empty string if not found.
func FindWorktreeByBranch(branchName string) (string, error) {
	cmd := exec.Command("git", "worktree", "list", "--porcelain")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	blocks := strings.Split(string(output), "\n\n")
	targetRef := "refs/heads/" + branchName

	for _, block := range blocks {
		lines := strings.Split(block, "\n")
		var path string
		var branch string
		for _, line := range lines {
			if strings.HasPrefix(line, "worktree ") {
				path = strings.TrimPrefix(line, "worktree ")
				if strings.HasPrefix(path, "\"") {
					if unquoted, err := strconv.Unquote(path); err == nil {
						path = unquoted
					}
				}
			} else if strings.HasPrefix(line, "branch ") {
				branch = strings.TrimPrefix(line, "branch ")
			}
		}
		if branch == targetRef {
			return path, nil
		}
	}
	return "", nil
}
