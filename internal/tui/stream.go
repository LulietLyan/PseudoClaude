package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"PseudoClaude/internal/agent"
	"PseudoClaude/internal/compact"
	"PseudoClaude/internal/permission"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
)

type agentMsg agent.Event
type compactMsg struct {
	output compact.ManageOutput
	err    error
}

func waitForAgentEvent(ch <-chan agent.Event) tea.Cmd {
	return func() tea.Msg {
		event, ok := <-ch
		if !ok {
			return agentMsg{Type: agent.EventStop, Stop: &agent.Stop{Reason: agent.StopCompleted, Message: "completed"}}
		}
		return agentMsg(event)
	}
}

func (m Model) updateIdle(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		if next, cmd, ok := m.handleCompletionKey(msg); ok {
			return next, cmd
		}
		if msg.String() == "shift+tab" {
			m.permissionMode = permission.NextMode(m.permissionMode)
			m.appendTranscript(transcriptEntry{kind: transcriptStatus, text: "Permission mode: " + m.permissionMode.String()})
			return m, nil
		}
		if msg.String() == "enter" {
			text := strings.TrimSpace(m.textarea.Value())
			if text == "" {
				return m, nil
			}
			if next, cmd, handled := m.dispatchInput(text); handled {
				return next, cmd
			}
			return m.submitUserText(text)
		}
	case compactMsg:
		m.progress = ""
		m.state = stateIdle
		m.textarea.Reset()
		if msg.err != nil {
			m.appendTranscript(transcriptEntry{kind: transcriptError, text: "压缩失败: " + msg.err.Error()})
			return m, m.textarea.Focus()
		}
		m.appendTranscript(transcriptEntry{kind: transcriptStatus, text: fmt.Sprintf("上下文已压缩：estimated tokens %d -> %d", msg.output.BeforeTokens, msg.output.AfterTokens)})
		return m, m.textarea.Focus()
	}

	var cmd tea.Cmd
	m.textarea, cmd = m.textarea.Update(msg)
	m = m.updateCompletionFromInput()
	return m, cmd
}

