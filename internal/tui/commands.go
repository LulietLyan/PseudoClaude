package tui

import (
	"PseudoClaude/internal/command"

	tea "charm.land/bubbletea/v2"
)

func (m Model) dispatchInput(text string) (Model, tea.Cmd, bool) {
	adapter := &commandAdapter{model: &m}
	result := command.Dispatch(m.commandRegistry, text, adapter)
	if !result.Handled {
		return m, nil, false
	}
	m = *adapter.model
	m.textarea.Reset()
	m.completion = completionState{}
	if result.Err != nil {
		m.appendTranscript(transcriptEntry{kind: transcriptError, text: result.Err.Error()})
	}
	return m, tea.Batch(adapter.cmds...), true
}
