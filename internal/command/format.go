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
		description := cmd.Description
		if cmd.Skill {
			description += " [skill]"
		}
		lines = append(lines, fmt.Sprintf("  %-*s  %-12s  %s", maxName, cmd.Name, cmd.Usage, description))
	}
	return strings.Join(lines, "\n")
}

func FormatSkills(skills []SkillSummary) string {
	if len(skills) == 0 {
		return "No skills loaded."
	}
	lines := []string{fmt.Sprintf("Available skills (%d):", len(skills)), ""}
	for _, skill := range skills {
		lines = append(lines, fmt.Sprintf("  /%s  %s", skill.Name, skill.Description))
	}
	lines = append(lines, "", "Run /<skill-name> [arguments] to invoke a skill.")
	return strings.Join(lines, "\n")
}

func FormatHooks(hooks []HookSummary, sources []string) string {
	if len(hooks) == 0 {
		return "No hooks loaded."
	}
	lines := []string{fmt.Sprintf("Loaded hooks (%d):", len(hooks)), ""}
	currentEvent := ""
	for _, hook := range hooks {
		if hook.Event != currentEvent {
			if currentEvent != "" {
				lines = append(lines, "")
			}
			currentEvent = hook.Event
			lines = append(lines, currentEvent+":")
		}
		flags := ""
		if len(hook.Flags) > 0 {
			flags = " [" + strings.Join(hook.Flags, ", ") + "]"
		}
		source := valueOrEmpty(hook.Source)
		lines = append(lines, fmt.Sprintf("  %s  %s%s  %s", hook.Name, hook.Action, flags, source))
	}
	if len(sources) > 0 {
		lines = append(lines, "", "Loaded from:")
		for _, source := range sources {
			lines = append(lines, "  "+source)
		}
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