func (m Model) startManualCompact() (tea.Model, tea.Cmd) {
	if m.provider == nil {
		m.appendTranscript(transcriptEntry{kind: transcriptError, text: "provider 尚未初始化"})
		return m, nil
	}
	if m.compactRuntime == nil {
		m.appendTranscript(transcriptEntry{kind: transcriptError, text: "compact runtime 尚未初始化"})
		return m, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.progress = "正在压缩上下文..."
	m.turnStart = time.Now()
	m.elapsed = 0
	m.state = stateStreaming
	m.textarea.Reset()
	m.appendTranscript(transcriptEntry{kind: transcriptStatus, text: "正在压缩上下文..."})
	return m, tea.Batch(func() tea.Msg {
		output, err := compact.ForceCompact(ctx, compact.ManageInput{
			Conversation: m.conv,
			Runtime:      m.compactRuntime,
			Provider:     m.provider,
			Trigger:      compact.TriggerManual,
		})
		return compactMsg{output: output, err: err}
	}, m.spinner.Tick)
}

func (m Model) submitUserText(text string) (tea.Model, tea.Cmd) {
	return m.submitAgentText(text, text)
}

func (m Model) submitPresetText(displayLabel, prompt string) (tea.Model, tea.Cmd) {
	printable := strings.TrimSpace(displayLabel)
	if printable == "" {
		printable = prompt
	}
	return m.submitAgentText(prompt, printable)
}

func (m Model) submitAgentText(text, printableOverride string) (tea.Model, tea.Cmd) {
	if m.provider == nil {
		m.appendTranscript(transcriptEntry{kind: transcriptError, text: "provider 尚未初始化"})
		return m, nil
	}
	req, printable, err := m.requestForInput(text)
	if err != nil {
		m.appendTranscript(transcriptEntry{kind: transcriptError, text: err.Error()})
		return m, nil
	}
	if strings.TrimSpace(printableOverride) != "" {
		printable = printableOverride
	}

	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.runner.Provider = m.provider
	m.runner.Registry = m.registry
	m.runner.Env = m.toolEnv
	m.runner.Permission = m.permissionEngine
	m.runner.Compact = m.compactRuntime
	m.runner.Instructions = m.instructions
	m.runner.Memory = m.memory
	m.events = m.runner.Run(ctx, req)
	m.turnStart = time.Now()
	m.elapsed = 0
	m.curReply = ""
	m.curTool = nil
	m.progress = "starting"
	m.usage = nil
	m.lastStop = nil
	m.textarea.Reset()
	m.completion = completionState{}
	m.state = stateStreaming
	m.appendTranscript(transcriptEntry{kind: transcriptUser, text: printable})

	return m, tea.Batch(
		waitForAgentEvent(m.events),
		m.spinner.Tick,
	)
}

func (m Model) requestForInput(text string) (agent.Request, string, error) {
	switch {
	case m.planMode:
		return agent.Request{Mode: agent.ModePlan, PlanTask: text, PermissionMode: m.permissionMode, Conversation: m.conv}, text, nil
	default:
		return agent.Request{Mode: agent.ModeChat, UserText: text, PermissionMode: m.permissionMode, Conversation: m.conv}, text, nil
	}
}

func (m Model) updateStreaming(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case compactMsg:
		m.elapsed = time.Since(m.turnStart)
		m.stopStream()
		m.state = stateIdle
		m.progress = ""
		m.curTool = nil
		if msg.err != nil {
			m.appendTranscript(transcriptEntry{kind: transcriptError, text: "压缩失败: " + msg.err.Error()})
			return m, m.textarea.Focus()
		}
		m.appendTranscript(transcriptEntry{kind: transcriptStatus, text: fmt.Sprintf("上下文已压缩：estimated tokens %d -> %d", msg.output.BeforeTokens, msg.output.AfterTokens)})
		return m, m.textarea.Focus()
	case agentMsg:
		return m.handleAgentEvent(agent.Event(msg))
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		m.elapsed = time.Since(m.turnStart)
		return m, cmd
	case tea.KeyPressMsg:
		if msg.String() == "esc" {
			m.stopStream()
			m.state = stateIdle
			m.curTool = nil
			m.progress = "canceled"
			m.appendTranscript(transcriptEntry{kind: transcriptStop, stop: agent.Stop{Reason: agent.StopCanceled, Message: "canceled"}})
			return m, m.textarea.Focus()
		}
		return m, nil
	}
	return m, nil
}

func (m Model) handleAgentEvent(event agent.Event) (tea.Model, tea.Cmd) {
	m.elapsed = time.Since(m.turnStart)
	switch event.Type {
	case agent.EventProgress:
		m.progress = event.Message
		if isCompactProgress(event.Message) {
			m.appendTranscript(transcriptEntry{kind: transcriptStatus, text: event.Message})
		}
		return m, waitForAgentEvent(m.events)
	case agent.EventTextDelta:
		m.curReply += event.Text
		return m, waitForAgentEvent(m.events)
	case agent.EventUsage:
		if event.Usage != nil {
			usage := *event.Usage
			m.usage = &usage
		}
		return m, waitForAgentEvent(m.events)
	case agent.EventAssistantText:
		return m, waitForAgentEvent(m.events)
	case agent.EventToolCallStart:
		if event.ToolCall != nil {
			m.curTool = &toolStatus{call: *event.ToolCall, started: time.Now()}
		}
		return m, waitForAgentEvent(m.events)
	case agent.EventToolResult:
		if event.ToolResult != nil {
			result := event.ToolResult.Result
			if m.curTool != nil && m.curTool.call.ID == event.ToolResult.Call.ID {
				m.curTool.result = &result
			}
			m.appendTranscript(transcriptEntry{kind: transcriptTool, result: result, elapsed: event.ToolResult.Elapsed})
			return m, waitForAgentEvent(m.events)
		}
		return m, waitForAgentEvent(m.events)
	case agent.EventToolCallDone:
		return m, waitForAgentEvent(m.events)
	case agent.EventApproval:
		if event.Approval != nil {
			m.pendingApproval = event.Approval
			m.approvalCursor = 0
			m.state = stateApproving
			return m, nil
		}
		return m, waitForAgentEvent(m.events)
	case agent.EventError:
		if event.Err != nil {
			m.appendTranscript(transcriptEntry{kind: transcriptError, text: event.Err.Error()})
		}
		return m, waitForAgentEvent(m.events)
	case agent.EventStop:
		return m.finishAgentRun(event)
	default:
		return m, waitForAgentEvent(m.events)
	}
}

