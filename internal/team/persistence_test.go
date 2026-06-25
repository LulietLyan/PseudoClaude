package team

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSanitizeName(t *testing.T) {
	tests := map[string]string{
		"Demo Team":          "demo-team",
		"  A/B:C  ":          "a-b-c",
		"alpha_beta.1":       "alpha_beta.1",
		"***":                "",
		"Already--Safe_Name": "already--safe_name",
	}
	for input, want := range tests {
		if got := SanitizeName(input); got != want {
			t.Fatalf("SanitizeName(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestUniqueSanitizedName(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "demo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "demo-2"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := uniqueSanitizedName(root, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if got != "demo-3" {
		t.Fatalf("uniqueSanitizedName = %q, want demo-3", got)
	}
	if _, err := uniqueSanitizedName(root, ""); err == nil {
		t.Fatal("uniqueSanitizedName accepted empty name")
	}
}

func TestLoadTeamDerivesPaths(t *testing.T) {
	home := t.TempDir()
	team := Team{
		Name:          "Demo",
		SanitizedName: "demo",
		ProjectRoot:   "/repo",
		LeadAgentID:   "lead",
		Backend:       BackendInProcess,
		CreatedAt:     time.Now(),
	}
	derivePaths(&team, home)
	if err := atomicWriteJSON(team.ConfigPath, team); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadTeam(team.ConfigPath, home)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ConfigDir != filepath.Join(home, ".PseudoClaude", "teams", "demo") {
		t.Fatalf("ConfigDir = %q", loaded.ConfigDir)
	}
	if loaded.InboxDir == "" || loaded.TasksPath == "" {
		t.Fatalf("paths not derived: %+v", loaded)
	}
}

func TestReloadMembers(t *testing.T) {
	home := t.TempDir()
	team := Team{Name: "Demo", SanitizedName: "demo", Backend: BackendInProcess}
	derivePaths(&team, home)
	if err := atomicWriteJSON(team.ConfigPath, team); err != nil {
		t.Fatal(err)
	}
	latest := team
	latest.Members = []MemberInfo{{Name: "alice", AgentID: "agent-a"}}
	if err := atomicWriteJSON(team.ConfigPath, latest); err != nil {
		t.Fatal(err)
	}
	if err := team.reloadMembers(home); err != nil {
		t.Fatal(err)
	}
	if len(team.Members) != 1 || team.Members[0].Name != "alice" {
		t.Fatalf("members = %#v", team.Members)
	}
}
