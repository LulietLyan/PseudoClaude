package tui

import (
	"fmt"
	"strings"
	"time"

	"PseudoClaude/internal/agent"
	"PseudoClaude/internal/llm"
	"PseudoClaude/internal/permission"
	"PseudoClaude/internal/prompt"
	"PseudoClaude/internal/tools"

	"charm.land/glamour/v2"
	"charm.land/lipgloss/v2"
)

var (
	columnStyle = lipgloss.NewStyle().
			Align(lipgloss.Left)
	inputBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("63")).
			Padding(0, 2)
	statusStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("244")).
			Padding(0, 1)
	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("203"))
	userFrameStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("81")).
			Foreground(lipgloss.Color("231")).
			Padding(0, 2)
	assistantFrameStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("114")).
				Foreground(lipgloss.Color("252")).
				Padding(1, 2)
	statusFrameStyle = lipgloss.NewStyle().
				Border(lipgloss.NormalBorder()).
				BorderForeground(lipgloss.Color("244")).
				Foreground(lipgloss.Color("244")).
				Padding(0, 2)
	errorFrameStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("203")).
			Foreground(lipgloss.Color("203")).
			Padding(0, 2)
	streamStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("63")).
			Foreground(lipgloss.Color("252")).
			Padding(0, 2)
	toolStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("220"))
	toolFrameStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("220")).
			Foreground(lipgloss.Color("229")).
			Padding(0, 2)
	toolOKStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("114"))
	toolErrorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("203"))
	approvalStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("220")).
			Padding(0, 1)
	approvalSelectedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("220")).
				Bold(true)
	logoStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("220"))
	bannerFrameStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("240")).
				Padding(1, 3)
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
		if m.showBanner {
			return strings.Join([]string{m.bannerView(), m.list.View()}, "\n\n")
		}
		return m.list.View()
	}

	columnWidth := m.contentWidth()
	parts := make([]string, 0, 4)
	if m.showBanner {
		parts = append(parts, m.bannerView())
	}
	if len(m.transcript) > 0 {
		parts = append(parts, m.centeredColumn(m.transcriptView(columnWidth)))
	}
	if m.state == stateStreaming {
		parts = append(parts, m.centeredColumn(m.streamingView(columnWidth)))
	}
	if m.state == stateApproving {
		parts = append(parts, m.centeredColumn(m.streamingView(columnWidth)), m.centeredColumn(m.approvalBlock(columnWidth)))
	}

	parts = append(parts, m.centeredColumn(inputBoxStyle.Width(columnWidth).Render(m.textarea.View())))
	parts = append(parts, m.centeredColumn(m.statusBar(columnWidth)))
	content := strings.Join(parts, "\n")
	if m.height <= 0 {
		return content
	}
	return spaceBeforeInput(parts, m.height)
}

func (m Model) contentWidth() int {
	terminalWidth := max(20, m.width)
	return clamp((terminalWidth*9)/10, 28, max(28, terminalWidth-4))
}

func (m Model) centeredColumn(content string) string {
	width := max(20, m.width)
	return lipgloss.NewStyle().Width(width).Align(lipgloss.Center).Render(columnStyle.Width(m.contentWidth()).Render(content))
}

func (m *Model) appendTranscript(entry transcriptEntry) {
	if entry.kind != transcriptTool && strings.TrimSpace(entry.text) == "" && entry.stop.Reason == "" {
		return
	}
	m.transcript = append(m.transcript, entry)
	const maxTranscriptBlocks = 80
	if len(m.transcript) > maxTranscriptBlocks {
		m.transcript = append([]transcriptEntry(nil), m.transcript[len(m.transcript)-maxTranscriptBlocks:]...)
	}
}

func (m Model) transcriptView(width int) string {
	blocks := make([]string, 0, len(m.transcript))
	for _, entry := range m.transcript {
		blocks = append(blocks, m.renderTranscriptEntry(entry, width))
	}
	return strings.Join(blocks, "\n\n")
}

func (m Model) renderTranscriptEntry(entry transcriptEntry, width int) string {
	switch entry.kind {
	case transcriptUser:
		return userBlock(entry.text, width)
	case transcriptAssistant:
		return assistantBlock(entry.text, entry.elapsed, width, m.renderer)
	case transcriptTool:
		return toolResultBlock(entry.result, entry.elapsed, width)
	case transcriptError:
		return errorBlockString(entry.text, width)
	case transcriptStop:
		return stopBlock(entry.stop, width)
	default:
		return statusMessageBlock(entry.text, width)
	}
}

func (m Model) bannerView() string {
	terminalWidth := max(20, m.width)
	frameWidth := clamp(terminalWidth-4, 24, 96)
	contentWidth := max(20, frameWidth-8)
	height := max(1, m.height)
	logo := prompt.SelectLogo(contentWidth, height)
	lines := make([]string, 0, 9)
	for _, line := range strings.Split(logo, "\n") {
		lines = append(lines, centerLine(logoStyle.Render(strings.TrimRight(line, " ")), contentWidth))
	}
	lines = append(lines,
		"",
		centerLine(bannerTitleStyle.Render("PseudoClaude v"+Version), contentWidth),
		centerLine(bannerMetaStyle.Render(fitLine("cwd: "+m.cwd, contentWidth)), contentWidth),
		centerLine(bannerMetaStyle.Render("Ready. Shift+Tab cycles permission mode."), contentWidth),
	)
	panel := bannerFrameStyle.Width(frameWidth).Render(strings.Join(lines, "\n"))
	return lipgloss.NewStyle().Width(terminalWidth).Align(lipgloss.Center).Render(panel)
}

