package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"PseudoClaude/internal/llm"
)

func TestStoreApplyAndPathSafety(t *testing.T) {
	store := NewStore(LevelProject, t.TempDir())
	now := time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC)
	err := store.Apply([]Operation{{
		Action:  "create",
		Level:   LevelProject,
		Type:    TypeProjectKnowledge,
		Title:   "Build",
		Summary: "Use go test",
		Slug:    "build",
		Content: "Run go test ./...",
	}}, now)
	if err != nil {
		t.Fatal(err)
	}
	index := store.LoadIndex()
	if !strings.Contains(index, "[project_knowledge] Build") {
		t.Fatalf("index = %q", index)
	}
	if _, err := store.NotePath("../bad.md"); err == nil {
		t.Fatal("expected path traversal rejection")
	}
	if _, err := os.Stat(filepath.Join(store.Dir, "project_knowledge_build.md")); err != nil {
		t.Fatal(err)
	}
}

func TestManagerIndexTrimOrder(t *testing.T) {
	project := t.TempDir()
	user := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, IndexFileName), []byte("- [project_knowledge] P — p\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(user, IndexFileName), []byte("- [user_preference] U — u\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	manager := NewManager(project, user)
	manager.RefreshIndex()
	index := manager.IndexText()
	if !(strings.Index(index, "Project Memory") < strings.Index(index, "User Memory")) {
		t.Fatalf("index order = %q", index)
	}

	var lines []string
	for i := 0; i < MaxIndexLines+5; i++ {
		lines = append(lines, "- [project_knowledge] X — y")
	}
	if got := trimIndex(strings.Join(lines, "\n")); !strings.Contains(got, "(index truncated)") {
		t.Fatalf("missing truncation marker")
	}
}

func TestRefreshAndIndexText(t *testing.T) {
	project := t.TempDir()
	user := t.TempDir()
	manager := NewManager(project, user)
	if got := manager.RefreshAndIndexText(); got != "" {
		t.Fatalf("empty index = %q", got)
	}
	if err := os.WriteFile(filepath.Join(user, IndexFileName), []byte("- [user_preference] User Profile — Go engineer\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := manager.RefreshAndIndexText()
	if !strings.Contains(got, "User Memory") || !strings.Contains(got, "Go engineer") {
		t.Fatalf("index = %q", got)
	}
}

func TestBuildUpdatePromptTreatsUserProfileAsMemory(t *testing.T) {
	messages := BuildUpdatePrompt([]llm.Message{
		{Role: "user", Content: "我是Go工程师，具备深入的Go专业知识。"},
		{Role: "assistant", Content: "我已经记住了。"},
		{Role: "user", Content: "我是27届计算机专业毕业生，想学习 AI Agent 知识。"},
	}, "", "- [user_preference] User Profile — User is a Go engineer. (user_preference_user-profile.md)")
	prompt := messages[0].Content
	for _, want := range []string{"explicit user self-disclosure", "Go engineer", "computer-science student", "AI Agent", "User Profile", "filename"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
	if !strings.Contains(prompt, "我是Go工程师") || !strings.Contains(prompt, "27届计算机专业毕业生") {
		t.Fatalf("prompt missing recent turn:\n%s", prompt)
	}
}

func TestExtractJSONArrayFromNoisyResponse(t *testing.T) {
	raw := `Sure:
[
  {"action":"create","level":"user","type":"user_preference","title":"User Profile","summary":"User is a Go engineer.","slug":"user-profile","content":"The user is a Go engineer."}
]
Done.`
	got := extractJSONArray(raw)
	if !strings.HasPrefix(got, "[") || !strings.Contains(got, "Go engineer") || strings.Contains(got, "Sure") {
		t.Fatalf("unexpected json extraction: %q", got)
	}
}

func TestValidateOperationRejectsIncompleteCreate(t *testing.T) {
	if err := ValidateOperation(Operation{Action: "create", Level: LevelUser}); err == nil {
		t.Fatal("expected incomplete create to be rejected")
	}
	if err := ValidateOperation(Operation{
		Action:   "update",
		Level:    LevelUser,
		Filename: "user_preference_user-profile.md",
		Content:  "The user is a Go engineer and wants to learn AI Agents.",
	}); err != nil {
		t.Fatalf("valid update rejected: %v", err)
	}
}
