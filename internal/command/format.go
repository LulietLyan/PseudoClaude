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

func FormatAgents(agents []AgentSummary) string {
	if len(agents) == 0 {
		return "No sub agents loaded."
	}
	lines := []string{fmt.Sprintf("Loaded sub agents (%d):", len(agents)), ""}
	for _, agent := range agents {
		limits := agentLimits(agent)
		if limits != "" {
			limits = "  " + limits
		}
		isolation := ""
		if strings.TrimSpace(agent.Isolation) != "" {
			isolation = " isolation=" + agent.Isolation
		}
		lines = append(lines, fmt.Sprintf("  %s  %s  source=%s model=%s maxTurns=%d background=%v%s%s",
			agent.Name,
			valueOrEmpty(agent.Description),
			valueOrEmpty(agent.Source),
			valueOrEmpty(agent.Model),
			agent.MaxTurns,
			agent.Background,
			isolation,
			limits,
		))
	}
	lines = append(lines, "", "Run /agents <name> for details or /agents reload to reload project and user agents.")
	return strings.Join(lines, "\n")
}

func FormatAgentDetail(detail AgentDetail) string {
	if detail.Active.Name == "" {
		return "Sub agent not found."
	}
	lines := []string{
		"Sub agent: " + detail.Active.Name,
		"Description: " + valueOrEmpty(detail.Active.Description),
		"Source: " + valueOrEmpty(detail.Active.Source),
		"Model: " + valueOrEmpty(detail.Active.Model),
		fmt.Sprintf("Max turns: %d", detail.Active.MaxTurns),
		fmt.Sprintf("Background: %v", detail.Active.Background),
	}
	if strings.TrimSpace(detail.Active.Isolation) != "" {
		lines = append(lines, "Isolation: "+detail.Active.Isolation)
	}
	if limits := agentLimits(detail.Active); limits != "" {
		lines = append(lines, "Limits: "+limits)
	}
	if len(detail.Overridden) > 0 {
		lines = append(lines, "", "Overridden sources:")
		for _, agent := range detail.Overridden {
			lines = append(lines, fmt.Sprintf("  %s  source=%s model=%s", agent.Name, valueOrEmpty(agent.Source), valueOrEmpty(agent.Model)))
		}
	}
	if strings.TrimSpace(detail.Prompt) != "" {
		lines = append(lines, "", "Prompt:", strings.TrimSpace(detail.Prompt))
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

func FormatWorktrees(items []WorktreeSummary) string {
	if len(items) == 0 {
		return "No worktrees."
	}
	lines := []string{fmt.Sprintf("Worktrees (%d):", len(items)), ""}
	for _, item := range items {
		lines = append(lines, "  "+strings.ReplaceAll(FormatWorktree(item), "\n", "\n  "))
	}
	return strings.Join(lines, "\n")
}

func FormatWorktree(item WorktreeSummary) string {
	flags := []string{}
	if item.Manual {
		flags = append(flags, "manual")
	}
	if item.Active {
		flags = append(flags, "active")
	}
	if item.Dirty {
		flags = append(flags, "dirty")
	}
	if item.Removed {
		flags = append(flags, "removed")
	}
	flagText := "(none)"
	if len(flags) > 0 {
		flagText = strings.Join(flags, ", ")
	}
	lines := []string{
		"Name: " + valueOrEmpty(item.Name),
		"Path: " + valueOrEmpty(item.Path),
		"Branch: " + valueOrEmpty(item.Branch),
		"Flags: " + flagText,
	}
	if item.DirtyError != "" {
		lines = append(lines, "Dirty reason: "+item.DirtyError)
	}
	return strings.Join(lines, "\n")
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

func agentLimits(agent AgentSummary) string {
	parts := []string{}
	if len(agent.Tools) > 0 {
		parts = append(parts, "tools="+strings.Join(agent.Tools, ","))
	}
	if len(agent.DisallowedTools) > 0 {
		parts = append(parts, "disallowed="+strings.Join(agent.DisallowedTools, ","))
	}
	return strings.Join(parts, " ")
}
