package tui

import (
	"strings"

	"PseudoClaude/internal/command"
	"PseudoClaude/internal/llm"
	"PseudoClaude/internal/prompt"
	"PseudoClaude/internal/skills"

	tea "charm.land/bubbletea/v2"
)

type skillRunnerAdapter struct {
	model *Model
	cmds  *[]tea.Cmd
}

func (a *skillRunnerAdapter) RunShared(label, text string) error {
	if a == nil || a.model == nil {
		return nil
	}
	next, cmd := a.model.submitPresetText(label, text)
	if model, ok := next.(Model); ok {
		*a.model = model
	}
	if cmd != nil {
		if a.cmds != nil {
			*a.cmds = append(*a.cmds, cmd)
		}
	}
	return nil
}

func (a *skillRunnerAdapter) RunIsolated(input skills.IsolatedInput) error {
	if a == nil || a.model == nil {
		return nil
	}
	next, cmd := a.model.submitAgentTextWithTools(input.Text, "/"+input.Name, input.Tools)
	if model, ok := next.(Model); ok {
		*a.model = model
	}
	if cmd != nil && a.cmds != nil {
		*a.cmds = append(*a.cmds, cmd)
	}
	return nil
}

func (m *Model) reloadSkills() skills.ReloadResult {
	if m == nil || m.skillCatalog == nil {
		return skills.ReloadResult{}
	}
	result := m.skillCatalog.Reload(skills.LoadOptions{WorkDir: m.cwd})
	command.RemoveSkillCommands(m.commandRegistry)
	errs := command.RegisterSkillCommands(m.commandRegistry, skillSummaries(m.skillCatalog.Summaries()))
	for _, warning := range result.Warnings {
		m.appendTranscript(transcriptEntry{kind: transcriptError, text: "Skill warning: " + warning.Reason})
	}
	for _, err := range errs {
		m.appendTranscript(transcriptEntry{kind: transcriptError, text: "Skill command warning: " + err.Error()})
	}
	return result
}

func (m Model) promptSkillCatalog() []prompt.SkillCatalogItem {
	if m.skillCatalog == nil {
		return nil
	}
	items := m.skillCatalog.PromptItems()
	out := make([]prompt.SkillCatalogItem, 0, len(items))
	for _, item := range items {
		out = append(out, prompt.SkillCatalogItem{Name: item.Name, Description: item.Description})
	}
	return out
}

func (m Model) promptActiveSkills() []prompt.ActiveSkillEntry {
	if m.activeSkills == nil {
		return nil
	}
	entries := m.activeSkills.Snapshot()
	out := make([]prompt.ActiveSkillEntry, 0, len(entries))
	for _, entry := range entries {
		out = append(out, prompt.ActiveSkillEntry{Name: entry.Name, Body: entry.Body})
	}
	return out
}

func skillSummaries(summaries []skills.Summary) []command.SkillSummary {
	out := make([]command.SkillSummary, 0, len(summaries))
	for _, summary := range summaries {
		out = append(out, command.SkillSummary{
			Name:        summary.Name,
			Description: summary.Description,
			Source:      string(summary.Source),
			Mode:        string(summary.Mode),
		})
	}
	return out
}

func isolatedMessages(messages []llm.Message, input skills.IsolatedInput) []llm.Message {
	switch input.History {
	case skills.HistoryNone:
		return nil
	case skills.HistorySummary:
		if len(messages) == 0 {
			return nil
		}
		return []llm.Message{{Role: "user", Content: "Compressed summary of main conversation is not available; use only the skill instructions."}}
	default:
		if len(messages) > 8 {
			return append([]llm.Message(nil), messages[len(messages)-8:]...)
		}
		return append([]llm.Message(nil), messages...)
	}
}

func lastAssistantText(messages []llm.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "assistant" && strings.TrimSpace(messages[i].Content) != "" {
			return messages[i].Content
		}
	}
	return ""
}
