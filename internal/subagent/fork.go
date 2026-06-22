package subagent

import (
	"strings"

	"PseudoClaude/internal/llm"
)

const ForkBoilerplateTag = "<pseudoclaude-fork-subagent>"

const ForkBoilerplate = ForkBoilerplateTag + `
You are running as a forked sub Agent. Follow these constraints:
- Do not start or delegate to another Agent.
- Do not ask the user follow-up questions.
- Focus only on the assigned task.
- Keep the final result concise and actionable.
</pseudoclaude-fork-subagent>`

func ForkDefinition() Definition {
	return Definition{
		Name:         "__fork__",
		Description:  "Forked sub Agent that inherits the parent conversation and runs in the background.",
		Model:        ModelInherit,
		Permission:   PermissionInherit,
		Background:   true,
		SystemPrompt: "Forked sub Agent.",
		Source:       SourceBuiltin,
	}
}

func BuildForkMessages(parent []llm.Message, task string) []llm.Message {
	messages := cloneMessages(parent)
	messages = ensureNoDanglingToolCalls(messages)
	content := strings.TrimSpace(ForkBoilerplate) + "\n\nTask:\n" + strings.TrimSpace(task)
	messages = append(messages, llm.Message{Role: "user", Content: content})
	return messages
}

func IsForkMessages(messages []llm.Message) bool {
	for _, msg := range messages {
		if strings.Contains(msg.Content, ForkBoilerplateTag) {
			return true
		}
		if msg.ToolResult != nil && strings.Contains(msg.ToolResult.Content, ForkBoilerplateTag) {
			return true
		}
	}
	return false
}

func cloneMessages(messages []llm.Message) []llm.Message {
	out := make([]llm.Message, len(messages))
	for i, msg := range messages {
		out[i] = msg
		out[i].ToolCalls = append([]llm.ToolCall(nil), msg.ToolCalls...)
		if msg.ToolResult != nil {
			result := *msg.ToolResult
			out[i].ToolResult = &result
		}
	}
	return out
}

func ensureNoDanglingToolCalls(messages []llm.Message) []llm.Message {
	if len(messages) == 0 {
		return messages
	}
	last := messages[len(messages)-1]
	if last.Role != "assistant" || len(last.ToolCalls) == 0 {
		return messages
	}
	for _, call := range last.ToolCalls {
		messages = append(messages, llm.Message{
			Role: "user",
			ToolResult: &llm.ToolResult{
				CallID:  call.ID,
				Name:    call.Name,
				Content: "tool result unavailable in forked context",
				IsError: true,
			},
		})
	}
	return messages
}
