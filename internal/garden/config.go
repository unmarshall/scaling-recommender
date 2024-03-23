package garden

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/mitchellh/go-homedir"
	"gopkg.in/yaml.v2"
)

type Config struct {
	Gardens []Garden `json:"gardens"`
}
type Garden struct {
	Name           string `yaml:"identity"`
	KubeConfigPath string `yaml:"kubeconfig"`
	Context        string `yaml:"context"`
}

func (c *Config) getVirtualGardenConfig(name string) (*Garden, error) {
	for _, g := range c.Gardens {
		if g.Name == name {
			return &g, nil
		}
	}
	return nil, fmt.Errorf("could not find any entry for Garden: %s", name)
}

func loadGardenConfig() (*Config, error) {
	basePath, err := getGardenCtlBasePath()
	if err != nil {
		return nil, err
	}
	configPath := filepath.Join(basePath, "gardenctl-v2.yaml")
	if _, err = os.Stat(configPath); err != nil {
		return nil, err
	}
	cfgBytes, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}
	cfg := &Config{}
	if err = yaml.Unmarshal(cfgBytes, cfg); err != nil {
		return nil, err
	}
	for i, g := range cfg.Gardens {
		kubeConfigPath, err := homedir.Expand(g.KubeConfigPath)
		if err != nil {
			return nil, err
		}
		cfg.Gardens[i].KubeConfigPath = kubeConfigPath
	}
	return cfg, nil
}

func getGardenCtlBasePath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, ".garden"), nil
}
