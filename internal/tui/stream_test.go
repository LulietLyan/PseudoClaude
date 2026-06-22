package tui

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"PseudoClaude/internal/agent"
	"PseudoClaude/internal/compact"
	"PseudoClaude/internal/config"
	"PseudoClaude/internal/hook"
	"PseudoClaude/internal/llm"
	"PseudoClaude/internal/memory"
	"PseudoClaude/internal/permission"
	"PseudoClaude/internal/session"
	"PseudoClaude/internal/skills"
	"PseudoClaude/internal/tools"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
)

func TestNewStartsTextareaFocusedAndAcceptsInput(t *testing.T) {
	model := New(nil, t.TempDir(), nil)
	if !model.textarea.Focused() {
		t.Fatal("textarea should start focused")
	}
	next, _ := model.updateIdle(tea.KeyPressMsg{Code: 'h', Text: "h"})
	model = next.(Model)
	next, _ = model.updateIdle(tea.KeyPressMsg{Code: 'i', Text: "i"})
	model = next.(Model)
	if model.textarea.Value() != "hi" {
		t.Fatalf("textarea value = %q, want hi", model.textarea.Value())
	}
}

func TestBannerUsesSmallLogoInSmallViewport(t *testing.T) {
	model := New(nil, t.TempDir(), nil)
	model.width = 30
	model.height = 8
	banner := model.bannerView()
	if !strings.Contains(banner, "PseudoClaude") {
		t.Fatalf("banner missing logo text: %q", banner)
	}
	if strings.Contains(banner, "terminal agent") {
		t.Fatalf("small banner should use tiny logo, got: %q", banner)
	}
}

func TestBannerCentersFileLogoAndMeta(t *testing.T) {
	model := New(nil, t.TempDir(), nil)
	model.width = 90
	model.height = 24
	banner := model.bannerView()
	plain := stripANSI(banner)
	if !strings.Contains(plain, "┏") || !strings.Contains(plain, "┗") {
		t.Fatalf("banner missing frame: %q", banner)
	}
	if !strings.Contains(banner, "██████╗") {
		t.Fatalf("banner missing file logo: %q", banner)
	}
	if !strings.Contains(banner, "PseudoClaude v"+Version) || !strings.Contains(banner, "Ready. Shift+Tab") {
		t.Fatalf("banner missing centered meta lines: %q", banner)
	}
	lines := strings.Split(banner, "\n")
	if len(lines) == 0 || !strings.HasPrefix(lines[0], " ") {
		t.Fatalf("first logo line should be centered with left padding: %q", banner)
	}
}

func TestBannerFrameCentersOnWideTerminal(t *testing.T) {
	model := New(nil, "/tmp/pseudoclaude", nil)
	model.width = 160
	model.height = 40
	banner := stripANSI(model.bannerView())
	lines := strings.Split(banner, "\n")
	if len(lines) == 0 {
		t.Fatal("banner has no lines")
	}
	if got := lipgloss.Width(lines[0]); got != model.width {
		t.Fatalf("banner line width = %d, want %d: %q", got, model.width, lines[0])
	}
	leftPad := strings.Index(lines[0], "┏")
	if leftPad <= 20 {
		t.Fatalf("frame not visually centered on wide terminal, left pad=%d line=%q", leftPad, lines[0])
	}
}

func TestBannerRerendersWhileIdleAndAfterReply(t *testing.T) {
	model := New([]config.ProviderConfig{}, t.TempDir(), nil)
	model.width = 90
	model.height = 24
	next, _ := model.updateIdle(tea.KeyPressMsg{Code: 'h', Text: "h"})
	model = next.(Model)
	view := model.view()
	if !strings.Contains(view, "██████╗") {
		t.Fatalf("banner should rerender while idle/typing: %q", view)
	}
	model = model.handleResize(tea.WindowSizeMsg{Width: 160, Height: 40})
	view = model.view()
	lines := strings.Split(view, "\n")
	if len(lines) == 0 || lipgloss.Width(lines[0]) != 160 {
		t.Fatalf("banner should use resized terminal width: %q", view)
	}

	model.state = stateStreaming
	model.turnStart = time.Now()
	next, _ = model.updateStreaming(agentMsg{Type: agent.EventTextDelta, Text: "hello"})
	model = next.(Model)
	next, _ = model.updateStreaming(agentMsg{Type: agent.EventStop, Stop: &agent.Stop{Reason: agent.StopCompleted}})
	model = next.(Model)
	if model.state != stateIdle {
		t.Fatalf("state = %v, want idle", model.state)
	}
	view = model.view()
	if !strings.Contains(view, "██████╗") {
		t.Fatalf("banner should remain visible after reply: %q", view)
	}
}

