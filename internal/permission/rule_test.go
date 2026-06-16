package permission

import "testing"

func TestRule(t *testing.T) {
	rule, ok := parseRule("Bash(git status)", DecisionAllow)
	if !ok {
		t.Fatal("rule did not parse")
	}
	if !ruleMatches(rule, "run_command", "git status", false) {
		t.Fatal("exact command should match")
	}
	if ruleMatches(rule, "run_command", "git push", false) {
		t.Fatal("different command should not match")
	}
	rule, _ = parseRule("Bash(git *)", DecisionAllow)
	if !ruleMatches(rule, "run_command", "git push origin main", false) {
		t.Fatal("command glob should match")
	}
	rule, _ = parseRule("Write(src/**)", DecisionAllow)
	if !ruleMatches(rule, "write_file", "src/a/b.go", true) {
		t.Fatal("path glob should match src subtree")
	}
	if ruleMatches(rule, "write_file", "docs/x.md", true) {
		t.Fatal("path glob should not match docs")
	}
	rule, _ = parseRule("Read", DecisionAllow)
	if !ruleMatches(rule, "read_file", "anything", true) {
		t.Fatal("tool-wide rule should match")
	}
	if _, ok := parseRule("Nope(*)", DecisionAllow); ok {
		t.Fatal("invalid friendly tool should not parse")
	}

	rs := RuleSet{
		Allow: []Rule{{Tool: "Bash", Pattern: "git *", Action: DecisionAllow}},
		Deny:  []Rule{{Tool: "Bash", Pattern: "git push", Action: DecisionDeny}},
	}
	got, ok := rs.Match("run_command", "git push", false)
	if !ok || got.Decision != DecisionDeny {
		t.Fatalf("deny should win, got %+v ok=%v", got, ok)
	}
}
