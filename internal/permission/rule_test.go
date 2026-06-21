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
	rule, _ = parseRule("Bash(=git status)", DecisionAllow)
	if !ruleMatches(rule, "run_command", "git status", false) || ruleMatches(rule, "run_command", "git status -s", false) {
		t.Fatal("exact command matcher should match only exact target")
	}
	rule, _ = parseRule("Bash(~^npm (install|test)$)", DecisionAllow)
	if !ruleMatches(rule, "run_command", "npm test", false) || ruleMatches(rule, "run_command", "npm run test", false) {
		t.Fatal("regex command matcher should match expected commands")
	}
	rule, _ = parseRule("Bash(!~^rm)", DecisionAllow)
	if ruleMatches(rule, "run_command", "rm -rf .", false) || !ruleMatches(rule, "run_command", "ls -lh", false) {
		t.Fatal("not command matcher should invert regex")
	}
	rule, _ = parseRule("Read", DecisionAllow)
	if !ruleMatches(rule, "read_file", "anything", true) {
		t.Fatal("tool-wide rule should match")
	}
	if _, ok := parseRule("Nope(*)", DecisionAllow); ok {
		t.Fatal("invalid friendly tool should not parse")
	}

	allow, _ := parseRule("Bash(git *)", DecisionAllow)
	deny, _ := parseRule("Bash(git push)", DecisionDeny)
	rs := RuleSet{Allow: []Rule{allow}, Deny: []Rule{deny}}
	got, ok := rs.Match("run_command", "git push", false)
	if !ok || got.Decision != DecisionDeny {
		t.Fatalf("deny should win, got %+v ok=%v", got, ok)
	}
}

func TestMCPRule(t *testing.T) {
	rule, ok := parseRule("mcp__github__get_issue", DecisionAllow)
	if !ok {
		t.Fatal("mcp exact rule did not parse")
	}
	if !ruleMatches(rule, "mcp__github__get_issue", "", false) {
		t.Fatal("mcp exact rule should match")
	}
	if ruleMatches(rule, "mcp__github__create_issue", "", false) {
		t.Fatal("mcp exact rule should not match other tool")
	}

	rule, ok = parseRule("mcp__github__*", DecisionAllow)
	if !ok {
		t.Fatal("mcp glob rule did not parse")
	}
	if !ruleMatches(rule, "mcp__github__get_issue", "", false) {
		t.Fatal("mcp glob should match same server")
	}
	if ruleMatches(rule, "mcp__gitlab__get_issue", "", false) {
		t.Fatal("mcp glob should not match other server")
	}

	allow, _ := parseRule("mcp__github__*", DecisionAllow)
	deny, _ := parseRule("mcp__github__delete_issue", DecisionDeny)
	rs := RuleSet{Allow: []Rule{allow}, Deny: []Rule{deny}}
	got, ok := rs.Match("mcp__github__delete_issue", "", false)
	if !ok || got.Decision != DecisionDeny {
		t.Fatalf("deny should win, got %+v ok=%v", got, ok)
	}
}