func TestTranscriptRendersAfterBanner(t *testing.T) {
	model := New([]config.ProviderConfig{}, t.TempDir(), nil)
	model.width = 120
	model.height = 50
	model.appendTranscript(transcriptEntry{kind: transcriptUser, text: "hello"})
	model.appendTranscript(transcriptEntry{kind: transcriptAssistant, text: "world", elapsed: time.Second})
	model.stickToBottom = false
	model.viewport.GotoTop()
	view := model.view()
	bannerIndex := strings.Index(view, "PseudoClaude v"+Version)
	userIndex := strings.Index(view, "hello")
	assistantIndex := strings.Index(view, "world")
	if bannerIndex < 0 || userIndex < 0 || assistantIndex < 0 {
		t.Fatalf("view missing expected blocks: %q", view)
	}
	if !(bannerIndex < userIndex && userIndex < assistantIndex) {
		t.Fatalf("transcript should render after banner: banner=%d user=%d assistant=%d view=%q", bannerIndex, userIndex, assistantIndex, view)
	}
}

func TestUserAndAssistantBlocksUseConsistentChrome(t *testing.T) {
	user := stripANSI(userBlock("hello", 50))
	assistant := stripANSI(assistantBlock("world", time.Second, 50, nil))
	for _, rendered := range []string{user, assistant} {
		if !strings.Contains(rendered, "━") || !strings.Contains(rendered, "┃") {
			t.Fatalf("block should use thick border: %q", rendered)
		}
	}
	userLines := strings.Split(user, "\n")
	assistantLines := strings.Split(assistant, "\n")
	if len(userLines) < 6 || len(assistantLines) < 6 {
		t.Fatalf("unexpected blocks:\n%s\n%s", user, assistant)
	}
	if userLines[1] != assistantLines[1] || strings.Contains(userLines[1], "You") || strings.Contains(assistantLines[1], "Assistant") {
		t.Fatalf("top padding should match:\nuser=%q\nassistant=%q", userLines[1], assistantLines[1])
	}
	if !strings.Contains(userLines[2], "You") || !strings.Contains(userLines[4], "hello") {
		t.Fatalf("user label/body spacing is wrong: %q", user)
	}
	if !strings.Contains(assistantLines[2], "Assistant") || !strings.Contains(assistantLines[4], "world") {
		t.Fatalf("assistant label/body spacing is wrong: %q", assistant)
	}
}

func TestBannerUsesThickFrame(t *testing.T) {
	model := New(nil, t.TempDir(), nil)
	model.width = 90
	model.height = 24
	banner := model.bannerView()
	if !strings.Contains(banner, "┏") || !strings.Contains(banner, "┗") || !strings.Contains(banner, "━") {
		t.Fatalf("banner should use thick frame: %q", banner)
	}
}

func TestStreamingTextDeltasDoNotPanicAfterModelCopies(t *testing.T) {
	model := New([]config.ProviderConfig{}, t.TempDir(), nil)
	model.state = stateStreaming
	model.turnStart = time.Now()
	var next tea.Model

	next, _ = model.updateStreaming(agentMsg{Type: agent.EventTextDelta, Text: "hel"})
	model = next.(Model)
	next, _ = model.updateStreaming(agentMsg{Type: agent.EventTextDelta, Text: "lo"})
	model = next.(Model)

	if model.curReply != "hello" {
		t.Fatalf("curReply = %q, want hello", model.curReply)
	}
}

