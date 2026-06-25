package config

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Providers []ProviderConfig `yaml:"providers"`
	Features  FeatureConfig    `yaml:"features"`
}

type FeatureConfig struct {
	CoordinatorMode bool `yaml:"coordinator_mode"`
	ForkTeammate    bool `yaml:"fork_teammate"`
}

type ProviderConfig struct {
	Name          string `yaml:"name"`
	Protocol      string `yaml:"protocol"`
	BaseURL       string `yaml:"base_url"`
	APIKey        string `yaml:"api_key"`
	Model         string `yaml:"model"`
	Thinking      bool   `yaml:"thinking"`
	ContextWindow int64  `yaml:"context_window"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("配置文件不存在: %s", path)
		}
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("解析 YAML 配置失败: %w", err)
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c Config) validate() error {
	if len(c.Providers) == 0 {
		return errors.New("providers 不能为空")
	}

	for i, p := range c.Providers {
		prefix := fmt.Sprintf("providers[%d]", i)
		if strings.TrimSpace(p.Name) == "" {
			return fmt.Errorf("%s.name 不能为空", prefix)
		}
		if strings.TrimSpace(p.Protocol) == "" {
			return fmt.Errorf("%s.protocol 不能为空", prefix)
		}
		if p.Protocol != "anthropic" && p.Protocol != "openai" {
			return fmt.Errorf("%s.protocol 必须是 anthropic 或 openai", prefix)
		}
		if strings.TrimSpace(p.APIKey) == "" {
			return fmt.Errorf("%s.api_key 不能为空", prefix)
		}
		if strings.TrimSpace(p.Model) == "" {
			return fmt.Errorf("%s.model 不能为空", prefix)
		}
		if p.ContextWindow < 0 {
			return fmt.Errorf("%s.context_window 不能为负数", prefix)
		}
	}

	return nil
}

func (p ProviderConfig) EffectiveContextWindow() int64 {
	if p.ContextWindow > 0 {
		return p.ContextWindow
	}
	switch p.Protocol {
	case "anthropic":
		return 200000
	case "openai":
		return 128000
	default:
		return 128000
	}
}
