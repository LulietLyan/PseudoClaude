package memory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"PseudoClaude/internal/llm"
)

func BuildUpdatePrompt(turn []llm.Message, projectIndex, userIndex string) []llm.Message {
	var b strings.Builder
	b.WriteString("You update long-term memory for a coding assistant. Do not call tools. Output only a JSON array of operations; no prose before or after it.\n")
	b.WriteString("Actions: create, update, delete. Levels: project or user. Types: user_preference, correction_feedback, project_knowledge, reference_material.\n")
	b.WriteString("Operation schema:\n")
	b.WriteString(`[{"action":"create|update|delete","level":"project|user","type":"user_preference|correction_feedback|project_knowledge|reference_material","title":"short title","summary":"one sentence","slug":"safe-slug-for-create","filename":"existing-file-for-update-or-delete.md","content":"markdown note body"}]`)
	b.WriteString("\n\n")
	b.WriteString("Memory policy:\n")
	b.WriteString("- Treat explicit user self-disclosure as durable user-level memory unless it is obviously temporary. This includes profession, expertise, education, graduation cohort, learning goals, language preferences, workflow preferences, and corrections about how the assistant should behave.\n")
	b.WriteString("- If the user says they are a Go engineer, have deep Go expertise, are a computer-science student, or want to learn AI Agent knowledge, create or update a user_preference note. Do not wait for the user to repeat or complain that it was not remembered.\n")
	b.WriteString("- When new self-disclosure extends an existing user profile, update the existing note instead of creating an unrelated duplicate. Prefer a single profile-style note titled \"User Profile\" with slug \"user-profile\" when no better existing note is present.\n")
	b.WriteString("- Use filenames shown in existing index lines, such as \"(...md)\", exactly in update/delete operations. If no suitable filename exists, use create with a stable slug.\n")
	b.WriteString("- Keep project facts and reference materials at project level. Keep cross-project user identity, expertise, learning goals, and style preferences at user level.\n")
	b.WriteString("- Return [] only when there is genuinely no stable fact, preference, correction, project knowledge, or reference material to save.\n\n")
	b.WriteString("[project index]\n")
	b.WriteString(projectIndex)
	b.WriteString("\n\n[user index]\n")
	b.WriteString(userIndex)
	b.WriteString("\n\n[recent turn]\n")
	for _, msg := range turn {
		b.WriteString("role=" + msg.Role + "\n")
		if msg.Content != "" {
			b.WriteString(msg.Content + "\n")
		}
	}
	return []llm.Message{{Role: "user", Content: b.String()}}
}

func collectJSONOperations(ctx context.Context, provider llm.Provider, messages []llm.Message) ([]Operation, error) {
	if provider == nil {
		return nil, nil
	}
	var b strings.Builder
	for event := range provider.Stream(ctx, llm.Request{Messages: messages}) {
		if event.Err != nil {
			return nil, event.Err
		}
		b.WriteString(event.Text)
		if event.Done {
			break
		}
	}
	raw := extractJSONArray(b.String())
	if raw == "" {
		return nil, errors.New("empty memory update response")
	}
	var ops []Operation
	if err := json.Unmarshal([]byte(raw), &ops); err != nil {
		return nil, err
	}
	return ops, nil
}

func extractJSONArray(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "[") {
		return raw
	}
	start := strings.Index(raw, "[")
	if start < 0 {
		return ""
	}
	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(raw); i++ {
		ch := raw[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}
		switch ch {
		case '"':
			inString = true
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return raw[start : i+1]
			}
			if depth < 0 {
				return ""
			}
		}
	}
	return ""
}

func ValidateOperation(op Operation) error {
	switch op.Action {
	case "create":
		if op.Level == "" || op.Type == "" || strings.TrimSpace(op.Title) == "" || strings.TrimSpace(op.Content) == "" {
			return fmt.Errorf("create operation missing required fields")
		}
	case "update":
		if op.Level == "" || strings.TrimSpace(op.Filename) == "" || strings.TrimSpace(op.Content) == "" {
			return fmt.Errorf("update operation missing required fields")
		}
	case "delete":
		if op.Level == "" || strings.TrimSpace(op.Filename) == "" {
			return fmt.Errorf("delete operation missing required fields")
		}
	case "", "noop":
		return nil
	default:
		return fmt.Errorf("unsupported action: %s", op.Action)
	}
	return nil
}
