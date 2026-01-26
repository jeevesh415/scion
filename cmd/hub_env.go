package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ptone/scion-agent/pkg/config"
	"github.com/ptone/scion-agent/pkg/hubclient"
	"github.com/spf13/cobra"
)

var (
	envGroveScope string
	envHostScope  string
	envOutputJSON bool
)

// hubEnvCmd is the parent command for environment variable operations
var hubEnvCmd = &cobra.Command{
	Use:   "env",
	Short: "Manage environment variables",
	Long: `Manage environment variables stored in the Hub.

Environment variables can be scoped to:
  - User (default): Available to all your agents
  - Grove: Available to agents in a specific grove
  - Host: Available to agents running on a specific host

Variables are resolved hierarchically when an agent starts:
  user -> grove -> host -> agent config

Examples:
  # Set a user-scoped variable
  scion hub env set API_URL https://api.example.com

  # Set a grove-scoped variable (infer grove from current directory)
  scion hub env set --grove API_URL https://api.example.com

  # Set a grove-scoped variable with explicit grove ID
  scion hub env set --grove=abc123 API_URL https://api.example.com

  # List all user variables
  scion hub env get

  # Get a specific variable
  scion hub env get API_URL

  # Delete a variable
  scion hub env clear API_URL`,
}

// hubEnvSetCmd sets an environment variable
var hubEnvSetCmd = &cobra.Command{
	Use:   "set KEY VALUE",
	Short: "Set an environment variable",
	Long: `Set an environment variable in the Hub.

By default, variables are scoped to the current user. Use --grove or --host
to set variables at different scopes.

Examples:
  scion hub env set API_URL https://api.example.com
  scion hub env set --grove LOG_LEVEL debug
  scion hub env set --host DATABASE_HOST localhost`,
	Args: cobra.ExactArgs(2),
	RunE: runEnvSet,
}

// hubEnvGetCmd gets environment variables
var hubEnvGetCmd = &cobra.Command{
	Use:   "get [KEY]",
	Short: "Get environment variables",
	Long: `Get environment variables from the Hub.

Without a key, lists all variables for the scope.
With a key, returns the specific variable.

Examples:
  scion hub env get                    # List all user variables
  scion hub env get API_URL            # Get specific variable
  scion hub env get --grove            # List grove variables
  scion hub env get --grove API_URL    # Get grove variable`,
	Args: cobra.MaximumNArgs(1),
	RunE: runEnvGet,
}

// hubEnvClearCmd clears an environment variable
var hubEnvClearCmd = &cobra.Command{
	Use:   "clear KEY",
	Short: "Clear an environment variable",
	Long: `Remove an environment variable from the Hub.

Examples:
  scion hub env clear API_URL
  scion hub env clear --grove API_URL
  scion hub env clear --host API_URL`,
	Args: cobra.ExactArgs(1),
	RunE: runEnvClear,
}

func init() {
	hubCmd.AddCommand(hubEnvCmd)
	hubEnvCmd.AddCommand(hubEnvSetCmd)
	hubEnvCmd.AddCommand(hubEnvGetCmd)
	hubEnvCmd.AddCommand(hubEnvClearCmd)

	// Add scope flags to all subcommands
	for _, cmd := range []*cobra.Command{hubEnvSetCmd, hubEnvGetCmd, hubEnvClearCmd} {
		cmd.Flags().StringVar(&envGroveScope, "grove", "", "Grove scope (use flag without value to infer from current directory, or provide grove ID)")
		cmd.Flags().StringVar(&envHostScope, "host", "", "Host scope (use flag without value to use current host, or provide host ID)")
	}

	hubEnvGetCmd.Flags().BoolVar(&envOutputJSON, "json", false, "Output in JSON format")
}

// resolveEnvScope determines the scope and scopeID based on flags
func resolveEnvScope(cmd *cobra.Command, settings *config.Settings) (scope, scopeID string, err error) {
	groveSet := cmd.Flags().Changed("grove")
	hostSet := cmd.Flags().Changed("host")

	if groveSet && hostSet {
		return "", "", fmt.Errorf("cannot specify both --grove and --host")
	}

	if groveSet {
		scope = "grove"
		if envGroveScope != "" {
			scopeID = envGroveScope
		} else {
			// Infer from settings
			if settings.Hub != nil && settings.Hub.GroveID != "" {
				scopeID = settings.Hub.GroveID
			} else {
				return "", "", fmt.Errorf("cannot infer grove ID: not registered with Hub. Use 'scion hub register' first or provide explicit grove ID")
			}
		}
		return scope, scopeID, nil
	}

	if hostSet {
		scope = "runtime_host"
		if envHostScope != "" {
			scopeID = envHostScope
		} else {
			// Infer from settings
			if settings.Hub != nil && settings.Hub.HostID != "" {
				scopeID = settings.Hub.HostID
			} else {
				return "", "", fmt.Errorf("cannot infer host ID: not registered with Hub. Use 'scion hub register' first or provide explicit host ID")
			}
		}
		return scope, scopeID, nil
	}

	// Default to user scope
	return "user", "", nil
}

