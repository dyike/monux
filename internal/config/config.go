package config

import (
	"bytes"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/dyike/monux/internal/monitor"
	"gopkg.in/yaml.v3"
)

type MonitorConfig struct {
	ID string `yaml:"id"`
}

type NodeConfig struct {
	Name string `yaml:"name"`
}

type PeerConfig struct {
	Name  string `yaml:"name"`
	URL   string `yaml:"url"`
	Token string `yaml:"token,omitempty"`
}

type Config struct {
	Node    NodeConfig               `yaml:"node,omitempty"`
	Peers   []PeerConfig             `yaml:"peers,omitempty"`
	Monitor MonitorConfig            `yaml:"monitor"`
	Inputs  map[string]monitor.Input `yaml:"inputs"`
}

func (c *Config) UnmarshalYAML(value *yaml.Node) error {
	var raw struct {
		Node    NodeConfig           `yaml:"node"`
		Peers   []PeerConfig         `yaml:"peers"`
		Monitor MonitorConfig        `yaml:"monitor"`
		Inputs  map[string]yaml.Node `yaml:"inputs"`
	}
	if err := value.Decode(&raw); err != nil {
		return err
	}

	c.Node = raw.Node
	c.Peers = raw.Peers
	c.Monitor = raw.Monitor
	c.Inputs = make(map[string]monitor.Input, len(raw.Inputs))
	for name, node := range raw.Inputs {
		if node.Kind != yaml.ScalarNode {
			return fmt.Errorf("input %q must be a connector name or numeric VCP value", name)
		}
		input, err := monitor.ParseInput(node.Value)
		if err != nil {
			return fmt.Errorf("input %q: %w", name, err)
		}
		c.Inputs[name] = input
	}
	return nil
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

// Save validates and atomically writes a configuration. Standard inputs use
// readable connector names; vendor-specific inputs retain their raw VCP value.
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
	if cfg.Node.Name != "" {
		root.Content = append(root.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: "node"},
			mappingNode("name", cfg.Node.Name),
		)
	}
	if len(cfg.Peers) > 0 {
		peers := &yaml.Node{Kind: yaml.SequenceNode}
		for _, peer := range cfg.Peers {
			entry := &yaml.Node{Kind: yaml.MappingNode}
			appendScalar(entry, "name", peer.Name)
			appendScalar(entry, "url", peer.URL)
			if peer.Token != "" {
				appendScalar(entry, "token", peer.Token)
			}
			peers.Content = append(peers.Content, entry)
		}
		root.Content = append(root.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: "peers"},
			peers,
		)
	}
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
		input := cfg.Inputs[name]
		value := input.String()
		tag := "!!int"
		comment := ""
		if connector, ok := input.ConnectorKey(); ok {
			value = connector
			tag = "!!str"
			comment = "DDC " + input.String()
		}
		inputs.Content = append(inputs.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: name},
			&yaml.Node{Kind: yaml.ScalarNode, Tag: tag, Value: value, LineComment: comment},
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

func mappingNode(key, value string) *yaml.Node {
	node := &yaml.Node{Kind: yaml.MappingNode}
	appendScalar(node, key, value)
	return node
}

func appendScalar(node *yaml.Node, key, value string) {
	node.Content = append(node.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Value: key},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value},
	)
}

func (c Config) Validate() error {
	if len(c.Inputs) == 0 {
		return errors.New("inputs must contain at least one named input")
	}
	for name := range c.Inputs {
		if strings.TrimSpace(name) == "" {
			return errors.New("input name must not be empty")
		}
	}
	seenPeers := make(map[string]bool, len(c.Peers))
	for index, peer := range c.Peers {
		if strings.TrimSpace(peer.Name) == "" {
			return fmt.Errorf("peer %d name must not be empty", index+1)
		}
		if seenPeers[peer.Name] {
			return fmt.Errorf("peer name %q is duplicated", peer.Name)
		}
		seenPeers[peer.Name] = true
		parsed, err := url.Parse(peer.URL)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			return fmt.Errorf("peer %q URL %q must be an absolute http or https URL", peer.Name, peer.URL)
		}
		if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
			return fmt.Errorf("peer %q URL must not contain credentials, a query, or a fragment", peer.Name)
		}
	}
	return nil
}

func DefaultPath() string {
	if path := os.Getenv("MONUX_CONFIG"); path != "" {
		return path
	}
	home, _ := os.UserHomeDir()
	configDir, _ := os.UserConfigDir()
	return defaultPathForPlatform(runtime.GOOS, home, configDir)
}

func defaultPathForPlatform(platform, home, configDir string) string {
	if platform == "darwin" && home != "" {
		return filepath.Join(home, ".config", "monux", "config.yaml")
	}
	if configDir != "" {
		return filepath.Join(configDir, "monux", "config.yaml")
	}
	return "config.yaml"
}
