package hook

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoader(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	logs := []string{}
	engine := Load(LoadOptions{ProjectRoot: root, HomeDir: home, Logf: func(format string, args ...any) {
		logs = append(logs, sprintf(format, args...))
	}})
	if len(engine.Rules()) != 0 || len(logs) != 0 {
		t.Fatalf("empty load rules=%+v logs=%+v", engine.Rules(), logs)
	}

	project := filepath.Join(root, ".PseudoClaude", "hooks.yaml")
	writeFile(t, project, `
hooks:
  - name: project-prompt
    event: SessionStart
    action:
      type: prompt
      text: hello
`)
	engine = Load(LoadOptions{ProjectRoot: root, HomeDir: home})
	rules := engine.Rules()
	if len(rules) != 1 || rules[0].Name != "project-prompt" || rules[0].Timeout != defaultTimeout || rules[0].Source != project {
		t.Fatalf("rules = %+v", rules)
	}

	user := filepath.Join(home, ".PseudoClaude", "hooks.yaml")
	writeFile(t, user, `
hooks:
  - name: user-shell
    event: Stop
    only_once: true
    async: true
    timeout: 5s
    action:
      type: shell
      command: "echo ok"
`)
	engine = Load(LoadOptions{ProjectRoot: root, HomeDir: home})
	if len(engine.Rules()) != 2 {
		t.Fatalf("merged rules = %+v", engine.Rules())
	}

	writeFile(t, project, `
hooks:
  - name: user-shell
    event: SessionStart
    action:
      type: prompt
      text: duplicate
  - name: good
    event: SessionStart
    if:
      all_of:
        - field: event
          match: { type: exact, value: SessionStart }
    action:
      type: prompt
      text: good
`)
	logs = nil
	engine = Load(LoadOptions{ProjectRoot: root, HomeDir: home, Logf: func(format string, args ...any) {
		logs = append(logs, sprintf(format, args...))
	}})
	if len(engine.Rules()) != 2 || !strings.Contains(strings.Join(logs, "\n"), "duplicate") {
		t.Fatalf("duplicate handling rules=%+v logs=%+v", engine.Rules(), logs)
	}
}

func TestLoaderValidation(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, ".PseudoClaude", "hooks.yaml")
	writeFile(t, project, `
hooks:
  - name: missing-event
    action: { type: prompt, text: x }
  - name: unknown-event
    event: Missing
    action: { type: prompt, text: x }
  - name: unknown-action
    event: Stop
    action: { type: missing }
  - name: bad-timeout
    event: Stop
    timeout: nope
    action: { type: prompt, text: x }
  - name: bad-regex
    event: Stop
    if:
      all_of:
        - field: event
          match: { type: regex, value: "[" }
    action: { type: prompt, text: x }
  - name: both-combine
    event: Stop
    if:
      all_of:
        - field: event
          match: { type: exact, value: Stop }
      any_of:
        - field: event
          match: { type: exact, value: Stop }
    action: { type: prompt, text: x }
  - name: bad-async
    event: PreToolUse
    async: true
    action: { type: shell, command: "echo x" }
  - name: empty-condition
    event: Stop
    if:
      all_of: []
    action: { type: prompt, text: x }
  - name: good
    event: Stop
    action: { type: subagent, agent_name: worker, prompt: go }
`)
	var logs []string
	engine := Load(LoadOptions{ProjectRoot: root, HomeDir: t.TempDir(), Logf: func(format string, args ...any) {
		logs = append(logs, sprintf(format, args...))
	}})
	if got := engine.Rules(); len(got) != 1 || got[0].Name != "good" {
		t.Fatalf("rules = %+v logs=%+v", got, logs)
	}
	joined := strings.Join(logs, "\n")
	for _, want := range []string{"unknown event", "unknown action", "invalid timeout", "invalid regex", "both all_of and any_of", "does not allow async", "condition must contain"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("logs missing %q: %s", want, joined)
		}
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func sprintf(format string, args ...any) string {
	return strings.TrimSpace(strings.NewReplacer("\n", " ").Replace(formatString(format, args...)))
}