func runEnvSet(cmd *cobra.Command, args []string) error {
	key := args[0]
	value := args[1]

	// Validate key
	if key == "" {
		return fmt.Errorf("key cannot be empty")
	}
	if strings.ContainsAny(key, "= \t\n") {
		return fmt.Errorf("key cannot contain spaces, tabs, newlines, or '='")
	}

	resolvedPath, _, err := config.ResolveGrovePath(grovePath)
	if err != nil {
		return fmt.Errorf("failed to resolve grove path: %w", err)
	}

	settings, err := config.LoadSettings(resolvedPath)
	if err != nil {
		return fmt.Errorf("failed to load settings: %w", err)
	}

	client, err := getHubClient(settings)
	if err != nil {
		return err
	}

	scope, scopeID, err := resolveEnvScope(cmd, settings)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req := &hubclient.SetEnvRequest{
		Value:   value,
		Scope:   scope,
		ScopeID: scopeID,
	}

	resp, err := client.Env().Set(ctx, key, req)
	if err != nil {
		return fmt.Errorf("failed to set environment variable: %w", err)
	}

	if resp.Created {
		fmt.Printf("Created %s=%s (scope: %s)\n", key, value, scope)
	} else {
		fmt.Printf("Updated %s=%s (scope: %s)\n", key, value, scope)
	}

	return nil
}

func runEnvGet(cmd *cobra.Command, args []string) error {
	resolvedPath, _, err := config.ResolveGrovePath(grovePath)
	if err != nil {
		return fmt.Errorf("failed to resolve grove path: %w", err)
	}

	settings, err := config.LoadSettings(resolvedPath)
	if err != nil {
		return fmt.Errorf("failed to load settings: %w", err)
	}

	client, err := getHubClient(settings)
	if err != nil {
		return err
	}

	scope, scopeID, err := resolveEnvScope(cmd, settings)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// If key is provided, get specific variable
	if len(args) == 1 {
		key := args[0]
		opts := &hubclient.EnvScopeOptions{
			Scope:   scope,
			ScopeID: scopeID,
		}

		envVar, err := client.Env().Get(ctx, key, opts)
		if err != nil {
			return fmt.Errorf("failed to get environment variable: %w", err)
		}

		if envOutputJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(envVar)
		}

		if envVar.Sensitive {
			fmt.Printf("%s=****** (sensitive, scope: %s)\n", envVar.Key, envVar.Scope)
		} else {
			fmt.Printf("%s=%s\n", envVar.Key, envVar.Value)
		}
		return nil
	}

	// List all variables for scope
	opts := &hubclient.ListEnvOptions{
		Scope:   scope,
		ScopeID: scopeID,
	}

	resp, err := client.Env().List(ctx, opts)
	if err != nil {
		return fmt.Errorf("failed to list environment variables: %w", err)
	}

	if envOutputJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(resp)
	}

	if len(resp.EnvVars) == 0 {
		fmt.Printf("No environment variables found (scope: %s)\n", scope)
		return nil
	}

	fmt.Printf("Environment variables (scope: %s):\n", scope)
	for _, v := range resp.EnvVars {
		if v.Sensitive {
			fmt.Printf("  %s=****** (sensitive)\n", v.Key)
		} else {
			fmt.Printf("  %s=%s\n", v.Key, v.Value)
		}
	}

	return nil
}

func runEnvClear(cmd *cobra.Command, args []string) error {
	key := args[0]

	resolvedPath, _, err := config.ResolveGrovePath(grovePath)
	if err != nil {
		return fmt.Errorf("failed to resolve grove path: %w", err)
	}

	settings, err := config.LoadSettings(resolvedPath)
	if err != nil {
		return fmt.Errorf("failed to load settings: %w", err)
	}

	client, err := getHubClient(settings)
	if err != nil {
		return err
	}

	scope, scopeID, err := resolveEnvScope(cmd, settings)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	opts := &hubclient.EnvScopeOptions{
		Scope:   scope,
		ScopeID: scopeID,
	}

	if err := client.Env().Delete(ctx, key, opts); err != nil {
		return fmt.Errorf("failed to delete environment variable: %w", err)
	}

	fmt.Printf("Deleted %s (scope: %s)\n", key, scope)
	return nil
}
