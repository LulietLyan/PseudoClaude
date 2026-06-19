package command

import (
	"fmt"
	"strings"
)

func FormatHelp(commands []Command) string {
	if len(commands) == 0 {
		return "No commands are registered."
	}
	maxName := 0
	for _, cmd := range commands {
		if len(cmd.Name) > maxName {
			maxName = len(cmd.Name)
		}
	}
	lines := []string{"Available commands:", ""}
	lines = append(lines, fmt.Sprintf("  %-*s  %-12s  %s", maxName, "Command", "Usage", "Description"))
	for _, cmd := range commands {
		lines = append(lines, fmt.Sprintf("  %-*s  %-12s  %s", maxName, cmd.Name, cmd.Usage, cmd.Description))
	}
	return strings.Join(lines, "\n")
}

func FormatHelpHint() string {
	return "Need help? Type /help to view available commands, usage, and descriptions."
}

func FormatStatus(info StatusInfo) string {
	return strings.Join([]string{
		"Status",
		"Work mode: " + string(info.WorkMode),
		"Permission mode: " + valueOrEmpty(info.PermissionMode),
		"Model: " + valueOrEmpty(info.Model),
		fmt.Sprintf("Tokens: input %d, output %d, total %d", info.Usage.InputTokens, info.Usage.OutputTokens, info.Usage.TotalTokens),
		"Session: " + valueOrEmpty(info.SessionID),
		"CWD: " + valueOrEmpty(info.CWD),
		"Runtime state: " + valueOrEmpty(info.RuntimeState),
	}, "\n")
}

func FormatSession(info SessionInfo) string {
	return strings.Join([]string{
		"Session",
		"ID: " + valueOrEmpty(info.ID),
		"JSONL path: " + valueOrEmpty(info.JSONLPath),
		fmt.Sprintf("Messages: %d", info.MessageCount),
		"Model: " + valueOrEmpty(info.Model),
	}, "\n")
}

func FormatMemory(summary string) string {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return "暂无长期记忆。"
	}
	return "Agent Memory\n\n" + summary
}

func valueOrEmpty(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "(none)"
	}
	return value
}
