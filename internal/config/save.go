package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/chetanparab/goli/internal/secret"
)

// Keys is where goli keeps secrets such as an API key (see package secret).
func (c *Config) Keys() secret.Store { return secret.Store{Dir: c.dir} }

// Path is config.yaml's location.
func (c *Config) Path() string { return filepath.Join(c.dir, "config.yaml") }

// SetAI chooses how AI search is powered and saves it to config.yaml, keeping
// the file's comments and every other setting. provider is "off",
// "claude-code" or "anthropic"; an empty model means the provider's default.
func (c *Config) SetAI(provider, model string) error {
	enabled := provider != "off"
	data, err := os.ReadFile(c.Path())
	if errors.Is(err, os.ErrNotExist) {
		data = nil
	} else if err != nil {
		return err
	}
	var doc yaml.Node
	if len(bytes.TrimSpace(data)) > 0 {
		if err := yaml.Unmarshal(data, &doc); err != nil {
			return fmt.Errorf("parse %s: %w", c.Path(), err)
		}
	}
	if doc.Kind == 0 || len(doc.Content) == 0 {
		doc = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode}}}
	}
	root := doc.Content[0]
	ai := mapChild(root, "ai")
	setScalar(ai, "enabled", fmt.Sprint(enabled), "!!bool")
	if enabled {
		setScalar(ai, "provider", provider, "!!str")
	}
	if model == "" {
		deleteKey(ai, "model")
	} else {
		setScalar(ai, "model", model, "!!str")
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return err
	}
	tmp := c.Path() + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, c.Path()); err != nil {
		return err
	}
	c.AI.Enabled = enabled
	if enabled {
		c.AI.Provider = provider
	}
	c.AI.Model = model
	return nil
}

// mapChild returns the mapping under key, creating it if needed.
func mapChild(m *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			if m.Content[i+1].Kind != yaml.MappingNode {
				m.Content[i+1] = &yaml.Node{Kind: yaml.MappingNode}
			}
			return m.Content[i+1]
		}
	}
	child := &yaml.Node{Kind: yaml.MappingNode}
	m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: key}, child)
	return child
}

func setScalar(m *yaml.Node, key, value, tag string) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			v := m.Content[i+1]
			v.Kind, v.Tag, v.Value, v.Style = yaml.ScalarNode, tag, value, 0
			v.Content = nil
			return
		}
	}
	m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: key},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: tag, Value: value})
}

func deleteKey(m *yaml.Node, key string) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			m.Content = append(m.Content[:i], m.Content[i+2:]...)
			return
		}
	}
}
