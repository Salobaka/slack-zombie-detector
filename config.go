package main

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type Channel struct {
	ID   string `yaml:"id"`
	Name string `yaml:"name"`
}

type Config struct {
	SlackToken      string    `yaml:"slack_token"`
	UserToken       string    `yaml:"user_token"`
	Channels        []Channel `yaml:"channels"`
	ReportRecipient string    `yaml:"report_recipient"`
	Whitelist       []string  `yaml:"whitelist"`
	RoyalMembers    []string  `yaml:"royal_members"`
}

func (c *Config) IsRoyal(userID, displayName string) bool {
	for _, entry := range c.RoyalMembers {
		if entry == userID {
			return true
		}
		if strings.EqualFold(entry, displayName) {
			return true
		}
	}
	return false
}

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	if cfg.SlackToken == "" {
		return nil, fmt.Errorf("slack_token is required")
	}
	if len(cfg.Channels) == 0 {
		return nil, fmt.Errorf("at least one channel is required")
	}
	if cfg.ReportRecipient == "" {
		return nil, fmt.Errorf("report_recipient is required")
	}

	return &cfg, nil
}

func (c *Config) IsWhitelisted(userID, displayName string) bool {
	for _, entry := range c.Whitelist {
		if entry == userID {
			return true
		}
		if strings.EqualFold(entry, displayName) {
			return true
		}
	}
	return false
}
