package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Template struct {
	Name string
	Path string
}

type GswarmConfig struct {
	Image    string `json:"image"`
	Detached *bool  `json:"detached"`
	UseTmux  bool   `json:"use_tmux"`
}

func (c *GswarmConfig) IsDetached() bool {
	if c.Detached == nil {
		return true
	}
	return *c.Detached
}

func (t *Template) LoadConfig() (*GswarmConfig, error) {
	path := filepath.Join(t.Path, "gswarm.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &GswarmConfig{}, nil
		}
		return nil, err
	}

	var cfg GswarmConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func FindTemplate(name string) (*Template, error) {
	// 1. Check project-local templates
	projectTemplatesDir, err := GetProjectTemplatesDir()
	if err == nil {
		path := filepath.Join(projectTemplatesDir, name)
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			return &Template{Name: name, Path: path}, nil
		}
	}

	// 2. Check global templates
	globalTemplatesDir, err := GetGlobalTemplatesDir()
	if err == nil {
		path := filepath.Join(globalTemplatesDir, name)
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			return &Template{Name: name, Path: path}, nil
		}
	}

	return nil, fmt.Errorf("template %s not found", name)
}

// GetTemplateChain returns a list of templates in inheritance order (base first)
func GetTemplateChain(name string) ([]*Template, error) {
	var chain []*Template

	// Always start with default if it's not the requested template
	if name != "default" {
		def, err := FindTemplate("default")
		if err == nil {
			chain = append(chain, def)
		}
	}

	tpl, err := FindTemplate(name)
	if err != nil {
		return nil, err
	}
	chain = append(chain, tpl)

	return chain, nil
}
