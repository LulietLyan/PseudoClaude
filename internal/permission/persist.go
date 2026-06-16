package permission

import (
	"os"
	"path/filepath"

	"PseudoClaude/internal/llm"

	"gopkg.in/yaml.v3"
)

func (e *Engine) ruleForCall(call llm.ToolCall) (Rule, string, bool) {
	if e == nil {
		return Rule{}, "", false
	}
	tool := friendlyName(call.Name)
	if tool == "" {
		return Rule{}, "", false
	}
	var pattern string
	if call.Name == "run_command" {
		command, ok := commandText(call)
		if !ok {
			return Rule{}, "", false
		}
		pattern = command
	} else {
		_, matchTarget, ok := pathTarget(call)
		if !ok {
			return Rule{}, "", false
		}
		pattern = normalizeRulePath(e.root, matchTarget)
	}
	rule := Rule{Tool: tool, Pattern: pattern, Action: DecisionAllow}
	return rule, ruleString(rule), true
}

func (e *Engine) AllowForSession(call llm.ToolCall) error {
	rule, _, ok := e.ruleForCall(call)
	if !ok {
		return os.ErrInvalid
	}
	e.session.Allow = appendUniqueRule(e.session.Allow, rule)
	return nil
}

func (e *Engine) PersistLocalAllow(call llm.ToolCall) error {
	rule, text, ok := e.ruleForCall(call)
	if !ok {
		return os.ErrInvalid
	}
	settings, _ := loadSettings(e.localPath)
	if !containsString(settings.Permissions.Allow, text) {
		settings.Permissions.Allow = append(settings.Permissions.Allow, text)
	}
	if err := os.MkdirAll(filepath.Dir(e.localPath), 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(settings)
	if err != nil {
		return err
	}
	if err := os.WriteFile(e.localPath, data, 0o644); err != nil {
		return err
	}
	e.local.Allow = appendUniqueRule(e.local.Allow, rule)
	return nil
}

func appendUniqueRule(rules []Rule, rule Rule) []Rule {
	for _, existing := range rules {
		if existing == rule {
			return rules
		}
	}
	return append(rules, rule)
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
