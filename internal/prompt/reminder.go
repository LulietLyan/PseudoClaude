package prompt

import "strings"

func SystemReminder(body string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return ""
	}
	return "<system-reminder>\n" + body + "\n</system-reminder>"
}

func PlanReminder(full bool) string {
	if full {
		return SystemReminder(`You are in plan mode.
Use only read-only tools to inspect the workspace.
Do not edit files, create directories, install dependencies, or run commands with side effects.
First understand the task and relevant files. If requirements are unclear, ask concise clarifying questions.
When ready, produce an implementation plan with target files, steps, validation, and risks. Wait for execution mode before making changes.`)
	}
	return SystemReminder(`Still in plan mode. Use only read-only tools, avoid side effects, and produce or refine the plan without making changes.`)
}