func TestInstallSkillToolResultRefreshesSkillCommands(t *testing.T) {
	work := t.TempDir()
	catalog := skills.LoadCatalog(skills.LoadOptions{WorkDir: work, HomeDir: t.TempDir()})
	model := New([]config.ProviderConfig{}, work, nil).WithSkills(catalog, skills.NewActiveSkills())
	if items := model.commandRegistry.Complete("/demo"); len(items) != 0 {
		t.Fatalf("demo should not exist yet: %+v", items)
	}
	dir := filepath.Join(work, ".PseudoClaude", "skills", "demo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: demo\ndescription: Demo skill.\n---\nBody"), 0o644); err != nil {
		t.Fatal(err)
	}
	model.events = make(chan agent.Event)
	model.turnStart = time.Now()
	next, _ := model.handleAgentEvent(agent.Event{
		Type: agent.EventToolResult,
		ToolResult: &agent.ToolResult{
			Result: tools.Success("install_skill", "installed", nil),
		},
	})
	model = next.(Model)
	items := model.commandRegistry.Complete("/demo")
	if len(items) != 1 || items[0].Name != "/demo" || !items[0].Skill {
		t.Fatalf("demo completion = %+v", items)
	}
}

func TestViewDoesNotInsertLargeGapBeforeInput(t *testing.T) {
	model := New(nil, t.TempDir(), nil)
	model.width = 80
	model.height = 30
	view := model.view()
	if strings.Contains(view, "\n\n\n\n") {
		t.Fatalf("view has excessive vertical gap: %q", view)
	}
}

func TestViewCanScrollTranscriptHistory(t *testing.T) {
	model := New(nil, t.TempDir(), nil)
	model.width = 80
	model.height = 12
	model.showBanner = false
	for i := 0; i < 20; i++ {
		model.appendTranscript(transcriptEntry{kind: transcriptStatus, text: "old status block"})
	}
	model.appendTranscript(transcriptEntry{kind: transcriptAssistant, text: "latest assistant output", elapsed: time.Second})

	view := model.view()
	if strings.Contains(view, "old status block") {
		t.Fatalf("old transcript should be cropped out: %q", view)
	}
	if !strings.Contains(view, "latest assistant output") {
		t.Fatalf("latest output should remain visible: %q", view)
	}
	if !strings.Contains(view, "DEFAULT") {
		t.Fatalf("status/input area should remain visible: %q", view)
	}
	if lines := strings.Split(view, "\n"); len(lines) > model.height {
		t.Fatalf("view height = %d, want <= %d\n%s", len(lines), model.height, view)
	}

	model.stickToBottom = false
	model.viewport.GotoTop()
	view = model.view()
	if !strings.Contains(view, "old status block") {
		t.Fatalf("scrolling to top should reveal old transcript: %q", view)
	}
	if !strings.Contains(model.statusBar(model.contentWidth()), "SCROLLED") {
		t.Fatalf("status should indicate scrolled mode")
	}
	model.stickToBottom = true
	model.viewport.GotoBottom()
	if !model.viewport.AtBottom() {
		t.Fatalf("viewport should be at bottom")
	}
}

func TestStreamingReplyLivesInScrollableViewport(t *testing.T) {
	model := New(nil, t.TempDir(), nil)
	model.width = 80
	model.height = 12
	model.showBanner = false
	model.state = stateStreaming
	model.turnStart = time.Now()
	model.curReply = "reply-start\n" + strings.Repeat("middle line\n", 30) + "reply-end"

	view := model.view()
	if strings.Contains(view, "reply-start") {
		t.Fatalf("default bottom view should not show top of long stream: %q", view)
	}
	if !strings.Contains(view, "reply-end") {
		t.Fatalf("default bottom view should show stream tail: %q", view)
	}

	model.stickToBottom = false
	model.viewport.GotoTop()
	view = model.view()
	if !strings.Contains(view, "reply-start") {
		t.Fatalf("scrolling up should reveal stream head: %q", view)
	}
}

