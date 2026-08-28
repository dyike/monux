package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/dyike/monux/internal/monitor"
	"gopkg.in/yaml.v3"
)

type MonitorConfig struct {
	ID string `yaml:"id"`
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

// Save validates and atomically writes a configuration. Input values are kept
// in hexadecimal so the generated file matches the VCP values shown by the
// CLI and monitor documentation.
func Save(path string, cfg Config) error {
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("validate config %q: %w", path, err)
	}

	data, err := marshal(cfg)
	if err != nil {
		return fmt.Errorf("encode config %q: %w", path, err)
	}
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return fmt.Errorf("create config directory %q: %w", directory, err)
	}

	mode := os.FileMode(0o600)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect config %q: %w", path, err)
	}

	temporary, err := os.CreateTemp(directory, ".monux-config-*")
	if err != nil {
		return fmt.Errorf("create temporary config in %q: %w", directory, err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)

	if err := temporary.Chmod(mode); err != nil {
		temporary.Close()
		return fmt.Errorf("set permissions on temporary config: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return fmt.Errorf("write temporary config: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary config: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("replace config %q: %w", path, err)
	}
	return nil
}

func marshal(cfg Config) ([]byte, error) {
	root := &yaml.Node{Kind: yaml.MappingNode}
	root.Content = append(root.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Value: "monitor"},
		&yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{
			{Kind: yaml.ScalarNode, Value: "id"},
			{Kind: yaml.ScalarNode, Tag: "!!str", Value: cfg.Monitor.ID, Style: yaml.DoubleQuotedStyle},
		}},
		&yaml.Node{Kind: yaml.ScalarNode, Value: "inputs"},
	)

	names := make([]string, 0, len(cfg.Inputs))
	for name := range cfg.Inputs {
		names = append(names, name)
	}
	sort.Strings(names)
	inputs := &yaml.Node{Kind: yaml.MappingNode}
	for _, name := range names {
		inputs.Content = append(inputs.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: name},
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: cfg.Inputs[name].String()},
		)
	}
	root.Content = append(root.Content, inputs)

	document := &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{root}}
	var output bytes.Buffer
	encoder := yaml.NewEncoder(&output)
	encoder.SetIndent(2)
	if err := encoder.Encode(document); err != nil {
		return nil, err
	}
	if err := encoder.Close(); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func (c Config) Validate() error {
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
