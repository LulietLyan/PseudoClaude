package tui

import (
	"fmt"

	"PseudoClaude/internal/command"

	tea "charm.land/bubbletea/v2"
)

type commandAdapter struct {
	model *Model
	cmds  []tea.Cmd
}

func (a *commandAdapter) Show(kind command.MessageKind, text string) {
	if a == nil || a.model == nil {
		return
	}
	entryKind := transcriptStatus
	if kind == command.MessageError {
		entryKind = transcriptError
	} else if kind == command.MessageHelp {
		entryKind = transcriptHelp
	}
	a.model.appendTranscript(transcriptEntry{kind: entryKind, text: text})
}

func (a *commandAdapter) IsIdle() bool {
	return a != nil && a.model != nil && a.model.state == stateIdle
}

func (a *commandAdapter) WorkMode() command.WorkMode {
	if a != nil && a.model != nil && a.model.planMode {
		return command.WorkModePlan
	}
	return command.WorkModeDefault
}

func (a *commandAdapter) SetWorkMode(mode command.WorkMode) {
	if a == nil || a.model == nil {
		return
	}
	a.model.planMode = mode == command.WorkModePlan
}

func (a *commandAdapter) PermissionMode() string {
	if a == nil || a.model == nil {
		return ""
	}
	return a.model.permissionMode.String()
}

func (a *commandAdapter) Status() command.StatusInfo {
	if a == nil || a.model == nil {
		return command.StatusInfo{}
	}
	return command.StatusInfo{
		WorkMode:       a.WorkMode(),
		PermissionMode: a.PermissionMode(),
		Model:          providerModel(a.model),
		Usage:          usageSnapshot(a.model),
		SessionID:      a.model.sessionCtx.ID,
		CWD:            a.model.cwd,
		RuntimeState:   stateLabel(a.model.state),
	}
}

func (a *commandAdapter) Session() command.SessionInfo {
	if a == nil || a.model == nil {
		return command.SessionInfo{}
	}
	return command.SessionInfo{
		ID:           a.model.sessionCtx.ID,
		JSONLPath:    a.model.sessionCtx.JSONLPath,
		MessageCount: a.model.conv.Len(),
		Model:        providerModel(a.model),
	}
}

func (a *commandAdapter) MemorySummary() string {
	if a == nil || a.model == nil || a.model.memory == nil {
		return ""
	}
	return a.model.memory.RefreshAndIndexText()
}

func (a *commandAdapter) TriggerCompact() {
	if a == nil || a.model == nil {
		return
	}
	next, cmd := a.model.startManualCompact()
	if model, ok := next.(Model); ok {
		*a.model = model
	}
	if cmd != nil {
		a.cmds = append(a.cmds, cmd)
	}
}

func (a *commandAdapter) ClearScreen() {
	if a == nil || a.model == nil {
		return
	}
	a.model.transcript = nil
	a.model.curReply = ""
	a.model.curTool = nil
	a.model.progress = ""
	a.model.lastStop = nil
	a.model.updateViewportContent()
}

func (a *commandAdapter) SendPresetUserMessage(displayLabel, prompt string) {
	if a == nil || a.model == nil {
		return
	}
	next, cmd := a.model.submitPresetText(displayLabel, prompt)
	if model, ok := next.(Model); ok {
		*a.model = model
	}
	if cmd != nil {
		a.cmds = append(a.cmds, cmd)
	}
}

func (a *commandAdapter) RefreshStatus() {
	if a == nil || a.model == nil {
		return
	}
	a.model.updateViewportContent()
}

func providerModel(m *Model) string {
	if m == nil || m.provider == nil {
		return ""
	}
	return m.provider.Model()
}

func usageSnapshot(m *Model) command.Usage {
	if m == nil || m.usage == nil {
		return command.Usage{}
	}
	return command.Usage{
		InputTokens:  m.usage.InputTokens,
		OutputTokens: m.usage.OutputTokens,
		TotalTokens:  m.usage.TotalTokens,
	}
}

func stateLabel(state sessionState) string {
	switch state {
	case stateSelecting:
		return "selecting"
	case stateIdle:
		return "idle"
	case stateStreaming:
		return "streaming"
	case stateApproving:
		return "approving"
	case stateResuming:
		return "resuming"
	default:
		return fmt.Sprintf("unknown(%d)", state)
	}
}
