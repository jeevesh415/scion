package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ptone/scion/pkg/config"
	"github.com/ptone/scion/pkg/runtime"
	"github.com/ptone/scion/pkg/util"
	"github.com/spf13/cobra"
)

var (
	agentName    string
	templateName string
	agentImage   string
	noAuth       bool
	attach       bool
	model        string
)

// startCmd represents the start command
var startCmd = &cobra.Command{
	Use:   "start <agent-name> <task...>",
	Short: "Launch a new scion agent",
	Long: `Provision and launch a new isolated Gemini agent to perform a specific task.
The agent will be created from a template and run in a detached container.

The agent-name is required as the first argument. All subsequent arguments 
form the task prompt, which is passed to the gemini command.`,
	Args: cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		agentName = args[0]
		task := strings.Join(args[1:], " ")

		fmt.Printf("Starting agent '%s' for task: %s\n", agentName, task)

		// 1. Prepare agent directories
		var agentsDir string
		projectDir, err := config.GetProjectDir()
		hasProjectConfig := false
		if err == nil {
			if info, err := os.Stat(projectDir); err == nil && info.IsDir() {
				hasProjectConfig = true
			}
		}

		if hasProjectConfig {
			// If .scion exists, verify .gitignore if in a repo
			if util.IsGitRepo() {
				if !util.IsIgnored(".scion/agents/") {
					return fmt.Errorf("security error: '.scion/agents/' must be in .gitignore when using a project-local grove")
				}
			}
			agentsDir = filepath.Join(projectDir, "agents")
		} else {
			// Fallback to global agents directory
			var err error
			agentsDir, err = config.GetGlobalAgentsDir()
			if err != nil {
				return err
			}
		}

		agentDir := filepath.Join(agentsDir, agentName)
		agentHome := filepath.Join(agentDir, "home")
		agentWorkspace := filepath.Join(agentDir, "workspace")

		if err := os.MkdirAll(agentHome, 0755); err != nil {
			return fmt.Errorf("failed to create agent home: %w", err)
		}

		if util.IsGitRepo() {
			fmt.Printf("Creating git worktree for agent '%s'...\n", agentName)
			// Remove existing workspace dir if it exists to allow worktree add
			os.RemoveAll(agentWorkspace)
			if err := util.CreateWorktree(agentWorkspace, agentName); err != nil {
				return fmt.Errorf("failed to create git worktree: %w", err)
			}
		} else {
			if err := os.MkdirAll(agentWorkspace, 0755); err != nil {
				return fmt.Errorf("failed to create agent workspace: %w", err)
			}
		}

		// 2. Load and copy templates
		chain, err := config.GetTemplateChain(templateName)
		if err != nil {
			return fmt.Errorf("failed to load template: %w", err)
		}

		// Track image from templates
		resolvedImage := ""

		for _, tpl := range chain {
			fmt.Printf("Applying template: %s\n", tpl.Name)
			if err := util.CopyDir(tpl.Path, agentHome); err != nil {
				return fmt.Errorf("failed to copy template %s: %w", tpl.Name, err)
			}

			// Load scion.json from this template to see if it specifies an image
			tplCfg, err := tpl.LoadConfig()
			if err == nil && tplCfg.Image != "" {
				resolvedImage = tplCfg.Image
			}
		}

		// Flag takes ultimate precedence
		if agentImage != "" {
			resolvedImage = agentImage
		}
		if resolvedImage == "" {
			resolvedImage = "gemini-cli-sandbox"
		}

		// 3. Propagate credentials
		var auth config.AuthConfig
		if !noAuth {
			// Load agent settings from the newly prepared home directory
			agentSettingsPath := filepath.Join(agentHome, ".gemini", "settings.json")
			agentSettings, _ := config.LoadGeminiSettings(agentSettingsPath)
			auth = config.DiscoverAuth(agentSettings)
		}

		// 4. Launch container
		rt := runtime.GetRuntime()

		// Determine detached mode and tmux from templates (last one wins)
		detached := true
		useTmux := false
		resolvedModel := "flash"
		for _, tpl := range chain {
			tplCfg, err := tpl.LoadConfig()
			if err == nil {
				detached = tplCfg.IsDetached()
				if tplCfg.UseTmux {
					useTmux = true
				}
				if tplCfg.Model != "" {
					resolvedModel = tplCfg.Model
				}
			}
		}

		// -a flag overrides detached config
		if cmd.Flags().Changed("attach") {
			detached = !attach
		}

		if model != "" {
			resolvedModel = model
		}

		if useTmux {
			tmuxImage := resolvedImage
			if !strings.Contains(tmuxImage, ":") {
				tmuxImage = tmuxImage + ":tmux"
			} else {
				parts := strings.SplitN(resolvedImage, ":", 2)
				tmuxImage = parts[0] + ":tmux"
			}

			exists, err := rt.ImageExists(context.Background(), tmuxImage)
			if err != nil || !exists {
				return fmt.Errorf("tmux support requested but image '%s' not found. Please ensure the image has a :tmux tag.", tmuxImage)
			}
			resolvedImage = tmuxImage
		}

		runCfg := runtime.RunConfig{
			Name:      agentName,
			Image:     resolvedImage,
			HomeDir:   agentHome,
			Workspace: agentWorkspace,
			Auth:      auth,
			UseTmux:   useTmux,
			Model:     resolvedModel,
			Task:      task,
			Env: []string{
				fmt.Sprintf("GEMINI_AGENT_NAME=%s", agentName),
			},
			Labels: map[string]string{
				"scion.agent": "true",
				"scion.name":  agentName,
			},
		}

		id, err := rt.Run(context.Background(), runCfg)
		if err != nil {
			return fmt.Errorf("failed to launch container: %w", err)
		}

		if detached {
			fmt.Printf("Agent '%s' launched successfully (ID: %s)\n", agentName, id)
		} else {
			fmt.Printf("Attaching to agent '%s'...\n", agentName)
			return rt.Attach(context.Background(), id)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(startCmd)
	startCmd.Flags().StringVarP(&templateName, "type", "t", "default", "Template to use")
	startCmd.Flags().StringVarP(&agentImage, "image", "i", "", "Container image to use (overrides template)")
	startCmd.Flags().BoolVar(&noAuth, "no-auth", false, "Disable authentication propagation")
	startCmd.Flags().BoolVarP(&attach, "attach", "a", false, "Attach to the agent TTY after starting")
	startCmd.Flags().StringVarP(&model, "model", "m", "", "Model to use (overrides template)")
}
			