func TestMacFriendlyScrollKeysAdjustTranscriptOffset(t *testing.T) {
	model := New(nil, t.TempDir(), nil)
	model.width = 80
	model.height = 10
	model.showBanner = false
	for i := 0; i < 20; i++ {
		model.appendTranscript(transcriptEntry{kind: transcriptStatus, text: "status block"})
	}

	next, cmd := model.Update(tea.KeyPressMsg{Code: 'u', Mod: uv.ModCtrl})
	model = next.(Model)
	if cmd != nil {
		t.Fatal("scroll key should not return command")
	}
	if model.viewport.YOffset() == 0 {
		t.Fatal("ctrl+u should increase scroll offset")
	}
	upOffset := model.viewport.YOffset()
	next, _ = model.Update(tea.KeyPressMsg{Code: 'd', Mod: uv.ModCtrl})
	model = next.(Model)
	if model.viewport.YOffset() <= upOffset {
		t.Fatalf("ctrl+d should move back down, got before=%d after=%d", upOffset, model.viewport.YOffset())
	}
	next, _ = model.Update(tea.KeyPressMsg{Code: 't', Mod: uv.ModCtrl})
	model = next.(Model)
	if model.viewport.YOffset() != 0 {
		t.Fatal("ctrl+t should jump to top")
	}
	model.stickToBottom = true
	model.viewport.GotoBottom()
	next, _ = model.Update(tea.KeyPressMsg{Code: 'b', Mod: uv.ModCtrl})
	model = next.(Model)
	if !model.viewport.AtBottom() {
		t.Fatalf("ctrl+b should return to bottom, got %d", model.viewport.YOffset())
	}
}

type fakeProvider struct {
	events []llm.StreamEvent
	model  string
}

func (f fakeProvider) Name() string { return "fake" }
func (f fakeProvider) Model() string {
	if f.model != "" {
		return f.model
	}
	return "fake-model"
}
func (f fakeProvider) Stream(_ context.Context, _ llm.Request) <-chan llm.StreamEvent {
	ch := make(chan llm.StreamEvent, len(f.events))
	for _, event := range f.events {
		ch <- event
	}
	close(ch)
	return ch
}

func TestAgentEventToolResultReturnsIdleAfterStop(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "note.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	registry, err := tools.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	model := New([]config.ProviderConfig{}, dir, registry)
	model.state = stateStreaming
	model.turnStart = time.Now()
	var next tea.Model
	next, _ = model.updateStreaming(agentMsg{Type: agent.EventToolCallStart, ToolCall: &llm.ToolCall{ID: "call_1", Name: "read_file"}})
	model = next.(Model)
	result := tools.Success("read_file", "hello", nil)
	next, _ = model.updateStreaming(agentMsg{Type: agent.EventToolResult, ToolResult: &agent.ToolResult{
		Call:   llm.ToolCall{ID: "call_1", Name: "read_file"},
		Result: result,
	}})
	model = next.(Model)
	next, _ = model.updateStreaming(agentMsg{Type: agent.EventTextDelta, Text: "done"})
	model = next.(Model)
	next, _ = model.updateStreaming(agentMsg{Type: agent.EventStop, Stop: &agent.Stop{Reason: agent.StopCompleted}})
	model = next.(Model)
	if model.state != stateIdle {
		t.Fatalf("state = %v, want idle", model.state)
	}
	if model.curTool != nil {
		t.Fatalf("curTool = %+v, want nil", model.curTool)
	}
}