func (m Model) streamingView(width int) string {
	reply := softWrap(m.curReply, max(20, width-6))
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
	return streamStyle.Width(width).Render(strings.Join(lines, "\n"))
}

func (m Model) statusBar(width int) string {
	width = max(3, width)
	left := permissionModeLabel(m.permissionMode)
	right := ""
	if m.provider != nil {
		right = m.provider.Model()
	}
	if m.planMode {
		left += " · PLAN WORKFLOW"
	}
	if m.lastStop != nil && m.lastStop.Reason != "" {
		left += " · " + string(m.lastStop.Reason)
	}
	innerWidth := max(1, width-2)
	leftWidth := lipgloss.Width(left)
	if leftWidth >= innerWidth {
		return statusStyle.Width(width).Render(fitLine(left, innerWidth))
	}
	maxRight := innerWidth - leftWidth - 1
	if maxRight < 0 {
		maxRight = 0
	}
	right = fitLine(right, maxRight)
	gap := innerWidth - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	line := left + strings.Repeat(" ", gap) + right
	if lipgloss.Width(line) > innerWidth {
		line = fitLine(line, innerWidth)
	}
	return statusStyle.Width(width).Render(line)
}

type approvalChoice struct {
	label    string
	decision permission.ApprovalDecision
}

var approvalChoices = []approvalChoice{
	{"1 Allow once", permission.ApprovalAllowOnce},
	{"2 Allow session", permission.ApprovalAllowSession},
	{"3 Allow forever", permission.ApprovalAllowForever},
	{"4 Deny once", permission.ApprovalDenyOnce},
}

func (m Model) approvalBlock(width int) string {
	req := m.pendingApproval
	if req == nil {
		return ""
	}
	innerWidth := max(20, width-6)
	lines := []string{
		"Permission required",
		"Tool: " + req.Call.Name,
		"Target: " + fitLine(req.Summary, innerWidth),
		"Reason: " + fitLine(req.Reason, innerWidth),
	}
	for i, choice := range approvalChoices {
		line := "  " + choice.label
		if i == m.approvalCursor {
			line = approvalSelectedStyle.Render("> " + choice.label)
		}
		lines = append(lines, line)
	}
	return approvalStyle.Width(width).Render(strings.Join(lines, "\n"))
}

func permissionModeLabel(mode permission.Mode) string {
	switch mode {
	case permission.ModeStrict:
		return "STRICT"
	case permission.ModeAcceptEdits:
		return "ACCEPT EDITS"
	case permission.ModeBypassPermissions:
		return "BYPASS"
	default:
		return "DEFAULT"
	}
}

func userBlock(text string, width int) string {
	return userFrameStyle.Width(width).Render("You\n" + text)
}

func statusMessageBlock(message string, width int) string {
	return statusFrameStyle.Width(width).Render(message)
}

func assistantBlock(reply string, elapsed time.Duration, width int, renderer *glamour.TermRenderer) string {
	rendered := strings.TrimSpace(reply)
	if renderer != nil && rendered != "" {
		if out, err := renderer.Render(reply); err == nil {
			rendered = strings.TrimSpace(out)
		}
	}
	if rendered == "" {
		rendered = "(empty response)"
	}
	return assistantFrameStyle.Width(width).Render(fmt.Sprintf("Assistant\n%s\n\nDone in %.1fs", rendered, elapsed.Seconds()))
}

func errorBlock(err error) string {
	if err == nil {
		return errorBlockString("Error", 0)
	}
	return errorBlockString(err.Error(), 0)
}

func errorBlockString(message string, width int) string {
	if strings.TrimSpace(message) == "" {
		message = "Error"
	}
	if width <= 0 {
		return errorFrameStyle.Render("Error\n" + message)
	}
	return errorFrameStyle.Width(width).Render("Error\n" + message)
}

func stopBlock(stop agent.Stop, width int) string {
	message := stop.Message
	if message == "" {
		message = string(stop.Reason)
	}
	return statusMessageBlock(fmt.Sprintf("Stop: %s (%s)", message, stop.Reason), width)
}

func toolCallBlock(call llm.ToolCall) string {
	id := call.ID
	if id == "" {
		id = "no-id"
	}
	return toolStyle.Render(fmt.Sprintf("  ↳ Tool: %s (%s)", call.Name, id))
}

func toolResultBlock(result tools.Result, elapsed time.Duration, width int) string {
	if result.OK {
		content := strings.TrimSpace(result.Content)
		if content == "" {
			content = "ok"
		}
		content, _ = truncateForDisplay(content, 800)
		return toolFrameStyle.Width(width).BorderForeground(lipgloss.Color("114")).Render(fmt.Sprintf("Tool · %s · %.1fs\n%s", result.Tool, elapsed.Seconds(), indentContinuation(toolOKStyle.Render(content), "  ")))
	}
	message := result.Error
	if message == "" {
		message = result.ErrorType
	}
	message, _ = truncateForDisplay(message, 800)
	return toolFrameStyle.Width(width).BorderForeground(lipgloss.Color("203")).Render(fmt.Sprintf("Tool · %s · %.1fs · %s\n%s", result.Tool, elapsed.Seconds(), result.ErrorType, indentContinuation(toolErrorStyle.Render(message), "  ")))
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

func clamp(value, minValue, maxValue int) int {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func fitLine(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= width {
		return s
	}
	prefix := "..."
	available := width - lipgloss.Width(prefix)
	if available <= 0 {
		return prefix[:width]
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
