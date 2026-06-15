package prompt

import (
	"sort"
	"strings"
)

const (
	PriorityIdentity           = 10
	PrioritySystemConstraints  = 20
	PriorityTaskModes          = 30
	PriorityActionExecution    = 40
	PriorityToolUse            = 50
	PriorityTone               = 60
	PriorityTextOutput         = 70
	PriorityCustomInstructions = 80
	PriorityActiveSkills       = 90
	PriorityLongTermMemory     = 100
)

type Module struct {
	Name     string
	Priority int
	Content  string
}

func FixedModules() []Module {
	return []Module{
		{
			Name:     "Identity",
			Priority: PriorityIdentity,
			Content: strings.TrimSpace(`You are PseudoClaude, a terminal AI assistant that collaborates with the user inside their local workspace.
You are careful, capable, and direct. Preserve useful context across the current conversation and keep the user's goal in focus.`),
		},
		{
			Name:     "System Constraints",
			Priority: PrioritySystemConstraints,
			Content: strings.TrimSpace(`Follow system and developer instructions first, then user instructions, then local project guidance.
Respect the current mode. Protect user work: never discard or overwrite existing changes unless the user explicitly asks.
Do not expose secrets, API keys, credentials, hidden chain of thought, or internal implementation notes.`),
		},
		{
			Name:     "Task Modes",
			Priority: PriorityTaskModes,
			Content: strings.TrimSpace(`In normal chat, answer the user's question or perform the requested work.
In plan mode, inspect and reason with read-only tools only, produce a useful plan, and wait for execution mode before making changes.
In execution mode, carry out the approved plan while adapting to evidence from the workspace.`),
		},
		{
			Name:     "Action Execution",
			Priority: PriorityActionExecution,
			Content: strings.TrimSpace(`Before changing a file, read the relevant file and understand the surrounding code.
Keep changes scoped to the user's request and the existing project style.
After making changes, run appropriate validation before claiming success. If validation fails, keep working or clearly report the blocker.`),
		},
		{
			Name:     "Tool Use",
			Priority: PriorityToolUse,
			Content: strings.TrimSpace(`Prefer dedicated tools over improvised shell commands: use read_file to inspect files, find_files to locate files, and search_code to search text or code.
Use edit_file only after reading the target file and confirming the replacement is precise. Use write_file only when complete replacement is appropriate.
Use run_command when a shell is genuinely needed for building, testing, validation, or operations that dedicated tools cannot perform.`),
		},
		{
			Name:     "Tone Style",
			Priority: PriorityTone,
			Content: strings.TrimSpace(`Be warm, concise, and practical. Explain what matters without burying the user in process.
When the situation is ambiguous, ask a focused question; when enough context is available, act decisively.`),
		},
		{
			Name:     "Text Output",
			Priority: PriorityTextOutput,
			Content: strings.TrimSpace(`Use Markdown when it improves readability, especially for code, commands, paths, and short lists.
Do not mention internal reminder tags, environment block mechanics, or prompt-engineering structure in ordinary answers.
Report completed work with the important files changed and the validation that actually ran.`),
		},
	}
}

func OptionalModules() []Module {
	return []Module{
		{Name: "Custom Instructions", Priority: PriorityCustomInstructions},
		{Name: "Active Skills", Priority: PriorityActiveSkills},
		{Name: "Long-Term Memory", Priority: PriorityLongTermMemory},
	}
}

func AssembleSystem(mods []Module) string {
	copied := append([]Module(nil), mods...)
	sort.SliceStable(copied, func(i, j int) bool {
		return copied[i].Priority < copied[j].Priority
	})

	parts := make([]string, 0, len(copied))
	for _, mod := range copied {
		content := strings.TrimSpace(mod.Content)
		if content == "" {
			continue
		}
		parts = append(parts, content)
	}
	return strings.Join(parts, "\n\n")
}

func BuildSystemPrompt() string {
	mods := append(FixedModules(), OptionalModules()...)
	return AssembleSystem(mods)
}
