package coordinator

import (
	"os"
	"strings"

	"PseudoClaude/internal/config"
)

const EnvVar = "PSEUDOCLAUDE_COORDINATOR_MODE"

func IsEnabled(cfg *config.Config) bool {
	if cfg == nil || !cfg.Features.CoordinatorMode {
		return false
	}
	return truthy(os.Getenv(EnvVar))
}

func AllowedTools() []string {
	return []string{
		"Agent",
		"TeamCreate",
		"TeamDelete",
		"TeamKill",
		"TaskCreate",
		"TaskUpdate",
		"TaskList",
		"TaskGet",
		"SendMessage",
		"find_files",
		"load_skill",
		"read_file",
		"run_command",
		"search_code",
	}
}

func SystemPromptSuffix() string {
	return strings.TrimSpace(`
Coordinator mode is active. Act as a team lead: decompose the user's goal, create or use a team, assign work to teammates, wait for reports, synthesize results, verify with shell commands, and merge completed work. Do not directly edit files; delegate implementation to teammates and use shell for inspection, tests, Git merge, conflict handling, and rollback.
`)
}

func truthy(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
