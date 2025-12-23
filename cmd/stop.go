package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ptone/scion/pkg/config"
	"github.com/ptone/scion/pkg/runtime"
	"github.com/ptone/scion/pkg/util"
	"github.com/spf13/cobra"
)

// stopCmd represents the stop command
var stopCmd = &cobra.Command{
	Use:   "stop <agent>",
	Short: "Stop and remove an agent",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		agentName := args[0]
		rt := runtime.GetRuntime()
		
		fmt.Printf("Stopping agent '%s'...\n", agentName)
		if err := rt.Stop(context.Background(), agentName); err != nil {
			return err
		}

		// Cleanup worktree if it exists in either project or global location
		var agentsDirs []string
		if pDir, err := config.GetProjectDir(); err == nil {
			agentsDirs = append(agentsDirs, filepath.Join(pDir, "agents"))
		}
		if globalDir, err := config.GetGlobalAgentsDir(); err == nil {
			agentsDirs = append(agentsDirs, globalDir)
		}

		for _, dir := range agentsDirs {
			agentWorkspace := filepath.Join(dir, agentName, "workspace")
			if util.IsGitRepo() {
				// Check if it's a worktree before trying to remove it
				if _, err := os.Stat(filepath.Join(agentWorkspace, ".git")); err == nil {
					fmt.Printf("Removing git worktree for agent '%s'...\n", agentName)
					if err := util.RemoveWorktree(agentWorkspace); err != nil {
						fmt.Printf("Warning: failed to remove worktree at %s: %v\n", agentWorkspace, err)
					}
				}
			}
			// Also ensure the agent directory is cleaned up
			agentDir := filepath.Join(dir, agentName)
			if _, err := os.Stat(agentDir); err == nil {
				os.RemoveAll(agentDir)
			}
		}

		fmt.Printf("Agent '%s' stopped and removed.\n", agentName)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(stopCmd)
}

