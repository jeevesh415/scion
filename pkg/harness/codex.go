package harness

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ptone/scion-agent/pkg/api"
	"github.com/ptone/scion-agent/pkg/util"
)

type Codex struct{}

func (c *Codex) Name() string {
	return "codex"
}

func (c *Codex) DiscoverAuth(agentHome string) api.AuthConfig {
	auth := api.AuthConfig{}
	// Check for Codex auth file in standard location
	home, _ := os.UserHomeDir()
	authPath := filepath.Join(home, ".codex", "auth.json")
	if _, err := os.Stat(authPath); err == nil {
		auth.CodexAuthFile = authPath
	}
	return auth
}

func (c *Codex) GetEnv(agentName string, agentHome string, unixUsername string, auth api.AuthConfig) map[string]string {
	env := make(map[string]string)
	if os.Getenv("OPENAI_API_KEY") != "" {
		env["OPENAI_API_KEY"] = os.Getenv("OPENAI_API_KEY")
	}
	if os.Getenv("CODEX_API_KEY") != "" {
		env["CODEX_API_KEY"] = os.Getenv("CODEX_API_KEY")
	}
	return env
}

func (c *Codex) GetCommand(task string, resume bool, baseArgs []string) []string {
	args := []string{"codex", "--yolo"}
	if resume {
		args = append(args, "resume", "--last")
	} else {
		if task != "" {
			args = append(args, task)
		}
	}

	args = append(args, baseArgs...)
	return args
}

func (c *Codex) PropagateFiles(homeDir, unixUsername string, auth api.AuthConfig) error {
	if auth.CodexAuthFile != "" {
		dst := filepath.Join(homeDir, ".codex", "auth.json")
		// Check if it already exists in the template/agent home
		if _, err := os.Stat(dst); err == nil {
			return nil
		}

		if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
			return err
		}
		if err := util.CopyFile(auth.CodexAuthFile, dst); err != nil {
			return fmt.Errorf("failed to copy codex auth file: %w", err)
		}
	}
	return nil
}

func (c *Codex) GetVolumes(unixUsername string, auth api.AuthConfig) []api.VolumeMount {
	return nil
}

func (c *Codex) DefaultConfigDir() string {
	return ""
}

func (c *Codex) HasSystemPrompt(agentHome string) bool {
	return false
}

func (c *Codex) Provision(ctx context.Context, agentName, agentHome, agentWorkspace string) error {
	auth := c.DiscoverAuth(agentHome)
	return c.PropagateFiles(agentHome, "", auth)
}

func (c *Codex) GetEmbedDir() string {
	return "codex"
}
