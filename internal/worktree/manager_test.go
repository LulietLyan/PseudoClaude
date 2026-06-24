package worktree

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestManagerNonGitRepository(t *testing.T) {
	_, err := NewManager(Options{RepoRoot: t.TempDir()})
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("NewManager err = %v, want ErrUnavailable", err)
	}
}

func TestCreateFastRecoverAndLifecycle(t *testing.T) {
	root := newGitRepo(t)
	m := newTestManager(t, root)
	ctx := context.Background()
	wt, err := m.Create(ctx, CreateInput{Name: "feature/a", Manual: true})
	if err != nil {
		t.Fatal(err)
	}
	if wt.FlatName != "feature+a" || wt.Branch != "worktree-feature+a" {
		t.Fatalf("unexpected mapping: %+v", wt)
	}
	if _, err := os.Stat(filepath.Join(wt.Path, "README.md")); err != nil {
		t.Fatalf("worktree file missing: %v", err)
	}
	second, err := m.Create(ctx, CreateInput{Name: "feature/a", Manual: true})
	if err != nil {
		t.Fatal(err)
	}
	if second.Path != wt.Path {
		t.Fatalf("fast recover path = %s, want %s", second.Path, wt.Path)
	}
	wd, _ := os.Getwd()
	if _, err := m.Enter(ctx, "feature/a"); err != nil {
		t.Fatal(err)
	}
	after, _ := os.Getwd()
	if wd != after {
		t.Fatalf("Enter changed process cwd: %s -> %s", wd, after)
	}
	if got := m.EffectiveCWD(root); got != wt.Path {
		t.Fatalf("effective cwd = %s, want %s", got, wt.Path)
	}
	if report, err := m.Exit(ctx, ExitOptions{}); err != nil || report.Removed {
		t.Fatalf("Exit keep = %+v %v", report, err)
	}
	if got := m.EffectiveCWD(root); got != root {
		t.Fatalf("effective cwd after exit = %s, want %s", got, root)
	}
}

func TestRemoveProtectsDirtyWorktree(t *testing.T) {
	root := newGitRepo(t)
	m := newTestManager(t, root)
	ctx := context.Background()
	wt, err := m.Create(ctx, CreateInput{Name: "dirty", Manual: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wt.Path, "dirty.txt"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Remove(ctx, "dirty", RemoveOptions{}); !errors.Is(err, ErrProtectedChanges) {
		t.Fatalf("Remove dirty err = %v, want ErrProtectedChanges", err)
	}
	if _, err := os.Stat(wt.Path); err != nil {
		t.Fatalf("dirty worktree removed despite protection: %v", err)
	}
	if _, err := m.Remove(ctx, "dirty", RemoveOptions{Discard: true}); err != nil {
		t.Fatalf("discard remove: %v", err)
	}
	if _, err := os.Stat(wt.Path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("worktree path exists after discard remove: %v", err)
	}
}

func TestAutoCleanupTemporaryWorktree(t *testing.T) {
	root := newGitRepo(t)
	m := newTestManager(t, root)
	ctx := context.Background()
	wt, err := m.Create(ctx, CreateInput{Name: "agent-a123abcd", Manual: false})
	if err != nil {
		t.Fatal(err)
	}
	report, err := m.AutoCleanup(ctx, wt.Name)
	if err != nil {
		t.Fatal(err)
	}
	if report.Kept {
		t.Fatalf("AutoCleanup kept clean temporary worktree: %+v", report)
	}
	if _, err := os.Stat(wt.Path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("worktree path exists after cleanup: %v", err)
	}
}

func TestRandomAgentName(t *testing.T) {
	name := RandomAgentName()
	if !isTemporaryName(name) {
		t.Fatalf("RandomAgentName = %q", name)
	}
}

func TestSweepSkipsCurrentAndRemovesStale(t *testing.T) {
	root := newGitRepo(t)
	now := time.Now()
	m := newTestManager(t, root)
	m.now = func() time.Time { return now }
	ctx := context.Background()
	keep, err := m.Create(ctx, CreateInput{Name: "agent-a1111111"})
	if err != nil {
		t.Fatal(err)
	}
	remove, err := m.Create(ctx, CreateInput{Name: "agent-a2222222"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Enter(ctx, keep.Name); err != nil {
		t.Fatal(err)
	}
	old := now.Add(-48 * time.Hour)
	if err := os.Chtimes(keep.Path, old, old); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(remove.Path, old, old); err != nil {
		t.Fatal(err)
	}
	results := m.SweepStale(ctx, now.Add(-24*time.Hour))
	var removed bool
	for _, result := range results {
		if result.Name == remove.Name && result.Removed {
			removed = true
		}
		if result.Name == keep.Name && result.Removed {
			t.Fatalf("current session was removed: %+v", result)
		}
	}
	if !removed {
		t.Fatalf("stale worktree was not removed: %+v", results)
	}
}

func newTestManager(t *testing.T, root string) *Manager {
	t.Helper()
	m, err := NewManager(Options{
		RepoRoot: root,
		Logf:     func(string, ...any) {},
	})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func newGitRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	run(t, root, "git", "init")
	run(t, root, "git", "config", "user.email", "test@example.com")
	run(t, root, "git", "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte(".PseudoClaude/worktrees/\n.PseudoClaude/worktree_session.json\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, root, "git", "add", ".")
	run(t, root, "git", "commit", "-m", "initial")
	return root
}

func run(t *testing.T, dir string, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_ASKPASS=")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s failed: %v\n%s", name, strings.Join(args, " "), err, out)
	}
	return string(out)
}