func isCompactProgress(message string) bool {
	return strings.Contains(message, "压缩上下文") ||
		strings.Contains(message, "上下文已压缩") ||
		strings.Contains(message, "工具结果已落盘")
}

func (m Model) updateApproving(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "up", "k":
			if m.approvalCursor > 0 {
				m.approvalCursor--
			}
			return m, nil
		case "down", "j":
			if m.approvalCursor < len(approvalChoices)-1 {
				m.approvalCursor++
			}
			return m, nil
		case "enter", " ":
			return m.finishApproval(approvalChoices[m.approvalCursor].decision)
		case "1":
			return m.finishApproval(permission.ApprovalAllowOnce)
		case "2":
			return m.finishApproval(permission.ApprovalAllowSession)
		case "3":
			return m.finishApproval(permission.ApprovalAllowForever)
		case "4":
			return m.finishApproval(permission.ApprovalDenyOnce)
		case "esc":
			m.stopStream()
			return m.finishApproval(permission.ApprovalDenyOnce)
		}
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		m.elapsed = time.Since(m.turnStart)
		return m, cmd
	}
	return m, nil
}

func (m Model) finishApproval(decision permission.ApprovalDecision) (tea.Model, tea.Cmd) {
	req := m.pendingApproval
	m.pendingApproval = nil
	m.approvalCursor = 0
	m.state = stateStreaming
	var cmds []tea.Cmd
	if req != nil {
		cmds = append(cmds, sendApprovalDecision(req, decision))
	}
	cmds = append(cmds, waitForAgentEvent(m.events), m.spinner.Tick)
	return m, tea.Batch(cmds...)
}

func sendApprovalDecision(req *agent.ApprovalRequest, decision permission.ApprovalDecision) tea.Cmd {
	return func() tea.Msg {
		select {
		case req.Respond <- decision:
		default:
		}
		return nil
	}
}

func (m Model) finishAgentRun(event agent.Event) (tea.Model, tea.Cmd) {
	m.elapsed = time.Since(m.turnStart)
	m.stopStream()
	m.state = stateIdle
	m.progress = ""
	if event.Stop != nil {
		stop := *event.Stop
		m.lastStop = &stop
	}
	reply := m.curReply
	var cmds []tea.Cmd
	if strings.TrimSpace(reply) != "" {
		m.appendTranscript(transcriptEntry{kind: transcriptAssistant, text: reply, elapsed: m.elapsed})
	}
	if event.Stop != nil && event.Stop.Reason != agent.StopCompleted {
		m.appendTranscript(transcriptEntry{kind: transcriptStop, stop: *event.Stop})
	}
	if event.Stop != nil && event.Stop.Reason == agent.StopCompleted {
		m.saveOrClearPlan(reply)
	}
	m.curReply = ""
	m.curTool = nil
	cmds = append(cmds, m.textarea.Focus())
	return m, tea.Batch(cmds...)
}

func (m *Model) saveOrClearPlan(reply string) {
	msgs := m.conv.Messages()
	if len(msgs) == 0 {
		return
	}
	lastUser := ""
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "user" && msgs[i].Content != "" {
			lastUser = msgs[i].Content
			break
		}
	}
	if strings.HasPrefix(lastUser, "Plan mode.") {
		m.lastPlan = &planState{task: extractPromptSection(lastUser, "Task:"), text: reply}
		return
	}
	if strings.HasPrefix(lastUser, "Execution mode.") {
		m.lastPlan = nil
		m.planMode = false
	}
}

func extractPromptSection(text, marker string) string {
	idx := strings.Index(text, marker)
	if idx < 0 {
		return strings.TrimSpace(text)
	}
	return strings.TrimSpace(text[idx+len(marker):])
}
