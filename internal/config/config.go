package config

import (
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

type FleetConfig struct {
	Project ProjectConfig `toml:"project"`
	Claude  ClaudeConfig  `toml:"claude"`
	Agents  []AgentConfig `toml:"agents"`
}

type ProjectConfig struct {
	Name     string `toml:"name"`
	RelayURL string `toml:"relay_url"`
	Cwd      string `toml:"cwd"`
}

type ClaudeConfig struct {
	Flags []string `toml:"flags"`
}

type AgentConfig struct {
	Name        string `toml:"name"`
	Color       string `toml:"color"`
	Role        string `toml:"role"`
	ReportsTo   string `toml:"reports_to,omitempty"`
	IsExecutive bool   `toml:"is_executive,omitempty"`
}

func FleetDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".fleet")
}

func Save(path string, cfg *FleetConfig) error {
	os.MkdirAll(filepath.Dir(path), 0755)
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(cfg)
}

func Load(path string) (*FleetConfig, error) {
	var cfg FleetConfig
	_, err := toml.DecodeFile(path, &cfg)
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

func SaveAsLast(cfg *FleetConfig) error {
	dir := FleetDir()
	configDir := filepath.Join(dir, "configs")
	os.MkdirAll(configDir, 0755)

	configPath := filepath.Join(configDir, cfg.Project.Name+".toml")
	if err := Save(configPath, cfg); err != nil {
		return err
	}

	lastPath := filepath.Join(dir, "last.toml")
	os.Remove(lastPath)
	return os.Symlink(configPath, lastPath)
}

func LoadLast() (*FleetConfig, error) {
	return Load(filepath.Join(FleetDir(), "last.toml"))
}
