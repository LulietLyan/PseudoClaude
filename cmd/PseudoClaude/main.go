package main

import (
	"fmt"
	"os"

	"PseudoClaude/internal/config"
	"PseudoClaude/internal/permission"
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

	if err := tui.New(cfg.Providers, cwd, registry, permissionEngine).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "运行错误: %v\n", err)
		os.Exit(1)
	}
}
