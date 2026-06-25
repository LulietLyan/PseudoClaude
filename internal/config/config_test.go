package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadValidConfig(t *testing.T) {
	path := writeConfig(t, `
providers:
  - name: Claude
    protocol: anthropic
    api_key: sk-ant-test
    model: claude-test
    thinking: true
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if len(cfg.Providers) != 1 {
		t.Fatalf("provider count = %d, want 1", len(cfg.Providers))
	}
	if cfg.Providers[0].Name != "Claude" || !cfg.Providers[0].Thinking {
		t.Fatalf("provider not parsed correctly: %+v", cfg.Providers[0])
	}
	if cfg.Features.CoordinatorMode || cfg.Features.ForkTeammate {
		t.Fatalf("features should default to false: %+v", cfg.Features)
	}
}

func TestLoadFeatureConfig(t *testing.T) {
	path := writeConfig(t, `
providers:
  - name: Claude
    protocol: anthropic
    api_key: sk-ant-test
    model: claude-test
features:
  coordinator_mode: true
  fork_teammate: true
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if !cfg.Features.CoordinatorMode {
		t.Fatal("coordinator_mode was not parsed")
	}
	if !cfg.Features.ForkTeammate {
		t.Fatal("fork_teammate was not parsed")
	}
}

func TestLoadRejectsInvalidConfig(t *testing.T) {
	tests := map[string]string{
		"missing api key": `
providers:
  - name: Claude
    protocol: anthropic
    model: claude-test
`,
		"invalid protocol": `
providers:
  - name: Local
    protocol: other
    api_key: test
    model: model
`,
		"empty providers": `
providers: []
`,
		"negative context window": `
providers:
  - name: Local
    protocol: openai
    api_key: test
    model: model
    context_window: -1
`,
	}

	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := Load(writeConfig(t, body)); err == nil {
				t.Fatal("Load returned nil error")
			}
		})
	}
}

func TestEffectiveContextWindow(t *testing.T) {
	tests := []struct {
		name string
		cfg  ProviderConfig
		want int64
	}{
		{"explicit", ProviderConfig{Protocol: "openai", ContextWindow: 32000}, 32000},
		{"anthropic default", ProviderConfig{Protocol: "anthropic"}, 200000},
		{"openai default", ProviderConfig{Protocol: "openai"}, 128000},
		{"fallback default", ProviderConfig{Protocol: "other"}, 128000},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.cfg.EffectiveContextWindow(); got != tc.want {
				t.Fatalf("EffectiveContextWindow = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestLoadMissingFile(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "missing.yaml"))
	if err == nil {
		t.Fatal("Load returned nil error")
	}
	if !strings.Contains(err.Error(), "配置文件不存在") {
		t.Fatalf("error = %q, want missing-file message", err.Error())
	}
}

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}