func TestUserPromptSubmitHookBlocks(t *testing.T) {
	engine := hook.NewEngine([]hook.Rule{{
		Name:  "block-delete",
		Event: hook.EventUserPromptSubmit,
		If: &hook.Condition{Mode: hook.CombineAllOf, Atoms: []hook.Atom{{
			Field:   "prompt",
			Matcher: mustTUIMatcher(t, "~(?i)delete"),
		}}},
		Action: hook.Action{Type: hook.ActionShell, Shell: &hook.ShellAction{Command: `echo "no delete" >&2; exit 2`}},
	}}, hook.Executor{}, nil)
	model := New([]config.ProviderConfig{}, t.TempDir(), nil).WithHooks(engine)
	model.provider = fakeProvider{events: []llm.StreamEvent{{Text: "should not run"}, {Done: true}}}
	before := model.conv.Len()
	next, cmd := model.submitAgentTextWithTools("please delete it", "", nil)
	model = next.(Model)
	if cmd != nil || model.state != stateIdle || model.conv.Len() != before {
		t.Fatalf("blocked submission state=%v cmd=%v conv=%d/%d", model.state, cmd, model.conv.Len(), before)
	}
	if len(model.transcript) == 0 || !strings.Contains(model.transcript[len(model.transcript)-1].text, "[hook block-delete] no delete") {
		t.Fatalf("transcript = %+v", model.transcript)
	}
}

func mustTUIMatcher(t *testing.T, pattern string) permission.Matcher {
	t.Helper()
	matcher, err := permission.CompileMatcher(pattern)
	if err != nil {
		t.Fatal(err)
	}
	return matcher
}

func TestPlanAndDoInputHandling(t *testing.T) {
	model := New([]config.ProviderConfig{}, t.TempDir(), nil)
	model.provider = fakeProvider{events: []llm.StreamEvent{{Text: "plan text"}, {Done: true}}}
	model.runner.Provider = model.provider
	model.planMode = true

	req, printable, err := model.requestForInput("change the thing")
	if err != nil {
		t.Fatal(err)
	}
	if req.Mode != agent.ModePlan || req.PlanTask != "change the thing" || printable != "change the thing" {
		t.Fatalf("plan request = %+v printable=%q", req, printable)
	}
	model.planMode = false
	req, printable, err = model.requestForInput("do the thing")
	if err != nil {
		t.Fatal(err)
	}
	if req.Mode != agent.ModeChat || req.UserText != "do the thing" || printable != "do the thing" {
		t.Fatalf("chat request = %+v printable=%q", req, printable)
	}
}

