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
	Name           string `json:"identity"`
	KubeConfigPath string `json:"kubeconfig"`
	Context        string `json:"context"`
}

func (c *Config) GetVirtualGardenConfig(name string) (*Garden, error) {
	for _, g := range c.Gardens {
		if g.Name == name {
			return &g, nil
		}
	}
	return nil, fmt.Errorf("could not find any entry for Garden: %s", name)
}

func LoadGardenConfig() (*Config, error) {
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
	for _, g := range cfg.Gardens {
		expandedPath, err := homedir.Expand(g.KubeConfigPath)
		if err != nil {
			return nil, err
		}
		g.KubeConfigPath = expandedPath
	}
	return cfg, nil
}

func getGardenCtlBasePath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, ".Garden"), nil
}
