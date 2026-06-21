package command

import (
	"strings"
	"testing"
)

func TestFormat(t *testing.T) {
	help := FormatHelp([]Command{{Name: "/help", Description: "show help", Usage: "/help"}})
	if !strings.Contains(help, "/help") || !strings.Contains(help, "Usage") || !strings.Contains(help, "Description") {
		t.Fatalf("help = %q", help)
	}
	if hint := FormatHelpHint(); !strings.Contains(hint, "/help") || !strings.Contains(hint, "usage") {
		t.Fatalf("hint = %q", hint)
	}
	status := FormatStatus(StatusInfo{WorkMode: WorkModePlan, PermissionMode: "strict", Model: "m", Usage: Usage{InputTokens: 1, OutputTokens: 2, TotalTokens: 3}, SessionID: "s", CWD: "/tmp", RuntimeState: "idle"})
	for _, want := range []string{"Work mode: plan", "Permission mode: strict", "Tokens: input 1, output 2, total 3", "Runtime state: idle"} {
		if !strings.Contains(status, want) {
			t.Fatalf("status missing %q: %q", want, status)
		}
	}
	session := FormatSession(SessionInfo{ID: "id", JSONLPath: "path", MessageCount: 2})
	if !strings.Contains(session, "Messages: 2") || !strings.Contains(session, "Model: (none)") {
		t.Fatalf("session = %q", session)
	}
	if got := FormatMemory(""); !strings.Contains(got, "暂无长期记忆") {
		t.Fatalf("memory empty = %q", got)
	}
	if got := FormatMemory("hello"); !strings.Contains(got, "Agent Memory") || !strings.Contains(got, "hello") {
		t.Fatalf("memory = %q", got)
	}
	if got := FormatSkills(nil); got != "No skills loaded." {
		t.Fatalf("skills empty = %q", got)
	}
	skills := FormatSkills([]SkillSummary{{Name: "demo", Description: "Demo."}})
	if !strings.Contains(skills, "Available skills (1):") || !strings.Contains(skills, "/demo") {
		t.Fatalf("skills = %q", skills)
	}
	if got := FormatHooks(nil, nil); got != "No hooks loaded." {
		t.Fatalf("hooks empty = %q", got)
	}
	hooks := FormatHooks([]HookSummary{
		{Name: "a", Event: "PreToolUse", Action: "shell", Flags: []string{"only_once"}, Source: "project"},
		{Name: "b", Event: "Stop", Action: "http", Flags: []string{"async", "timeout=5s"}, Source: "user"},
	}, []string{"user", "project"})
	for _, want := range []string{"Loaded hooks (2):", "PreToolUse:", "a  shell [only_once]  project", "Stop:", "b  http [async, timeout=5s]  user", "Loaded from:"} {
		if !strings.Contains(hooks, want) {
			t.Fatalf("hooks missing %q: %q", want, hooks)
		}
	}
}
