package cmd

import (
	"fmt"

	"github.com/ptone/scion/pkg/config"
	"github.com/spf13/cobra"
)

var globalInit bool

// groveCmd represents the grove command
var groveCmd = &cobra.Command{
	Use:     "grove",
	Aliases: []string{"group"},
	Short:   "Manage scion groves (agent groups)",
	Long:    `A grove is the grouping construct for a set of agents. The .scion folder represents a grove.`,
}

// groveInitCmd represents the init subcommand for grove
var groveInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a new grove",
	Long: `Initialize a new grove by creating the .scion directory structure
and seeding the default template. 

By default, it initializes in:
- The root of the current git repo if run inside a repo
- The current directory

With --global, it initializes in the user's home folder.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if globalInit {
			fmt.Println("Initializing global scion directory...")
			if err := config.InitGlobal(); err != nil {
				return fmt.Errorf("failed to initialize global config: %w", err)
			}
		} else {
			fmt.Println("Initializing scion project grove...")
			if err := config.InitProject(""); err != nil {
				return fmt.Errorf("failed to initialize project grove: %w", err)
			}
		}

		fmt.Println("scion grove successfully initialized.")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(groveCmd)
	groveCmd.AddCommand(groveInitCmd)

	groveInitCmd.Flags().BoolVarP(&globalInit, "global", "g", false, "Initialize the global grove in the home directory")
}
