package permission

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSettings(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "missing.yaml")
	if settings, issues := loadSettings(missing); len(issues) != 0 || settings.DefaultMode != "" {
		t.Fatalf("missing settings = %+v issues=%+v", settings, issues)
	}
	bad := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(bad, []byte("permissions: ["), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, issues := loadSettings(bad); len(issues) == 0 {
		t.Fatal("invalid yaml should produce issue")
	}
	settings := Settings{DefaultMode: "acceptEdits"}
	settings.Permissions.Allow = []string{"Read", "Bogus(*)"}
	rules, issues := settingsToRuleSet(settings)
	if len(rules.Allow) != 1 || len(issues) != 1 {
		t.Fatalf("rules=%+v issues=%+v", rules, issues)
	}
	if got := chooseStartMode(Settings{DefaultMode: "strict"}, Settings{DefaultMode: "acceptEdits"}, Settings{DefaultMode: "bypassPermissions"}); got != ModeStrict {
		t.Fatalf("local mode priority = %s", got)
	}
	if got := chooseStartMode(Settings{}, Settings{DefaultMode: "acceptEdits"}, Settings{DefaultMode: "strict"}); got != ModeAcceptEdits {
		t.Fatalf("project mode priority = %s", got)
	}
	if got := chooseStartMode(Settings{}, Settings{}, Settings{}); got != ModeDefault {
		t.Fatalf("default mode = %s", got)
	}
}
