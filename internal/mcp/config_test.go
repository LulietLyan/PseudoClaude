package mcp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigMergesAndOverrides(t *testing.T) {
	home := t.TempDir()
	root := t.TempDir()
	t.Setenv("HOME", home)
	writeConfig(t, filepath.Join(home, ".PseudoClaude", "config.yaml"), `
mcp_servers:
  shared:
    type: stdio
    command: user-cmd
  user_only:
    type: http
    url: http://user.example/mcp
`)
	writeConfig(t, filepath.Join(root, ".PseudoClaude", "config.yaml"), `
mcp_servers:
  shared:
    type: http
    url: http://project.example/mcp
  project_only:
    type: stdio
    command: project-cmd
    read_only: true
`)

	cfg, issues := LoadConfig(root)
	if len(issues) != 0 {
		t.Fatalf("issues = %+v", issues)
	}
	if got := cfg.Servers["shared"]; got.Type != TransportHTTP || got.URL != "http://project.example/mcp" || got.Command != "" {
		t.Fatalf("shared = %+v", got)
	}
	if _, ok := cfg.Servers["user_only"]; !ok {
		t.Fatal("missing user_only")
	}
	if _, ok := cfg.Servers["project_only"]; !ok {
		t.Fatal("missing project_only")
	}
	if !cfg.Servers["project_only"].ReadOnly {
		t.Fatal("read_only flag not loaded")
	}
}

func TestLoadConfigSkipsInvalidLayer(t *testing.T) {
	home := t.TempDir()
	root := t.TempDir()
	t.Setenv("HOME", home)
	writeConfig(t, filepath.Join(home, ".PseudoClaude", "config.yaml"), `mcp_servers: [`)
	writeConfig(t, filepath.Join(root, ".PseudoClaude", "config.yaml"), `
mcp_servers:
  ok:
    type: stdio
    command: demo
`)
	cfg, issues := LoadConfig(root)
	if len(issues) == 0 {
		t.Fatal("expected issue")
	}
	if _, ok := cfg.Servers["ok"]; !ok {
		t.Fatal("valid project config should survive invalid user config")
	}
}

func TestExpandVarsBoundaries(t *testing.T) {
	home := t.TempDir()
	root := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("TOKEN", "secret")
	writeConfig(t, filepath.Join(root, ".PseudoClaude", "config.yaml"), `
mcp_servers:
  ${SERVER}:
    type: http
    url: http://${HOST}/mcp
    headers:
      Authorization: Bearer ${TOKEN}
      Missing: ${MISSING}
  local:
    type: stdio
    command: ${COMMAND}
    args: ["${ARG}"]
    env:
      TOKEN: ${TOKEN}
`)
	cfg, issues := LoadConfig(root)
	if got := cfg.Servers["${SERVER}"].Headers["Authorization"]; got != "Bearer secret" {
		t.Fatalf("header = %q", got)
	}
	if got := cfg.Servers["${SERVER}"].Headers["Missing"]; got != "" {
		t.Fatalf("missing env expansion = %q", got)
	}
	if got := cfg.Servers["${SERVER}"].URL; got != "http://${HOST}/mcp" {
		t.Fatalf("url expanded unexpectedly: %q", got)
	}
	if got := cfg.Servers["local"].Command; got != "${COMMAND}" {
		t.Fatalf("command expanded unexpectedly: %q", got)
	}
	if got := cfg.Servers["local"].Args[0]; got != "${ARG}" {
		t.Fatalf("arg expanded unexpectedly: %q", got)
	}
	if got := cfg.Servers["local"].Env["TOKEN"]; got != "secret" {
		t.Fatalf("env = %q", got)
	}
	if len(issues) != 1 || issues[0].Server != "${SERVER}" {
		t.Fatalf("issues = %+v", issues)
	}
}

func TestValidateServer(t *testing.T) {
	cases := map[string]rawServer{
		"no_type":     {},
		"bad_type":    {Type: "sse"},
		"no_command":  {Type: "stdio"},
		"no_url":      {Type: "http"},
		"valid_stdio": {Type: "stdio", Command: "demo"},
		"valid_http":  {Type: "http", URL: "http://example/mcp"},
	}
	valid := 0
	for name, raw := range cases {
		_, _, ok := validateServer(name, raw)
		if ok {
			valid++
		}
	}
	if valid != 2 {
		t.Fatalf("valid count = %d, want 2", valid)
	}
}

func TestExampleConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GITHUB_PERSONAL_ACCESS_TOKEN", "placeholder")
	t.Setenv("MCP_HTTP_TOKEN", "placeholder")
	root := t.TempDir()
	data, err := os.ReadFile(filepath.Join("..", "..", "docs", "Part_6", "mcp-servers.example.yaml"))
	if err != nil {
		t.Skipf("example not present yet: %v", err)
	}
	writeConfig(t, filepath.Join(root, ".PseudoClaude", "config.yaml"), string(data))
	cfg, issues := LoadConfig(root)
	if len(issues) != 0 {
		t.Fatalf("issues = %+v", issues)
	}
	if len(cfg.Servers) < 2 {
		t.Fatalf("servers = %+v", cfg.Servers)
	}
}

func writeConfig(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
