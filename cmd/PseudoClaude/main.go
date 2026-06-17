package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"PseudoClaude/internal/config"
	"PseudoClaude/internal/instructions"
	"PseudoClaude/internal/mcp"
	"PseudoClaude/internal/memory"
	"PseudoClaude/internal/permission"
	"PseudoClaude/internal/session"
	"PseudoClaude/internal/tools"
	"PseudoClaude/internal/tui"
)

func main() {
	cfg, err := config.Load(".PseudoClaude/config.yaml")
	if err != nil {
		fmt.Fprintf(os.Stderr, "配置错误: %v\n", err)
		os.Exit(1)
	}

	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}
	home, _ := os.UserHomeDir()
	instructionResult := instructions.NewLoader(cwd).Load()
	memoryManager := memory.NewManager(memory.DefaultProjectDir(cwd), memory.DefaultUserDir(home))
	memoryManager.RefreshIndex()
	go func() {
		for _, err := range session.CleanExpired(cwd, time.Now()) {
			fmt.Fprintf(os.Stderr, "会话清理提示: %v\n", err)
		}
	}()
	permissionEngine, err := permission.NewEngine(cwd, permission.DefaultOptions(cwd))
	if err != nil {
		fmt.Fprintf(os.Stderr, "权限系统初始化错误: %v\n", err)
		os.Exit(1)
	}
	for _, issue := range permissionEngine.LoadIssues() {
		if issue.Path != "" {
			fmt.Fprintf(os.Stderr, "权限配置提示: %s: %s\n", issue.Path, issue.Message)
		} else {
			fmt.Fprintf(os.Stderr, "权限配置提示: %s\n", issue.Message)
		}
	}

	registry, err := tools.DefaultRegistry()
	if err != nil {
		fmt.Fprintf(os.Stderr, "工具注册错误: %v\n", err)
		os.Exit(1)
	}

	mcpCfg, loadIssues := mcp.LoadConfig(cwd)
	for _, issue := range loadIssues {
		printMCPLoadIssue(issue)
	}
	mcpManager := mcp.NewManager(context.Background(), mcpCfg, mcp.ManagerOptions{
		ClientInfo: mcp.ClientInfo{Name: "PseudoClaude", Version: "dev"},
	})
	defer mcpManager.Close()
	for _, issue := range mcpManager.Issues() {
		printMCPIssue(issue)
	}
	mcpStats := mcpManager.Stats()
	for _, tool := range mcpManager.Tools() {
		if err := registry.Register(tool); err != nil {
			fmt.Fprintf(os.Stderr, "MCP 工具注册提示: %s: %v\n", tool.Definition().Name, err)
			continue
		}
		mcpStats.Registered++
	}
	mcpStatus := fmt.Sprintf("MCP: %d/%d connected, %d registered",
		mcpStats.Connected, mcpStats.Configured, mcpStats.Registered)

	var startup []string
	startup = append(startup, mcpStatus)
	if len(instructionResult.Loaded) > 0 {
		startup = append(startup, fmt.Sprintf("Instructions: %d loaded", len(instructionResult.Loaded)))
	}
	for _, warning := range instructionResult.Warnings {
		startup = append(startup, "Instruction warning: "+warning)
	}
	if filepath.IsAbs(memory.DefaultProjectDir(cwd)) {
		startup = append(startup, "Memory: index loaded")
	}
	model := tui.New(cfg.Providers, cwd, registry, permissionEngine).
		WithPersistentContext(instructionResult.Content, memoryManager).
		WithStartupStatus(startup...)
	if err := model.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "运行错误: %v\n", err)
		os.Exit(1)
	}
}

func printMCPLoadIssue(issue mcp.LoadIssue) {
	switch {
	case issue.Path != "" && issue.Server != "":
		fmt.Fprintf(os.Stderr, "MCP 配置提示: %s: %s: %s\n", issue.Path, issue.Server, issue.Message)
	case issue.Path != "":
		fmt.Fprintf(os.Stderr, "MCP 配置提示: %s: %s\n", issue.Path, issue.Message)
	case issue.Server != "":
		fmt.Fprintf(os.Stderr, "MCP 配置提示: %s: %s\n", issue.Server, issue.Message)
	default:
		fmt.Fprintf(os.Stderr, "MCP 配置提示: %s\n", issue.Message)
	}
}

func printMCPIssue(issue mcp.Issue) {
	tool := ""
	if issue.Tool != "" {
		tool = ": " + issue.Tool
	}
	fmt.Fprintf(os.Stderr, "MCP 连接提示: %s: %s%s: %s\n", issue.Server, issue.Stage, tool, issue.Message)
}
