package prompt

import (
	"regexp"
	"strings"
	"testing"
)

func TestSelectLogoByViewport(t *testing.T) {
	if got := SelectLogo(100, 24); got != LogoBanner {
		t.Fatalf("wide logo = %q, want file logo", got)
	}
	if got := SelectLogo(50, 12); got != MiniLogo {
		t.Fatalf("medium logo = %q, want mini logo", got)
	}
	if got := SelectLogo(30, 8); got != TinyLogo {
		t.Fatalf("small logo = %q, want tiny logo", got)
	}
}

func TestResponsiveBannerIncludesNextLineContent(t *testing.T) {
	banner := RenderResponsiveBanner("0.1.0", "/tmp", 30, 8)
	if !strings.Contains(banner, "PseudoClaude v0.1.0") {
		t.Fatalf("banner missing version line: %q", banner)
	}
	if strings.Contains(banner, "\n\nPseudoClaude v") {
		t.Fatalf("banner has extra blank line before version: %q", banner)
	}
}

func TestBuildSystemPromptFixedModuleOrder(t *testing.T) {
	got := BuildSystemPrompt()
	parts := []string{
		"You are PseudoClaude",
		"Follow system and developer instructions",
		"In normal chat",
		"Before changing a file",
		"Prefer dedicated tools",
		"Be warm",
		"Use Markdown",
	}
	last := -1
	for _, part := range parts {
		idx := strings.Index(got, part)
		if idx < 0 {
			t.Fatalf("system prompt missing %q:\n%s", part, got)
		}
		if idx <= last {
			t.Fatalf("%q appeared out of order in:\n%s", part, got)
		}
		last = idx
	}
	if !strings.Contains(got, "\n\nFollow system") {
		t.Fatalf("modules are not separated by blank lines:\n%s", got)
	}
}

func TestAssembleSystemSkipsEmptyModulesAndAllowsExtension(t *testing.T) {
	got := AssembleSystem([]Module{
		{Name: "empty", Priority: 20, Content: "  \n\t"},
		{Name: "late", Priority: 30, Content: "late"},
		{Name: "early", Priority: 10, Content: "early"},
		{Name: "middle", Priority: 25, Content: "middle"},
	})
	if got != "early\n\nmiddle\n\nlate" {
		t.Fatalf("assembled = %q", got)
	}
	if regexp.MustCompile(`\n{3,}`).MatchString(got) {
		t.Fatalf("unexpected extra blank lines: %q", got)
	}
}

func TestBuildSystemPromptStableAndDynamicFree(t *testing.T) {
	first := BuildSystemPrompt()
	second := BuildSystemPrompt()
	if first != second {
		t.Fatal("stable prompt changed between calls")
	}
	for _, dynamic := range []string{"working directory", "git status", "provider", "model", "Runtime environment"} {
		if strings.Contains(first, dynamic) {
			t.Fatalf("stable prompt contains dynamic field %q:\n%s", dynamic, first)
		}
	}
}

func TestSystemPromptMentionsDedicatedToolsAndReadBeforeEdit(t *testing.T) {
	got := BuildSystemPrompt()
	for _, part := range []string{"read_file", "find_files", "search_code", "Use edit_file only after reading"} {
		if !strings.Contains(got, part) {
			t.Fatalf("system prompt missing %q:\n%s", part, got)
		}
	}
}

func TestEnvironmentRenderIncludesFields(t *testing.T) {
	got := (Environment{
		WorkingDir: "/tmp/project",
		Platform:   "test/os",
		Date:       "2026-06-16",
		GitStatus:  "clean",
		Version:    "0.1.0",
		Provider:   "fake",
		Model:      "fake-model",
	}).Render()
	for _, part := range []string{"/tmp/project", "test/os", "2026-06-16", "clean", "0.1.0", "fake", "fake-model"} {
		if !strings.Contains(got, part) {
			t.Fatalf("environment missing %q:\n%s", part, got)
		}
	}
}

func TestGatherEnvironmentNonGitDirectory(t *testing.T) {
	env := GatherEnvironment("0.1.0", "fake", "fake-model", t.TempDir())
	if env.WorkingDir == "" || env.Platform == "" || env.Date == "" || env.Provider != "fake" || env.Model != "fake-model" {
		t.Fatalf("environment = %+v", env)
	}
	if got := env.Render(); !strings.Contains(got, "Runtime environment:") {
		t.Fatalf("rendered environment = %q", got)
	}
}

func TestSystemReminderAndPlanReminder(t *testing.T) {
	if got := SystemReminder(" \n "); got != "" {
		t.Fatalf("empty reminder = %q", got)
	}
	got := SystemReminder("hello")
	if !strings.HasPrefix(got, "<system-reminder>") || !strings.HasSuffix(got, "</system-reminder>") || !strings.Contains(got, "hello") {
		t.Fatalf("reminder = %q", got)
	}
	full := PlanReminder(true)
	short := PlanReminder(false)
	if full == short || !strings.Contains(full, "read-only tools") || !strings.Contains(short, "Still in plan mode") {
		t.Fatalf("full=%q short=%q", full, short)
	}
	for _, reminder := range []string{full, short} {
		if !strings.HasPrefix(reminder, "<system-reminder>") || !strings.HasSuffix(reminder, "</system-reminder>") {
			t.Fatalf("plan reminder missing tags: %q", reminder)
		}
	}
}
