package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"PseudoClaude/internal/agent"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
)

type agentMsg agent.Event

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
		if msg.String() == "enter" {
			text := strings.TrimSpace(m.textarea.Value())
			if text == "" {
				return m, nil
			}
			if text == "/exit" {
				return m, tea.Quit
			}
			return m.submit(text)
		}
	}

	var cmd tea.Cmd
	m.textarea, cmd = m.textarea.Update(msg)
	return m, cmd
}

func (m Model) submit(text string) (tea.Model, tea.Cmd) {
	if m.provider == nil {
		return m, tea.Println(errorBlock(fmt.Errorf("provider 尚未初始化")))
	}
	req, printable, err := m.requestForInput(text)
	if err != nil {
		return m, tea.Println(errorBlock(err))
	}

	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.runner.Provider = m.provider
	m.runner.Registry = m.registry
	m.runner.Env = m.toolEnv
	m.events = m.runner.Run(ctx, req)
	m.turnStart = time.Now()
	m.elapsed = 0
	m.curReply.Reset()
	m.curTool = nil
	m.progress = "starting"
	m.usage = nil
	m.lastStop = nil
	m.textarea.Reset()
	m.state = stateStreaming

	return m, tea.Batch(
		tea.Println(userBlock(printable)),
		waitForAgentEvent(m.events),
		m.spinner.Tick,
	)
}

func (m Model) requestForInput(text string) (agent.Request, string, error) {
	switch {
	case strings.HasPrefix(text, "/plan"):
		task := strings.TrimSpace(strings.TrimPrefix(text, "/plan"))
		if task == "" {
			return agent.Request{}, "", fmt.Errorf("/plan 后面需要任务内容")
		}
		return agent.Request{Mode: agent.ModePlan, PlanTask: task, Conversation: m.conv}, "/plan " + task, nil
	case text == "/do":
		if m.lastPlan == nil || strings.TrimSpace(m.lastPlan.text) == "" {
			return agent.Request{}, "", fmt.Errorf("没有可执行的最近计划，请先使用 /plan <任务>")
		}
		return agent.Request{Mode: agent.ModeDo, PlanTask: m.lastPlan.task, PlanText: m.lastPlan.text, Conversation: m.conv}, "/do", nil
	default:
		return agent.Request{Mode: agent.ModeChat, UserText: text, Conversation: m.conv}, text, nil
	}
}

func (m Model) updateStreaming(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
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
			return m, tea.Batch(m.textarea.Focus(), tea.Println(stopBlock(agent.Stop{Reason: agent.StopCanceled, Message: "canceled"})))
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
		return m, waitForAgentEvent(m.events)
	case agent.EventTextDelta:
		m.curReply.WriteString(event.Text)
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
			return m, tea.Batch(tea.Println(toolResultBlock(result, event.ToolResult.Elapsed)), waitForAgentEvent(m.events))
		}
		return m, waitForAgentEvent(m.events)
	case agent.EventToolCallDone:
		return m, waitForAgentEvent(m.events)
	case agent.EventError:
		return m, tea.Batch(tea.Println(errorBlock(event.Err)), waitForAgentEvent(m.events))
	case agent.EventStop:
		return m.finishAgentRun(event)
	default:
		return m, waitForAgentEvent(m.events)
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
	reply := m.curReply.String()
	var cmds []tea.Cmd
	if strings.TrimSpace(reply) != "" {
		cmds = append(cmds, tea.Println(assistantBlock(reply, m.elapsed, m.renderer)))
	}
	if event.Stop != nil && event.Stop.Reason != agent.StopCompleted {
		cmds = append(cmds, tea.Println(stopBlock(*event.Stop)))
	}
	if event.Stop != nil && event.Stop.Reason == agent.StopCompleted {
		m.saveOrClearPlan(reply)
	}
	m.curReply.Reset()
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
	}
}

func extractPromptSection(text, marker string) string {
	idx := strings.Index(text, marker)
	if idx < 0 {
		return strings.TrimSpace(text)
	}
	return strings.TrimSpace(text[idx+len(marker):])
}
