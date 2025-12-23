package cmd

import (
	"context"
	"fmt"

	"github.com/ptone/scion-agent/pkg/runtime"
	"github.com/spf13/cobra"
)

// deleteCmd represents the delete command
var deleteCmd = &cobra.Command{
	Use:     "delete <agent>",
	Aliases: []string{"rm"},
	Short:   "Delete an agent",
	Long:    `Stop and remove an agent container and its associated files and worktree.`,
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		agentName := args[0]
		rt := runtime.GetRuntime()

		fmt.Printf("Deleting agent '%s'...\n", agentName)
		
		// Try to stop first, ignore error if already stopped
		_ = rt.Stop(context.Background(), agentName)

		if err := rt.Delete(context.Background(), agentName); err != nil {
			fmt.Printf("Warning: failed to delete container: %v\n", err)
		}

		if err := DeleteAgentFiles(agentName); err != nil {
			return err
		}

		fmt.Printf("Agent '%s' deleted.\n", agentName)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(deleteCmd)
}

