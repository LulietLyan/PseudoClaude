package tui

import (
	"context"
	"fmt"

	"PseudoClaude/internal/command"
	"PseudoClaude/internal/subagent"
	"PseudoClaude/internal/worktree"

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
		CWD:            a.model.effectiveCWD(),
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

func (a *commandAdapter) ListSkills() []command.SkillSummary {
	if a == nil || a.model == nil || a.model.skillCatalog == nil {
		return nil
	}
	return skillSummaries(a.model.skillCatalog.Summaries())
}

func (a *commandAdapter) ListHooks() []command.HookSummary {
	if a == nil || a.model == nil || a.model.hookEngine == nil {
		return nil
	}
	summaries := a.model.hookEngine.Summaries()
	out := make([]command.HookSummary, 0, len(summaries))
	for _, summary := range summaries {
		out = append(out, command.HookSummary{
			Name:   summary.Name,
			Event:  summary.Event,
			Action: summary.Action,
			Flags:  append([]string(nil), summary.Flags...),
			Source: summary.Source,
		})
	}
	return out
}

func (a *commandAdapter) HookSources() []string {
	if a == nil || a.model == nil || a.model.hookEngine == nil {
		return nil
	}
	return a.model.hookEngine.Sources()
}

func (a *commandAdapter) ListAgents() []command.AgentSummary {
	if a == nil || a.model == nil || a.model.subagents == nil {
		return nil
	}
	defs := a.model.subagents.List()
	out := make([]command.AgentSummary, 0, len(defs))
	for _, def := range defs {
		out = append(out, agentSummary(def))
	}
	return out
}

func (a *commandAdapter) DescribeAgent(name string) (command.AgentDetail, bool) {
	if a == nil || a.model == nil || a.model.subagents == nil {
		return command.AgentDetail{}, false
	}
	active, ok := a.model.subagents.Resolve(name)
	if !ok {
		return command.AgentDetail{}, false
	}
	all := a.model.subagents.ListAll(name)
	var overridden []command.AgentSummary
	for _, def := range all {
		if def.Source == active.Source && def.Path == active.Path {
			continue
		}
		overridden = append(overridden, agentSummary(def))
	}
	return command.AgentDetail{Active: agentSummary(active), Overridden: overridden, Prompt: active.SystemPrompt}, true
}

func (a *commandAdapter) ReloadAgents() {
	if a == nil || a.model == nil || a.model.subagents == nil {
		return
	}
	result := a.model.subagents.Reload(subagent.LoadOptions{ProjectRoot: a.model.repoCWD})
	for _, warning := range result.Warnings {
		a.model.appendTranscript(transcriptEntry{kind: transcriptStatus, text: "Agent warning: " + subagent.FormatWarning(warning)})
	}
}

func (a *commandAdapter) WorktreeAvailable() bool {
	return a != nil && a.model != nil && a.model.worktrees != nil
}

func (a *commandAdapter) CreateWorktree(name string) (command.WorktreeSummary, error) {
	wt, err := a.model.worktrees.Create(context.Background(), worktree.CreateInput{Name: name, Manual: true})
	if err != nil {
		return command.WorktreeSummary{}, err
	}
	return worktreeSummary(worktree.Summary{Name: wt.Name, Path: wt.Path, Branch: wt.Branch, Manual: wt.Manual}), nil
}

func (a *commandAdapter) ListWorktrees() []command.WorktreeSummary {
	items := a.model.worktrees.List(context.Background())
	out := make([]command.WorktreeSummary, 0, len(items))
	for _, item := range items {
		out = append(out, worktreeSummary(item))
	}
	return out
}

func (a *commandAdapter) EnterWorktree(name string) (command.WorktreeSummary, error) {
	session, err := a.model.worktrees.Enter(context.Background(), name)
	if err != nil {
		return command.WorktreeSummary{}, err
	}
	a.model.setActiveCWD(session.WorktreePath)
	return command.WorktreeSummary{Name: session.WorktreeName, Path: session.WorktreePath, Active: true}, nil
}

func (a *commandAdapter) ExitWorktree(remove bool, discard bool) (command.WorktreeSummary, error) {
	report, err := a.model.worktrees.Exit(context.Background(), worktree.ExitOptions{Remove: remove, Discard: discard})
	if err != nil {
		return command.WorktreeSummary{}, err
	}
	a.model.setActiveCWD(a.model.repoCWD)
	return command.WorktreeSummary{Name: report.Name, Path: report.Path, Branch: report.Branch, Removed: report.Removed}, nil
}

func (a *commandAdapter) RemoveWorktree(name string, discard bool) (command.WorktreeSummary, error) {
	report, err := a.model.worktrees.Remove(context.Background(), name, worktree.RemoveOptions{Discard: discard})
	if err != nil {
		return command.WorktreeSummary{}, err
	}
	if a.model.effectiveCWD() == report.Path {
		a.model.setActiveCWD(a.model.repoCWD)
	}
	return command.WorktreeSummary{Name: report.Name, Path: report.Path, Branch: report.Branch, Removed: true}, nil
}

func worktreeSummary(item worktree.Summary) command.WorktreeSummary {
	return command.WorktreeSummary{
		Name:       item.Name,
		Path:       item.Path,
		Branch:     item.Branch,
		Manual:     item.Manual,
		Active:     item.Active,
		Dirty:      item.Dirty,
		DirtyError: item.DirtyError,
	}
}

func (a *commandAdapter) RunSkill(name, args string) error {
	if a == nil || a.model == nil {
		return nil
	}
	a.model.skillExecutor.Runner = &skillRunnerAdapter{model: a.model, cmds: &a.cmds}
	return a.model.skillExecutor.Execute(name, args)
}

func agentSummary(def subagent.Definition) command.AgentSummary {
	return command.AgentSummary{
		Name:            def.Name,
		Description:     def.Description,
		Source:          def.Source.String(),
		Model:           string(def.Model),
		MaxTurns:        def.MaxTurns,
		Background:      def.Background,
		Isolation:       string(def.Isolation),
		Tools:           append([]string(nil), def.Tools...),
		DisallowedTools: append([]string(nil), def.DisallowedTools...),
	}
}

func (a *commandAdapter) ReloadSkills() {
	if a == nil || a.model == nil {
		return
	}
	a.model.reloadSkills()
}

func (a *commandAdapter) ClearActiveSkills() {
	if a == nil || a.model == nil || a.model.activeSkills == nil {
		return
	}
	a.model.activeSkills.Clear()
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
