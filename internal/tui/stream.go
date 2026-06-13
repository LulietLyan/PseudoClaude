package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"PseudoClaude/internal/llm"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
)

type streamMsg llm.StreamEvent

func waitForEvent(ch <-chan llm.StreamEvent) tea.Cmd {
	return func() tea.Msg {
		event, ok := <-ch
		if !ok {
			return streamMsg{Done: true}
		}
		return streamMsg(event)
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

	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.conv.AddUser(text)
	m.events = m.provider.Stream(ctx, m.conv.Messages())
	m.turnStart = time.Now()
	m.elapsed = 0
	m.curReply.Reset()
	m.textarea.Reset()
	m.state = stateStreaming

	return m, tea.Batch(
		tea.Println(userBlock(text)),
		waitForEvent(m.events),
		m.spinner.Tick,
	)
}

func (m Model) updateStreaming(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case streamMsg:
		event := llm.StreamEvent(msg)
		if event.Err != nil {
			m.elapsed = time.Since(m.turnStart)
			m.stopStream()
			m.state = stateIdle
			return m, tea.Batch(m.textarea.Focus(), tea.Println(errorBlock(event.Err)))
		}
		if event.Done {
			m.elapsed = time.Since(m.turnStart)
			m.stopStream()
			reply := m.curReply.String()
			if strings.TrimSpace(reply) != "" {
				m.conv.AddAssistant(reply)
			}
			m.state = stateIdle
			m.curReply.Reset()
			return m, tea.Batch(m.textarea.Focus(), tea.Println(assistantBlock(reply, m.elapsed, m.renderer)))
		}
		if event.Text != "" {
			m.curReply.WriteString(event.Text)
		}
		return m, waitForEvent(m.events)
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		m.elapsed = time.Since(m.turnStart)
		return m, cmd
	case tea.KeyPressMsg:
		return m, nil
	}
	return m, nil
}
