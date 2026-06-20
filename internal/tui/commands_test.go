package tui

import (
	"strings"
	"testing"

	"PseudoClaude/internal/command"
	"PseudoClaude/internal/config"
	"PseudoClaude/internal/llm"
	"PseudoClaude/internal/skills"

	tea "charm.land/bubbletea/v2"
)

func TestCommandDispatchHelpStatusAndModes(t *testing.T) {
	model := New([]config.ProviderConfig{}, t.TempDir(), nil)
	before := model.conv.Len()

	model.textarea.SetValue("/Help")
	next, cmd := model.updateIdle(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = next.(Model)
	if cmd != nil {
		t.Fatal("unexpected command")
	}
	if model.conv.Len() != before || model.textarea.Value() != "" {
		t.Fatalf("help side effects conv=%d textarea=%q", model.conv.Len(), model.textarea.Value())
	}
	if len(model.transcript) == 0 || model.transcript[len(model.transcript)-1].kind != transcriptHelp || !strings.Contains(model.transcript[len(model.transcript)-1].text, "/status") {
		t.Fatalf("missing help transcript: %+v", model.transcript)
	}

	model.textarea.SetValue("/plan")
	next, _ = model.updateIdle(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = next.(Model)
	if !model.planMode || !strings.Contains(model.statusBar(model.contentWidth()), "[PLAN]") {
		t.Fatalf("/plan did not set plan mode")
	}

	model.textarea.SetValue("/do")
	next, _ = model.updateIdle(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = next.(Model)
	if model.planMode || !strings.Contains(model.statusBar(model.contentWidth()), "[DEFAULT]") {
		t.Fatalf("/do did not set default mode")
	}

	before = model.conv.Len()
	model.textarea.SetValue("/st")
	next, _ = model.updateIdle(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = next.(Model)
	if model.conv.Len() != before || !strings.Contains(model.transcript[len(model.transcript)-1].text, "Status") {
		t.Fatalf("status side effects conv=%d transcript=%+v", model.conv.Len(), model.transcript)
	}
}

func TestInvalidCommandUsageShowsHelpHint(t *testing.T) {
	model := New([]config.ProviderConfig{}, t.TempDir(), nil)
	model.textarea.SetValue("/help extra")
	next, cmd := model.updateIdle(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = next.(Model)
	if cmd != nil {
		t.Fatal("unexpected command")
	}
	if model.conv.Len() != 0 {
		t.Fatalf("conversation length = %d", model.conv.Len())
	}
	if len(model.transcript) == 0 || model.transcript[len(model.transcript)-1].kind != transcriptHelp {
		t.Fatalf("missing help transcript: %+v", model.transcript)
	}
	text := model.transcript[len(model.transcript)-1].text
	if !strings.Contains(text, "Invalid usage") || !strings.Contains(text, "/help") {
		t.Fatalf("unexpected help text: %q", text)
	}
	view := stripANSI(model.renderTranscriptEntry(model.transcript[len(model.transcript)-1], 80))
	if !strings.Contains(view, "Invalid usage") || strings.Contains(view, "Error") {
		t.Fatalf("help should render as help, not error: %q", view)
	}
}

func TestCommandClearKeepsConversation(t *testing.T) {
	model := New([]config.ProviderConfig{}, t.TempDir(), nil)
	model.conv.AddUser("hello")
	model.appendTranscript(transcriptEntry{kind: transcriptUser, text: "hello"})
	before := model.conv.Len()

	model.textarea.SetValue("/clear")
	next, _ := model.updateIdle(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = next.(Model)
	if model.conv.Len() != before {
		t.Fatalf("conversation length changed: %d -> %d", before, model.conv.Len())
	}
	if len(model.transcript) != 1 || !strings.Contains(model.transcript[0].text, "preserved") {
		t.Fatalf("clear transcript = %+v", model.transcript)
	}
}

func TestReviewSkillCommandRunsIsolated(t *testing.T) {
	work := t.TempDir()
	catalog := skills.LoadCatalog(skills.LoadOptions{WorkDir: work, HomeDir: t.TempDir()})
	model := New([]config.ProviderConfig{}, work, nil).WithSkills(catalog, skills.NewActiveSkills())
	model.provider = fakeProvider{events: []llm.StreamEvent{{Text: "reviewed"}, {Done: true}}}
	model.runner.Provider = model.provider

	model.textarea.SetValue("/review")
	next, cmd := model.updateIdle(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = next.(Model)
	if cmd == nil || model.state != stateStreaming {
		t.Fatalf("review skill should start streaming state=%v cmd=%v", model.state, cmd)
	}
	for model.state == stateStreaming {
		msg := waitForAgentEvent(model.events)()
		next, cmd = model.updateStreaming(msg)
		model = next.(Model)
		if cmd == nil && model.state == stateStreaming {
			t.Fatal("streaming without next command")
		}
	}
	msgs := model.conv.Messages()
	if len(msgs) < 2 || msgs[len(msgs)-1].Content != "reviewed" {
		t.Fatalf("conversation messages = %+v", msgs)
	}
	if len(model.transcript) == 0 || model.transcript[len(model.transcript)-1].text != "reviewed" {
		t.Fatalf("review transcript = %+v", model.transcript)
	}
}

func TestSkillCommandListsBuiltinSkills(t *testing.T) {
	work := t.TempDir()
	catalog := skills.LoadCatalog(skills.LoadOptions{WorkDir: work, HomeDir: t.TempDir()})
	model := New([]config.ProviderConfig{}, work, nil).WithSkills(catalog, skills.NewActiveSkills())

	model.textarea.SetValue("/skill")
	next, cmd := model.updateIdle(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = next.(Model)
	if cmd != nil {
		t.Fatal("unexpected command")
	}
	if len(model.transcript) == 0 {
		t.Fatal("missing transcript")
	}
	text := model.transcript[len(model.transcript)-1].text
	for _, want := range []string{"/commit", "/review", "/test"} {
		if !strings.Contains(text, want) {
			t.Fatalf("/skill output missing %s: %q", want, text)
		}
	}
}

func TestCommandAdapterSnapshots(t *testing.T) {
	model := New([]config.ProviderConfig{}, t.TempDir(), nil)
	model.provider = fakeProvider{model: "test-model"}
	model.usage = &llm.Usage{InputTokens: 1, OutputTokens: 2, TotalTokens: 3}
	model.planMode = true
	adapter := &commandAdapter{model: &model}

	status := adapter.Status()
	if status.WorkMode != command.WorkModePlan || status.Model != "test-model" || status.Usage.TotalTokens != 3 {
		t.Fatalf("status = %+v", status)
	}
	session := adapter.Session()
	if session.ID == "" || session.JSONLPath == "" || session.Model != "test-model" {
		t.Fatalf("session = %+v", session)
	}
}

func TestNonIdleCommandBoundaries(t *testing.T) {
	model := New([]config.ProviderConfig{}, t.TempDir(), nil)
	model.state = stateStreaming

	next, _, handled := model.dispatchInput("/compact")
	model = next
	if !handled || model.state != stateStreaming || len(model.transcript) == 0 || !strings.Contains(model.transcript[len(model.transcript)-1].text, "wait") {
		t.Fatalf("non-idle compact result state=%v transcript=%+v", model.state, model.transcript)
	}

	before := len(model.transcript)
	next, _, handled = model.dispatchInput("/status")
	model = next
	if !handled || len(model.transcript) <= before || !strings.Contains(model.transcript[len(model.transcript)-1].text, "Status") {
		t.Fatalf("non-idle status transcript=%+v", model.transcript)
	}
}
