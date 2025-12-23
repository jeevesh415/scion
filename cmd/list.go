package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/ptone/scion-agent/pkg/config"
	"github.com/ptone/scion-agent/pkg/runtime"
	"github.com/spf13/cobra"
)

var (
	listAll bool
)

// listCmd represents the list command
var listCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List running scion agents",
	RunE: func(cmd *cobra.Command, args []string) error {
		rt := runtime.GetRuntime()
		filters := map[string]string{
			"scion.agent": "true",
		}

		if !listAll {
			projectDir, err := config.GetResolvedProjectDir(grovePath)
			if err != nil {
				return err
			}
			groveName := config.GetGroveName(projectDir)
			filters["scion.grove"] = groveName
		}

		agents, err := rt.List(context.Background(), filters)
		if err != nil {
			return err
		}

		// Also find "created" agents that don't have a container yet
		var grovesToScan []string
		if listAll {
			// This is a bit hard since we don't track all groves
			// For now, let's at least check current and global
			if pd, err := config.GetResolvedProjectDir(""); err == nil {
				grovesToScan = append(grovesToScan, pd)
			}
			if gd, err := config.GetGlobalDir(); err == nil {
				grovesToScan = append(grovesToScan, gd)
			}
		} else {
			projectDir, err := config.GetResolvedProjectDir(grovePath)
			if err == nil {
				grovesToScan = append(grovesToScan, projectDir)
			}
		}

		runningNames := make(map[string]bool)
		for _, a := range agents {
			runningNames[a.Name] = true
		}

		for _, gp := range grovesToScan {
			agentsDir := filepath.Join(gp, "agents")
			entries, err := os.ReadDir(agentsDir)
			if err != nil {
				continue
			}
			groveName := config.GetGroveName(gp)
			for _, e := range entries {
				if !e.IsDir() {
					continue
				}
				if runningNames[e.Name()] {
					continue
				}

				// Check scion.json
				agentScionJSON := filepath.Join(agentsDir, e.Name(), "home", "scion.json")
				data, err := os.ReadFile(agentScionJSON)
				if err != nil {
					continue
				}
				var cfg config.ScionConfig
				if err := json.Unmarshal(data, &cfg); err == nil && cfg.Agent != nil {
					if cfg.Agent.Status == "created" {
						agents = append(agents, runtime.AgentInfo{
							Name:      e.Name(),
							Template:  cfg.Template,
							Grove:     groveName,
							GrovePath: gp,
							Status:    "created",
							Image:     cfg.Image,
						})
					}
				}
			}
		}

		if len(agents) == 0 {
			if listAll {
				fmt.Println("No active agents found across any groves.")
			} else {
				fmt.Println("No active agents found in the current grove.")
			}
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "NAME\tTEMPLATE\tGROVE\tAGENT STATUS\tCONTAINER")
		for _, a := range agents {
			agentStatus := "unknown"
			if a.GrovePath != "" {
				// agent home: <GrovePath>/agents/<AgentName>/home/scion.json
				agentScionJSON := filepath.Join(a.GrovePath, "agents", a.Name, "home", "scion.json")
				data, err := os.ReadFile(agentScionJSON)
				if err == nil {
					var cfg config.ScionConfig
					if err := json.Unmarshal(data, &cfg); err == nil && cfg.Agent != nil {
						agentStatus = cfg.Agent.Status
						if agentStatus == "" {
							agentStatus = "IDLE"
						}
					}
				}
			}
			containerStatus := a.Status
			if containerStatus == "created" && a.ID == "" {
				containerStatus = "none"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", a.Name, a.Template, a.Grove, agentStatus, containerStatus)
		}
		w.Flush()
		return nil
	},
}

func init() {
	rootCmd.AddCommand(listCmd)
	listCmd.Flags().BoolVarP(&listAll, "all", "a", false, "List all agents across all groves")
}

