package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/glamour/v2"
	"charm.land/lipgloss/v2"
)

var (
	inputBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(0, 1)
	statusStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("244"))
	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("203"))
	userStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("81"))
	assistantStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))
	streamStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))
)

func (m Model) view() string {
	if m.initErr != nil {
		return errorStyle.Render("● " + m.initErr.Error())
	}
	if m.state == stateSelecting {
		return m.list.View()
	}

	parts := make([]string, 0, 3)
	if m.state == stateStreaming {
		parts = append(parts, m.streamingView())
	}

	inputWidth := max(20, m.width-2)
	parts = append(parts, inputBoxStyle.Width(inputWidth-2).Render(m.textarea.View()))
	parts = append(parts, m.statusBar())
	return strings.Join(parts, "\n")
}

func (m Model) streamingView() string {
	reply := softWrap(m.curReply.String(), max(20, m.width-4))
	timer := fmt.Sprintf("%s Imagining... (%ds)", m.spinner.View(), int(m.elapsed.Seconds()))
	if strings.TrimSpace(reply) == "" {
		return streamStyle.Render("● " + timer)
	}
	return streamStyle.Render("● " + reply + "\n" + timer)
}

func (m Model) statusBar() string {
	left := "No provider"
	right := ""
	if m.provider != nil {
		left = m.provider.Name()
		right = m.provider.Model()
	}
	width := max(20, m.width)
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return statusStyle.Width(width).Render(left + strings.Repeat(" ", gap) + right)
}

func userBlock(text string) string {
	return userStyle.Render("● " + text)
}

func assistantBlock(reply string, elapsed time.Duration, renderer *glamour.TermRenderer) string {
	rendered := strings.TrimSpace(reply)
	if renderer != nil && rendered != "" {
		if out, err := renderer.Render(reply); err == nil {
			rendered = strings.TrimSpace(out)
		}
	}
	if rendered == "" {
		rendered = "(empty response)"
	}
	return assistantStyle.Render(fmt.Sprintf("● %s\nDone in %.1fs", rendered, elapsed.Seconds()))
}

func errorBlock(err error) string {
	return errorStyle.Render("● Error: " + err.Error())
}

func softWrap(s string, width int) string {
	if width <= 0 || s == "" {
		return s
	}
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = wrapLine(line, width)
	}
	return strings.Join(lines, "\n")
}

func wrapLine(line string, width int) string {
	var b strings.Builder
	col := 0
	for _, r := range line {
		w := lipgloss.Width(string(r))
		if col > 0 && col+w > width {
			b.WriteByte('\n')
			col = 0
		}
		b.WriteRune(r)
		col += w
	}
	return b.String()
}
