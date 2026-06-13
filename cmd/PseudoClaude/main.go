package main

import (
	"fmt"
	"os"

	"PseudoClaude/internal/config"
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

	if err := tui.New(cfg.Providers, cwd).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "运行错误: %v\n", err)
		os.Exit(1)
	}
}
