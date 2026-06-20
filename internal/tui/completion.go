package tui

import (
	"strings"

	"PseudoClaude/internal/command"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

const maxCompletionRows = 6

type completionState struct {
	active  bool
	manual  bool
	items   []command.Completion
	cursor  int
	message string
}

var (
	completionStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("244")).
			Padding(0, 1)
	completionSelectedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("220")).
				Bold(true)
	completionMutedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("244"))
)

func (m Model) updateCompletionFromInput() Model {
	value := m.textarea.Value()
	if !isCompletableInput(value) {
		m.completion = completionState{}
		return m
	}
	items := m.commandRegistry.Complete(currentCommandToken(value))
	manual := m.completion.manual
	cursor := m.completion.cursor
	if len(items) == 0 {
		m.completion = completionState{message: "No matching commands."}
		return m
	}
	if cursor >= len(items) {
		cursor = len(items) - 1
	}
	if cursor < 0 {
		cursor = 0
	}
	m.completion = completionState{active: true, manual: manual, items: items, cursor: cursor}
	return m
}

func (m Model) handleCompletionKey(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	switch msg.String() {
	case "esc":
		if m.completion.active || m.completion.message != "" {
			m.completion = completionState{}
			return m, nil, true
		}
	case "up", "k":
		if m.completion.active && m.completion.manual {
			if m.completion.cursor > 0 {
				m.completion.cursor--
			}
			return m, nil, true
		}
	case "down", "j":
		if m.completion.active && m.completion.manual {
			if m.completion.cursor < len(m.completion.items)-1 {
				m.completion.cursor++
			}
			return m, nil, true
		}
	case "enter":
		if m.completion.active && m.completion.manual {
			m = m.applyCompletion(m.completion.cursor)
			return m, nil, true
		}
	case "tab":
		if !isCompletableInput(m.textarea.Value()) {
			m.completion = completionState{}
			return m, nil, false
		}
		items := m.commandRegistry.Complete(currentCommandToken(m.textarea.Value()))
		switch len(items) {
		case 0:
			m.completion = completionState{message: "No matching commands."}
		case 1:
			m.textarea.SetValue(items[0].Text)
			m.completion = completionState{}
		default:
			m.completion = completionState{active: true, manual: true, items: items}
		}
		return m, nil, true
	}
	return m, nil, false
}

func (m Model) applyCompletion(index int) Model {
	if index < 0 || index >= len(m.completion.items) {
		return m
	}
	m.textarea.SetValue(m.completion.items[index].Text)
	m.completion = completionState{}
	return m
}

func (m Model) completionView(width int) string {
	width = max(20, width)
	if m.completion.message != "" {
		return completionStyle.Width(width).Render(completionMutedStyle.Render(m.completion.message))
	}
	if !m.completion.active || len(m.completion.items) == 0 {
		return ""
	}
	start := 0
	if m.completion.cursor >= maxCompletionRows {
		start = m.completion.cursor - maxCompletionRows + 1
	}
	end := min(len(m.completion.items), start+maxCompletionRows)
	lines := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		item := m.completion.items[i]
		line := completionLine(item, max(1, width-6))
		if i == m.completion.cursor {
			prefix := "  "
			if m.completion.manual {
				prefix = "> "
			}
			line = completionSelectedStyle.Render(prefix + line)
		} else {
			line = "  " + line
		}
		lines = append(lines, line)
	}
	return completionStyle.Width(width).Render(strings.Join(lines, "\n"))
}

func completionLine(item command.Completion, width int) string {
	label := item.Name
	if item.Skill {
		label += " [skill]"
	}
	label = strings.TrimSpace(label)
	description := strings.TrimSpace(item.Description)
	if description == "" {
		return fitLineEnd(label, width)
	}
	separator := "  "
	labelWidth := lipgloss.Width(label)
	remaining := width - labelWidth - lipgloss.Width(separator)
	if remaining <= 0 {
		return fitLineEnd(label, width)
	}
	return label + separator + fitLineEnd(description, remaining)
}

func fitLineEnd(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= width {
		return s
	}
	ellipsis := "..."
	ellipsisWidth := lipgloss.Width(ellipsis)
	if width <= ellipsisWidth {
		return ellipsis[:width]
	}
	available := width - ellipsisWidth
	var out []rune
	for _, r := range []rune(s) {
		candidate := string(append(out, r))
		if lipgloss.Width(candidate) > available {
			break
		}
		out = append(out, r)
	}
	return string(out) + ellipsis
}

func isCompletableInput(value string) bool {
	value = strings.TrimSpace(value)
	return strings.HasPrefix(value, "/") && !strings.Contains(value, "\n")
}

func currentCommandToken(value string) string {
	value = strings.TrimSpace(value)
	if idx := strings.IndexFunc(value, func(r rune) bool { return r == ' ' || r == '\t' || r == '\n' || r == '\r' }); idx >= 0 {
		return value[:idx]
	}
	return value
}