func TestUnknownSlashCommandDoesNotSubmitUserMessage(t *testing.T) {
	model := New([]config.ProviderConfig{}, t.TempDir(), nil)
	model.textarea.SetValue("/unknown")
	next, cmd := model.updateIdle(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = next.(Model)
	if cmd != nil {
		t.Fatal("unexpected command")
	}
	if model.conv.Len() != 0 {
		t.Fatalf("conversation length = %d", model.conv.Len())
	}
	if len(model.transcript) == 0 || model.transcript[len(model.transcript)-1].kind != transcriptHelp || !strings.Contains(model.transcript[len(model.transcript)-1].text, "/help") {
		t.Fatalf("missing command help transcript: %+v", model.transcript)
	}
}

func TestManualCompactCommandDoesNotAddUserMessage(t *testing.T) {
	model := New([]config.ProviderConfig{}, t.TempDir(), nil)
	model.provider = fakeProvider{events: []llm.StreamEvent{{Text: "<analysis>x</analysis><summary>short summary</summary>"}, {Done: true}}}
	runtime, err := compact.NewRuntime(t.TempDir(), 1000)
	if err != nil {
		t.Fatal(err)
	}
	model.compactRuntime = runtime
	model.conv.AddUser("hello")
	before := model.conv.Len()

	model.textarea.SetValue("/compact")
	next, cmd := model.updateIdle(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = next.(Model)
	if model.conv.Len() != before {
		t.Fatalf("/compact added a user message: before=%d after=%d", before, model.conv.Len())
	}
	if cmd == nil || model.state != stateStreaming {
		t.Fatalf("state=%v cmd=%v", model.state, cmd)
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, inner := range batch {
			result := inner()
			if compactResult, ok := result.(compactMsg); ok {
				next, _ = model.updateStreaming(compactResult)
				model = next.(Model)
				break
			}
		}
	}
	if model.state != stateIdle {
		t.Fatalf("state = %v", model.state)
	}
	if !strings.Contains(model.transcript[len(model.transcript)-1].text, "estimated tokens") {
		t.Fatalf("missing compact success transcript: %+v", model.transcript)
	}
}

func TestMemoryCommandShowsIndexWithoutSubmitting(t *testing.T) {
	project := t.TempDir()
	user := t.TempDir()
	manager := memory.NewManager(filepath.Join(project, ".PseudoClaude", "memory"), user)
	if err := os.MkdirAll(user, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(user, memory.IndexFileName), []byte("- [user_preference] User Profile — Go engineer\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	model := New([]config.ProviderConfig{}, project, nil).WithPersistentContext("", manager)
	before := model.conv.Len()
	model.textarea.SetValue("/memory")
	next, cmd := model.updateIdle(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = next.(Model)
	if cmd != nil {
		t.Fatal("unexpected command")
	}
	if model.conv.Len() != before {
		t.Fatalf("/memory added conversation message: before=%d after=%d", before, model.conv.Len())
	}
	if len(model.transcript) == 0 || !strings.Contains(model.transcript[len(model.transcript)-1].text, "Go engineer") {
		t.Fatalf("missing memory transcript: %+v", model.transcript)
	}
}

func TestMemoryCommandEmptyState(t *testing.T) {
	model := New([]config.ProviderConfig{}, t.TempDir(), nil)
	model.textarea.SetValue("/memory")
	next, _ := model.updateIdle(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = next.(Model)
	if len(model.transcript) == 0 || !strings.Contains(model.transcript[len(model.transcript)-1].text, "暂无长期记忆") {
		t.Fatalf("missing empty memory state: %+v", model.transcript)
	}
}

func TestResumeLightweightSelection(t *testing.T) {
	workspace := t.TempDir()
	createTestSession(t, workspace, time.Date(2026, 6, 18, 10, 0, 0, 0, time.Local), "first")
	createTestSession(t, workspace, time.Date(2026, 6, 18, 11, 0, 0, 0, time.Local), "second")
	model := New([]config.ProviderConfig{}, workspace, nil)

	next, cmd := model.startResume()
	model = next.(Model)
	if cmd != nil {
		t.Fatal("unexpected command")
	}
	if model.state != stateResuming || len(model.resumeChoices) != 2 || model.resumeCursor != 0 {
		t.Fatalf("state=%v choices=%d cursor=%d", model.state, len(model.resumeChoices), model.resumeCursor)
	}
	view := model.view()
	if !strings.Contains(view, "Resume session") || !strings.Contains(view, "second") || strings.Contains(view, "Filter") {
		t.Fatalf("resume view = %q", view)
	}

	next, _ = model.updateResuming(tea.KeyPressMsg{Code: 'j', Text: "j"})
	model = next.(Model)
	if model.resumeCursor != 1 {
		t.Fatalf("cursor = %d", model.resumeCursor)
	}
	next, _ = model.updateResuming(tea.KeyPressMsg{Code: '1', Text: "1"})
	model = next.(Model)
	if model.state != stateIdle || !strings.Contains(model.transcript[len(model.transcript)-1].text, "已恢复会话") {
		t.Fatalf("resume failed state=%v transcript=%+v", model.state, model.transcript)
	}
}

func TestResumeEscCancels(t *testing.T) {
	workspace := t.TempDir()
	createTestSession(t, workspace, time.Date(2026, 6, 18, 10, 0, 0, 0, time.Local), "first")
	model := New([]config.ProviderConfig{}, workspace, nil)
	next, _ := model.startResume()
	model = next.(Model)
	next, _ = model.updateResuming(tea.KeyPressMsg{Code: tea.KeyEsc})
	model = next.(Model)
	if model.state != stateIdle || len(model.resumeChoices) != 0 {
		t.Fatalf("state=%v choices=%d", model.state, len(model.resumeChoices))
	}
}

func TestPlanModePersistsForPlainFollowupInput(t *testing.T) {
	model := New([]config.ProviderConfig{}, t.TempDir(), nil)
	model.provider = fakeProvider{events: []llm.StreamEvent{{Text: "plan text"}, {Done: true}}}
	model.runner.Provider = model.provider
	model.planMode = true

	req, printable, err := model.requestForInput("我要做个电商系统")
	if err != nil {
		t.Fatal(err)
	}
	if req.Mode != agent.ModePlan || req.PlanTask != "我要做个电商系统" || printable != "我要做个电商系统" {
		t.Fatalf("request = %+v printable=%q", req, printable)
	}
}

func createTestSession(t *testing.T, workspace string, now time.Time, text string) session.Context {
	t.Helper()
	ctx, err := session.NewContext(workspace, now)
	if err != nil {
		t.Fatal(err)
	}
	writer, err := session.NewWriter(ctx, "test-model", nil)
	if err != nil {
		t.Fatal(err)
	}
	writer.AppendMessage(llm.Message{Role: "user", Content: text})
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return ctx
}

func TestPlanStateSavedAndCleared(t *testing.T) {
	model := New([]config.ProviderConfig{}, t.TempDir(), nil)
	model.conv.AddUser("Plan mode. Create an implementation plan.\n\nTask:\nchange it")
	model.saveOrClearPlan("plan text")
	if model.lastPlan == nil || model.lastPlan.task != "change it" || model.lastPlan.text != "plan text" {
		t.Fatalf("lastPlan = %+v", model.lastPlan)
	}
	model.conv.AddUser("Execution mode. Carry out the following task.\n\nOriginal task:\nchange it\n\nPlan:\nplan text")
	model.saveOrClearPlan("done")
	if model.lastPlan != nil {
		t.Fatalf("lastPlan should be cleared: %+v", model.lastPlan)
	}
}

func TestPermissionModeSwitchAndRequest(t *testing.T) {
	model := New([]config.ProviderConfig{}, t.TempDir(), nil)
	if model.permissionMode != permission.ModeDefault {
		t.Fatalf("initial mode = %s", model.permissionMode)
	}
	want := []permission.Mode{permission.ModeAcceptEdits, permission.ModeBypassPermissions, permission.ModeStrict, permission.ModeDefault}
	for _, mode := range want {
		next, _ := model.updateIdle(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab, Mod: uv.ModShift}))
		model = next.(Model)
		if model.permissionMode != mode {
			t.Fatalf("mode = %s, want %s", model.permissionMode, mode)
		}
	}
	model.permissionMode = permission.ModeAcceptEdits
	req, _, err := model.requestForInput("hi")
	if err != nil {
		t.Fatal(err)
	}
	if req.PermissionMode != permission.ModeAcceptEdits {
		t.Fatalf("request permission mode = %s", req.PermissionMode)
	}
	req, _, err = model.requestForInput("/plan inspect")
	if err != nil || req.PermissionMode != permission.ModeAcceptEdits || req.Mode != agent.ModeChat {
		t.Fatalf("plan request = %+v", req)
	}
}

func TestApprovalInteraction(t *testing.T) {
	model := New([]config.ProviderConfig{}, t.TempDir(), nil)
	model.state = stateStreaming
	model.turnStart = time.Now()
	req := &agent.ApprovalRequest{
		Call:        llm.ToolCall{ID: "call", Name: "write_file"},
		Summary:     "note.txt",
		Reason:      "default mode requires ask",
		Result:      permission.CheckResult{Decision: permission.DecisionAsk, Source: "mode"},
		SourceLabel: "explore/task-1",
		Respond:     make(chan permission.ApprovalDecision, 1),
	}
	next, cmd := model.handleAgentEvent(agent.Event{Type: agent.EventApproval, Approval: req})
	model = next.(Model)
	if cmd != nil || model.state != stateApproving || model.pendingApproval != req {
		t.Fatalf("approval state=%v pending=%v cmd=%v", model.state, model.pendingApproval != nil, cmd)
	}
	next, _ = model.updateApproving(tea.KeyPressMsg{Code: 'j', Text: "j"})
	model = next.(Model)
	if model.approvalCursor != 1 {
		t.Fatalf("cursor = %d", model.approvalCursor)
	}
	if block := stripANSI(model.approvalBlock(80)); !strings.Contains(block, "SubAgent explore/task-1") {
		t.Fatalf("approval block missing source: %q", block)
	}
	next, cmd = model.updateApproving(tea.KeyPressMsg{Code: '3', Text: "3"})
	model = next.(Model)
	if model.state != stateStreaming || model.pendingApproval != nil {
		t.Fatalf("state=%v pending=%v", model.state, model.pendingApproval)
	}
	if cmd == nil {
		t.Fatal("expected approval command")
	}
	runCmd(sendApprovalDecision(req, permission.ApprovalAllowForever))
	select {
	case got := <-req.Respond:
		if got != permission.ApprovalAllowForever {
			t.Fatalf("decision = %s", got)
		}
	default:
		t.Fatal("missing approval decision")
	}
}

func runCmd(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, inner := range batch {
			runCmd(inner)
		}
	}
}

func TestStatusBarShowsPermissionMode(t *testing.T) {
	model := New([]config.ProviderConfig{}, t.TempDir(), nil)
	model.provider = fakeProvider{}
	model.permissionMode = permission.ModeStrict
	model.planMode = true
	status := model.statusBar(model.contentWidth())
	if !strings.Contains(status, "STRICT") || !strings.Contains(status, "[PLAN]") {
		t.Fatalf("status missing mode/plan: %q", status)
	}
	if strings.Contains(status, " fake ") {
		t.Fatalf("status should not show provider name: %q", status)
	}
}

func TestStatusBarTruncatesLongModelName(t *testing.T) {
	model := New([]config.ProviderConfig{}, t.TempDir(), nil)
	model.width = 80
	model.provider = fakeProvider{model: strings.Repeat("very-long-model-name-", 8)}
	status := model.statusBar(40)
	if lipgloss.Height(status) != 1 {
		t.Fatalf("status should stay on one line: %q", status)
	}
	for _, line := range strings.Split(status, "\n") {
		if lipgloss.Width(line) > 40 {
			t.Fatalf("status line overflow width=%d: %q", lipgloss.Width(line), line)
		}
	}
}

func TestStatusBarDoesNotWrapAtNarrowWidths(t *testing.T) {
	model := New([]config.ProviderConfig{}, t.TempDir(), nil)
	model.width = 24
	model.provider = fakeProvider{model: strings.Repeat("oversized-model-name-", 6)}
	model.permissionMode = permission.ModeBypassPermissions
	model.planMode = true
	model.lastStop = &agent.Stop{Reason: "max_tokens"}
	status := model.statusBar(18)
	if lipgloss.Height(status) != 1 {
		t.Fatalf("status should stay on one line: %q", status)
	}
	if lipgloss.Width(status) > 18 {
		t.Fatalf("status overflow width=%d: %q", lipgloss.Width(status), status)
	}
}

func TestInputAndStatusUseCenteredColumn(t *testing.T) {
	model := New([]config.ProviderConfig{}, t.TempDir(), nil)
	model.width = 140
	if got, want := model.contentWidth(), 126; got != want {
		t.Fatalf("content width = %d, want %d", got, want)
	}
	view := stripANSI(model.view())
	lines := strings.Split(view, "\n")
	var inputLine, statusLine string
	for _, line := range lines {
		if strings.Contains(line, "┏") && strings.Contains(line, "━") && !strings.Contains(line, "PseudoClaude") {
			inputLine = line
		}
		if strings.Contains(line, "DEFAULT") {
			statusLine = line
		}
	}
	if strings.Index(inputLine, "┏") < 5 {
		t.Fatalf("input should be centered away from edge: %q", inputLine)
	}
	if strings.Index(statusLine, "DEFAULT") < 5 {
		t.Fatalf("status should be centered away from edge: %q", statusLine)
	}
}

var ansiRE = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(s string) string {
	return ansiRE.ReplaceAllString(s, "")
}
