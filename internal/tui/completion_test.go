package tui

import (
	"strings"
	"testing"

	"PseudoClaude/internal/command"

	tea "charm.land/bubbletea/v2"
)

func TestCompletionSingleMultipleAndNoMatch(t *testing.T) {
	model := New(nil, t.TempDir(), nil)

	model.textarea.SetValue("/st")
	next, _, ok := model.handleCompletionKey(tea.KeyPressMsg{Code: tea.KeyTab})
	model = next
	if !ok || model.textarea.Value() != "/status" || model.completion.active {
		t.Fatalf("single completion value=%q state=%+v", model.textarea.Value(), model.completion)
	}

	model.textarea.SetValue("/s")
	next, _, ok = model.handleCompletionKey(tea.KeyPressMsg{Code: tea.KeyTab})
	model = next
	if !ok || !model.completion.active || !model.completion.manual || len(model.completion.items) < 2 {
		t.Fatalf("multi completion state=%+v", model.completion)
	}
	view := stripANSI(model.completionView(80))
	if !strings.Contains(view, "/session") || !strings.Contains(view, "/status") {
		t.Fatalf("completion view = %q", view)
	}

	model.textarea.SetValue("/zz")
	next, _, ok = model.handleCompletionKey(tea.KeyPressMsg{Code: tea.KeyTab})
	model = next
	if !ok || model.textarea.Value() != "/zz" || !strings.Contains(model.completionView(80), "No matching") {
		t.Fatalf("no match value=%q state=%+v", model.textarea.Value(), model.completion)
	}
}

func TestCompletionMenuNavigationAndSync(t *testing.T) {
	model := New(nil, t.TempDir(), nil)
	model.textarea.SetValue("/s")
	next, _, _ := model.handleCompletionKey(tea.KeyPressMsg{Code: tea.KeyTab})
	model = next
	next, _, _ = model.handleCompletionKey(tea.KeyPressMsg{Code: tea.KeyDown})
	model = next
	next, _, _ = model.handleCompletionKey(tea.KeyPressMsg{Code: tea.KeyDown})
	model = next
	if model.completion.cursor != 2 {
		t.Fatalf("cursor = %d", model.completion.cursor)
	}
	next, _, _ = model.handleCompletionKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = next
	if model.textarea.Value() != "/status" || model.completion.active {
		t.Fatalf("selected value=%q state=%+v", model.textarea.Value(), model.completion)
	}

	model.textarea.SetValue("/s")
	next, _, _ = model.handleCompletionKey(tea.KeyPressMsg{Code: tea.KeyTab})
	model = next
	model.textarea.SetValue("s")
	model = model.updateCompletionFromInput()
	if model.completion.active {
		t.Fatalf("completion should close after slash deletion")
	}

	model.textarea.SetValue("/s\nmore")
	next, _, ok := model.handleCompletionKey(tea.KeyPressMsg{Code: tea.KeyTab})
	model = next
	if ok || model.completion.active {
		t.Fatalf("multiline should not complete")
	}
}

func TestCompletionAutoSuggestsWhileTypingSlashCommands(t *testing.T) {
	model := New(nil, t.TempDir(), nil)

	model.textarea.SetValue("/")
	model = model.updateCompletionFromInput()
	if !model.completion.active || model.completion.manual || len(model.completion.items) == 0 {
		t.Fatalf("slash should auto-show suggestions: %+v", model.completion)
	}
	view := stripANSI(model.completionView(80))
	if !strings.Contains(view, "/clear") || !strings.Contains(view, "/help") {
		t.Fatalf("slash suggestions = %q", view)
	}

	model.textarea.SetValue("/s")
	model = model.updateCompletionFromInput()
	if !model.completion.active || model.completion.manual {
		t.Fatalf("prefix should keep auto suggestions: %+v", model.completion)
	}
	view = stripANSI(model.completionView(80))
	if !strings.Contains(view, "/session") || !strings.Contains(view, "/skill") || !strings.Contains(view, "/status") || strings.Contains(view, "/help") {
		t.Fatalf("prefix suggestions = %q", view)
	}

	next, _, ok := model.handleCompletionKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = next
	if ok || model.textarea.Value() != "/s" {
		t.Fatalf("auto suggestions should not consume enter: ok=%v value=%q", ok, model.textarea.Value())
	}
}

func TestCompletionShowsSkillMarker(t *testing.T) {
	model := New(nil, t.TempDir(), nil)
	errs := command.RegisterSkillCommands(model.commandRegistry, []command.SkillSummary{{Name: "demo", Description: "Demo skill."}})
	if len(errs) != 0 {
		t.Fatalf("errs = %+v", errs)
	}
	model.textarea.SetValue("/d")
	model = model.updateCompletionFromInput()
	view := stripANSI(model.completionView(80))
	if !strings.Contains(view, "/demo") || !strings.Contains(view, "[skill]") {
		t.Fatalf("view = %q", view)
	}
}

func TestCompletionKeepsSkillNameWhenDescriptionIsLong(t *testing.T) {
	item := command.Completion{
		Name:        "/very-long-skill-name",
		Description: strings.Repeat("long description ", 12),
		Skill:       true,
	}
	line := completionLine(item, 42)
	if !strings.HasPrefix(line, "/very-long-skill-name [skill]  ") {
		t.Fatalf("skill label should remain at the front: %q", line)
	}
	if !strings.HasSuffix(line, "...") {
		t.Fatalf("description should be truncated at the end: %q", line)
	}
	if strings.HasPrefix(line, "...") {
		t.Fatalf("line should not truncate from the front: %q", line)
	}
}
