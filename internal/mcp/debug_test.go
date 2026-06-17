package mcp

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestDebugProjectMCPConfig(t *testing.T) {
	if os.Getenv("PSEUDOCLAUDE_DEBUG_MCP") != "1" {
		t.Skip("set PSEUDOCLAUDE_DEBUG_MCP=1 to connect configured MCP servers")
	}
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root = "../.."
	cfg, loadIssues := LoadConfig(root)
	for _, issue := range loadIssues {
		t.Logf("load issue: path=%s server=%s message=%s", issue.Path, issue.Server, issue.Message)
	}
	manager := NewManager(context.Background(), cfg, ManagerOptions{ConnectTimeout: 60 * time.Second})
	defer manager.Close()
	t.Logf("stats: %+v", manager.Stats())
	for _, issue := range manager.Issues() {
		t.Logf("manager issue: server=%s tool=%s stage=%s message=%s", issue.Server, issue.Tool, issue.Stage, issue.Message)
	}
	for _, tool := range manager.Tools() {
		def := tool.Definition()
		t.Logf("tool: %s safety=%s", def.Name, def.Safety)
	}
	if len(manager.Tools()) == 0 {
		t.Fatal("no MCP tools discovered")
	}
}
