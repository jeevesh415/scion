package config

import (
	"fmt"
	"os"
	"path/filepath"
)

const DefaultSettingsJSON = `{
  "yolo": true,
  "security": {
    "auth": {
      "selectedType": "gemini-api-key"
    }
  },
	"telemetry": {
    "enabled": false
  },
	"general": {
    "disableAutoUpdate": true,
    "disableUpdateNag": true,
    "previewFeatures": true
  },
	"ui": {
    "accessibility": {
      "disableLoadingPhrases": true
    },
    "hideFooter": true,
    "hideWindowTitle": true
  }
}
`

const DefaultSystemPrompt = `# Scion Agent
You are a specialized agent working within a Scion.
`

const DefaultScionJSON = `{
  "image": "gemini-cli-sandbox",
  "use_tmux": true,
  "model": "flash"
}
`

const DefaultGeminiMD = `## Scion Context
`

const DefaultBashrc = `# scion agent bashrc
alias g="gemini"
`

func InitProject(targetDir string) error {
	var projectDir string
	var err error

	if targetDir != "" {
		projectDir = targetDir
	} else {
		projectDir, err = GetTargetProjectDir()
		if err != nil {
			return err
		}
	}

	templatesDir := filepath.Join(projectDir, "templates")
	defaultTemplateDir := filepath.Join(templatesDir, "default")
	agentsDir := filepath.Join(projectDir, "agents")

	// Create directories
	dirs := []string{
		projectDir,
		templatesDir,
		defaultTemplateDir,
		filepath.Join(defaultTemplateDir, ".gemini"),
		filepath.Join(defaultTemplateDir, ".config", "gcloud"),
		agentsDir,
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	// Seed default template files
	files := []struct {
		path    string
		content string
	}{
		{filepath.Join(defaultTemplateDir, "scion.json"), DefaultScionJSON},
		{filepath.Join(defaultTemplateDir, ".gemini", "settings.json"), DefaultSettingsJSON},
		{filepath.Join(defaultTemplateDir, ".gemini", "system_prompt.md"), DefaultSystemPrompt},
		{filepath.Join(defaultTemplateDir, "gemini.md"), DefaultGeminiMD},
		{filepath.Join(defaultTemplateDir, ".bashrc"), DefaultBashrc},
	}

	for _, f := range files {
		// Always write settings.json to ensure it matches current defaults
		if filepath.Base(f.path) == "settings.json" {
			if err := os.WriteFile(f.path, []byte(f.content), 0644); err != nil {
				return fmt.Errorf("failed to write file %s: %w", f.path, err)
			}
			continue
		}

		if _, err := os.Stat(f.path); os.IsNotExist(err) {
			if err := os.WriteFile(f.path, []byte(f.content), 0644); err != nil {
				return fmt.Errorf("failed to write file %s: %w", f.path, err)
			}
		}
	}

	return nil
}

func InitGlobal() error {
	globalDir, err := GetGlobalDir()
	if err != nil {
		return err
	}

	templatesDir := filepath.Join(globalDir, "templates")
	defaultTemplateDir := filepath.Join(templatesDir, "default")
	agentsDir := filepath.Join(globalDir, "agents")

	// Create directories
	dirs := []string{
		globalDir,
		templatesDir,
		defaultTemplateDir,
		filepath.Join(defaultTemplateDir, ".gemini"),
		agentsDir,
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create global directory %s: %w", dir, err)
		}
	}

	// Seed default template files for global as well
	files := []struct {
		path    string
		content string
	}{
		{filepath.Join(defaultTemplateDir, "scion.json"), DefaultScionJSON},
		{filepath.Join(defaultTemplateDir, ".gemini", "settings.json"), DefaultSettingsJSON},
		{filepath.Join(defaultTemplateDir, ".gemini", "system_prompt.md"), DefaultSystemPrompt},
		{filepath.Join(defaultTemplateDir, ".gemini", "gemini.md"), DefaultGeminiMD},
		{filepath.Join(defaultTemplateDir, ".bashrc"), DefaultBashrc},
	}

	for _, f := range files {
		// Always write settings.json to ensure it matches current defaults
		if filepath.Base(f.path) == "settings.json" {
			if err := os.WriteFile(f.path, []byte(f.content), 0644); err != nil {
				return fmt.Errorf("failed to write global file %s: %w", f.path, err)
			}
			continue
		}

		if _, err := os.Stat(f.path); os.IsNotExist(err) {
			if err := os.WriteFile(f.path, []byte(f.content), 0644); err != nil {
				return fmt.Errorf("failed to write global file %s: %w", f.path, err)
			}
		}
	}

	return nil
}
