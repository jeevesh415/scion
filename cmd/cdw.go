package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"github.com/ptone/scion-agent/pkg/config"
	"github.com/ptone/scion-agent/pkg/util"
	"github.com/spf13/cobra"
)

var cdwCmd = &cobra.Command{
	Use:   "cdw <agent-name|branch-name>",
	Short: "Change directory to an agent's workspace or a branch's worktree",
	Long: `Resolves the path to a worktree and changes into (starts a new shell in that directory).
First checks for an agent by name and enters its workspace if found.
Then checks for any git worktree checked out to the specified branch.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]
		var targetPath string

		// 1. Check for Agent
		projectDir, _ := config.GetResolvedProjectDir(grovePath)

		// Check project grove
		if projectDir != "" {
			agentDir := filepath.Join(projectDir, "agents", name)
			workspace := filepath.Join(agentDir, "workspace")
			if _, err := os.Stat(workspace); err == nil {
				targetPath = workspace
			}
		}

		// Check global grove if not found
		if targetPath == "" {
			globalDir, _ := config.GetGlobalAgentsDir()
			if globalDir != "" {
				// globalDir is .../agents, so agent path is globalDir/name
				agentDir := filepath.Join(globalDir, name)
				workspace := filepath.Join(agentDir, "workspace")
				if _, err := os.Stat(workspace); err == nil {
					targetPath = workspace
				}
			}
		}

		// 2. Check for Branch Worktree if not found
		if targetPath == "" && util.IsGitRepo() {
			path, err := util.FindWorktreeByBranch(name)
			if err == nil && path != "" {
				targetPath = path
			}
		}

		// Not found
		if targetPath == "" {
			fmt.Fprintf(os.Stderr, "Error: no agent or worktree found for '%s'\n", name)
			os.Exit(1)
		}

		// Change directory
		if err := os.Chdir(targetPath); err != nil {
			fmt.Fprintf(os.Stderr, "Error changing directory to '%s': %v\n", targetPath, err)
			os.Exit(1)
		}

		// Get shell
		shell := os.Getenv("SHELL")
		if shell == "" {
			shell = "bash" // fallback
		}

		shellPath, err := exec.LookPath(shell)
		if err != nil {
			// Try /bin/bash or /bin/sh if LookPath fails
			if _, err := os.Stat("/bin/bash"); err == nil {
				shellPath = "/bin/bash"
			} else if _, err := os.Stat("/bin/sh"); err == nil {
				shellPath = "/bin/sh"
			} else {
				fmt.Fprintf(os.Stderr, "Error finding shell '%s': %v\n", shell, err)
				os.Exit(1)
			}
		}

		// Execute shell
		// We use syscall.Exec to replace the current process with the shell
		err = syscall.Exec(shellPath, []string{shell}, os.Environ())
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error executing shell '%s': %v\n", shellPath, err)
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(cdwCmd)
}
