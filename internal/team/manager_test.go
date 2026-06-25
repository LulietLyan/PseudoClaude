package team

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"PseudoClaude/internal/worktree"
)

func TestManagerCreateAndRecover(t *testing.T) {
	home := t.TempDir()
	mgr, err := NewManager(ManagerOptions{HomeDir: home, ProjectRoot: "/repo"})
	if err != nil {
		t.Fatal(err)
	}
	team, err := mgr.Create(context.Background(), CreateInput{Name: "Demo Team", LeadAgentID: "lead-1"})
	if err != nil {
		t.Fatal(err)
	}
	if team.SanitizedName != "demo-team" {
		t.Fatalf("sanitized = %q", team.SanitizedName)
	}
	if _, err := os.Stat(team.ConfigPath); err != nil {
		t.Fatalf("config missing: %v", err)
	}
	if _, err := os.Stat(team.InboxDir); err != nil {
		t.Fatalf("inbox missing: %v", err)
	}
	if _, err := os.Stat(team.TasksPath); err != nil {
		t.Fatalf("tasks missing: %v", err)
	}
	second, err := mgr.Create(context.Background(), CreateInput{Name: "Demo Team"})
	if err != nil {
		t.Fatal(err)
	}
	if second.SanitizedName != "demo-team-2" {
		t.Fatalf("second sanitized = %q", second.SanitizedName)
	}

	recovered, err := NewManager(ManagerOptions{HomeDir: home, ProjectRoot: "/repo"})
	if err != nil {
		t.Fatal(err)
	}
	if got := recovered.List(); len(got) != 2 {
		t.Fatalf("recovered team count = %d", len(got))
	}
}

func TestManagerRecoverSkipsDamagedConfig(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, ".PseudoClaude", "teams")
	if err := os.MkdirAll(filepath.Join(root, "bad"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "bad", "config.json"), []byte("{bad json"), 0o644); err != nil {
		t.Fatal(err)
	}
	var warnings []string
	mgr, err := NewManager(ManagerOptions{
		HomeDir: home,
		Logf: func(format string, args ...any) {
			warnings = append(warnings, format)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(mgr.List()) != 0 {
		t.Fatal("damaged team was recovered")
	}
	if len(warnings) == 0 || !strings.Contains(warnings[0], "skipping damaged") {
		t.Fatalf("warnings = %#v", warnings)
	}
}

func TestTeamMemberOperationsReloadBeforeSave(t *testing.T) {
	home := t.TempDir()
	mgr, err := NewManager(ManagerOptions{HomeDir: home})
	if err != nil {
		t.Fatal(err)
	}
	team, err := mgr.Create(context.Background(), CreateInput{Name: "Demo"})
	if err != nil {
		t.Fatal(err)
	}
	if err := team.AddMember(home, MemberInfo{Name: "alice", AgentID: "agent-a", BackendType: BackendInProcess, IsActive: boolPtr(true)}); err != nil {
		t.Fatal(err)
	}
	stale := *team
	latest, err := loadTeam(team.ConfigPath, home)
	if err != nil {
		t.Fatal(err)
	}
	if err := latest.AddMember(home, MemberInfo{Name: "bob", AgentID: "agent-b", BackendType: BackendInProcess, IsActive: boolPtr(true)}); err != nil {
		t.Fatal(err)
	}
	if err := stale.SetMemberActive(home, "alice", false); err != nil {
		t.Fatal(err)
	}
	reloaded, err := loadTeam(team.ConfigPath, home)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := reloaded.MemberByName("bob"); !ok {
		t.Fatalf("reload-before-save lost bob: %+v", reloaded.Members)
	}
	alice, ok := reloaded.MemberByName("alice")
	if !ok || alice.IsActive == nil || *alice.IsActive {
		t.Fatalf("alice not inactive: %+v", alice)
	}
}

func TestManagerDeleteAndKillMember(t *testing.T) {
	home := t.TempDir()
	backend := &fakeBackend{}
	worktrees := &fakeWorktrees{}
	mgr, err := NewManager(ManagerOptions{HomeDir: home, Backend: backend, Worktrees: worktrees})
	if err != nil {
		t.Fatal(err)
	}
	team, err := mgr.Create(context.Background(), CreateInput{Name: "Demo"})
	if err != nil {
		t.Fatal(err)
	}
	sessionDir := filepath.Join(home, "sessions", "a")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := team.AddMember(home, MemberInfo{Name: "alice", AgentID: "agent-a", BackendType: BackendInProcess, WorktreeName: "wt-a", SessionDir: sessionDir, IsActive: boolPtr(true)}); err != nil {
		t.Fatal(err)
	}
	if err := mgr.Delete(context.Background(), "Demo", false); err != ErrTeamActive {
		t.Fatalf("Delete active err = %v, want ErrTeamActive", err)
	}
	if err := mgr.Delete(context.Background(), "Demo", true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(team.ConfigDir); !os.IsNotExist(err) {
		t.Fatalf("team dir still exists: %v", err)
	}
	if backend.kills != 1 {
		t.Fatalf("kills = %d, want 1", backend.kills)
	}
	if worktrees.removed != "wt-a" {
		t.Fatalf("removed worktree = %q", worktrees.removed)
	}
}

type fakeBackend struct {
	kills int
}

func (b *fakeBackend) Kill(ctx context.Context, req KillRequest) error {
	b.kills++
	return nil
}

type fakeWorktrees struct {
	removed string
}

func (w *fakeWorktrees) AutoCleanup(ctx context.Context, name string) (*worktree.AutoCleanupReport, error) {
	w.removed = name
	return &worktree.AutoCleanupReport{}, nil
}

func (w *fakeWorktrees) Create(ctx context.Context, in worktree.CreateInput) (*worktree.Worktree, error) {
	return &worktree.Worktree{Name: in.Name, Path: "/tmp/" + in.Name, Branch: "worktree-" + in.Name}, nil
}
