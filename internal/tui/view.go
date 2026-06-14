package tui

import (
	"fmt"
	"strings"
	"time"

	"PseudoClaude/internal/agent"
	"PseudoClaude/internal/llm"
	"PseudoClaude/internal/prompt"
	"PseudoClaude/internal/tools"

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
	toolStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("220"))
	toolOKStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("114"))
	toolErrorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("203"))
	logoStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("220"))
	bannerTitleStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("81")).
				Bold(true)
	bannerMetaStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("244"))
)

func (m Model) view() string {
	if m.initErr != nil {
		return errorStyle.Render("● " + m.initErr.Error())
	}
	if m.state == stateSelecting {
		return m.list.View()
	}

	parts := make([]string, 0, 4)
	if m.state == stateStreaming {
		parts = append(parts, m.streamingView())
	}

	inputWidth := max(20, m.width-2)
	parts = append(parts, inputBoxStyle.Width(inputWidth-2).Render(m.textarea.View()))
	parts = append(parts, m.statusBar())
	content := strings.Join(parts, "\n")
	if m.height <= 0 {
		return content
	}
	return spaceBeforeInput(parts, m.height)
}

func (m Model) bannerView() string {
	width := max(20, m.width-4)
	height := max(1, m.height)
	logo := prompt.SelectLogo(width, height)
	lines := make([]string, 0, 9)
	for _, line := range strings.Split(logo, "\n") {
		lines = append(lines, centerLine(logoStyle.Render(strings.TrimRight(line, " ")), width))
	}
	lines = append(lines,
		centerLine(bannerTitleStyle.Render("PseudoClaude v"+Version), width),
		centerLine(bannerMetaStyle.Render(fitLine("cwd: "+m.cwd, width)), width),
		centerLine(bannerMetaStyle.Render("Ready. Start a conversation when you are."), width),
	)
	return strings.Join(lines, "\n")
}

func (m Model) streamingView() string {
	reply := softWrap(m.curReply.String(), max(20, m.width-4))
	label := m.progress
	if label == "" {
		label = "Imagining..."
	}
	timer := fmt.Sprintf("%s %s (%ds)", m.spinner.View(), label, int(m.elapsed.Seconds()))
	lines := make([]string, 0, 3)
	if strings.TrimSpace(reply) != "" {
		lines = append(lines, "● "+reply)
	}
	if m.curTool != nil {
		lines = append(lines, toolProgressLine(*m.curTool, time.Since(m.curTool.started)))
	}
	if m.usage != nil {
		lines = append(lines, usageLine(*m.usage))
	}
	lines = append(lines, "● "+timer)
	return streamStyle.Render(strings.Join(lines, "\n"))
}

func (m Model) statusBar() string {
	left := "No provider"
	right := ""
	if m.provider != nil {
		left = m.provider.Name()
		right = m.provider.Model()
	}
	if m.lastStop != nil && m.lastStop.Reason != "" {
		left += " · " + string(m.lastStop.Reason)
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
	if err == nil {
		return errorStyle.Render("● Error")
	}
	return errorStyle.Render("● Error: " + err.Error())
}

func stopBlock(stop agent.Stop) string {
	message := stop.Message
	if message == "" {
		message = string(stop.Reason)
	}
	return statusStyle.Render(fmt.Sprintf("● Stop: %s (%s)", message, stop.Reason))
}

func toolCallBlock(call llm.ToolCall) string {
	id := call.ID
	if id == "" {
		id = "no-id"
	}
	return toolStyle.Render(fmt.Sprintf("  ↳ Tool: %s (%s)", call.Name, id))
}

func toolResultBlock(result tools.Result, elapsed time.Duration) string {
	if result.OK {
		content := strings.TrimSpace(result.Content)
		if content == "" {
			content = "ok"
		}
		content, _ = truncateForDisplay(content, 800)
		return toolOKStyle.Render(fmt.Sprintf("  ✓ Tool: %s completed in %.1fs\n  %s", result.Tool, elapsed.Seconds(), indentContinuation(content, "  ")))
	}
	message := result.Error
	if message == "" {
		message = result.ErrorType
	}
	message, _ = truncateForDisplay(message, 800)
	return toolErrorStyle.Render(fmt.Sprintf("  × Tool: %s failed in %.1fs [%s]\n  %s", result.Tool, elapsed.Seconds(), result.ErrorType, indentContinuation(message, "  ")))
}

func toolProgressLine(status toolStatus, elapsed time.Duration) string {
	name := status.call.Name
	if name == "" {
		name = "unknown"
	}
	if status.result != nil && status.result.OK {
		return toolOKStyle.Render(fmt.Sprintf("  ✓ Tool: %s completed in %.1fs", name, elapsed.Seconds()))
	}
	if status.result != nil {
		return toolErrorStyle.Render(fmt.Sprintf("  × Tool: %s failed in %.1fs", name, elapsed.Seconds()))
	}
	return toolStyle.Render(fmt.Sprintf("  ↳ Tool: %s running... (%ds)", name, int(elapsed.Seconds())))
}

func usageLine(usage llm.Usage) string {
	return statusStyle.Render(fmt.Sprintf("  tokens: in %d · out %d · total %d", usage.InputTokens, usage.OutputTokens, usage.TotalTokens))
}

func spaceBeforeInput(parts []string, height int) string {
	used := 0
	for _, part := range parts {
		used += lipgloss.Height(part)
	}
	if len(parts) > 1 {
		used += len(parts) - 1
	}
	pad := height - used
	if pad <= 0 {
		return strings.Join(parts, "\n")
	}
	if pad > 2 {
		pad = 2
	}
	top := strings.Join(parts[:max(0, len(parts)-2)], "\n")
	bottom := strings.Join(parts[max(0, len(parts)-2):], "\n")
	if top == "" {
		return strings.Repeat("\n", pad) + bottom
	}
	return top + strings.Repeat("\n", pad+1) + bottom
}

func indentContinuation(s, prefix string) string {
	lines := strings.Split(s, "\n")
	for i := 1; i < len(lines); i++ {
		lines[i] = prefix + lines[i]
	}
	return strings.Join(lines, "\n")
}

func centerLine(s string, width int) string {
	if width <= 0 {
		return s
	}
	gap := width - lipgloss.Width(s)
	if gap <= 0 {
		return s
	}
	return strings.Repeat(" ", gap/2) + s
}

func fitLine(s string, width int) string {
	if width <= 0 || lipgloss.Width(s) <= width {
		return s
	}
	prefix := "..."
	available := width - lipgloss.Width(prefix)
	if available <= 0 {
		return prefix
	}
	runes := []rune(s)
	var out []rune
	for i := len(runes) - 1; i >= 0; i-- {
		candidate := string(append([]rune{runes[i]}, out...))
		if lipgloss.Width(candidate) > available {
			break
		}
		out = append([]rune{runes[i]}, out...)
	}
	return prefix + string(out)
}

func truncateForDisplay(s string, limit int) (string, bool) {
	if len(s) <= limit {
		return s, false
	}
	return s[:limit] + "...[truncated]", true
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
