package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/dyike/monux/internal/monitor"
	"gopkg.in/yaml.v3"
)

type MonitorConfig struct {
	Bus int `yaml:"bus"`
}

type Config struct {
	Monitor MonitorConfig            `yaml:"monitor"`
	Inputs  map[string]monitor.Input `yaml:"inputs"`
}

func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config %q: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config %q: %w", path, err)
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, fmt.Errorf("validate config %q: %w", path, err)
	}
	return cfg, nil
}

func (c Config) Validate() error {
	if c.Monitor.Bus <= 0 {
		return errors.New("monitor.bus must be greater than zero")
	}
	if len(c.Inputs) == 0 {
		return errors.New("inputs must contain at least one named input")
	}
	for name := range c.Inputs {
		if name == "" {
			return errors.New("input name must not be empty")
		}
	}
	return nil
}

func DefaultPath() string {
	if path := os.Getenv("MONUX_CONFIG"); path != "" {
		return path
	}
	if dir, err := os.UserConfigDir(); err == nil {
		return filepath.Join(dir, "monux", "config.yaml")
	}
	return "config.yaml"
}
