package permission

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

type Rule struct {
	Tool    string
	Pattern string
	Matcher Matcher
	Action  Decision
}

type RuleSet struct {
	Allow []Rule
	Deny  []Rule
}

func parseRule(text string, action Decision) (Rule, bool) {
	rule, err := parseRuleWithError(text, action)
	return rule, err == nil
}

func parseRuleWithError(text string, action Decision) (Rule, error) {
	if action != DecisionAllow && action != DecisionDeny {
		return Rule{}, fmt.Errorf("invalid action %q", action)
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return Rule{}, fmt.Errorf("empty rule")
	}
	tool := text
	pattern := ""
	if idx := strings.Index(text, "("); idx >= 0 {
		if !strings.HasSuffix(text, ")") {
			return Rule{}, fmt.Errorf("missing closing parenthesis")
		}
		tool = strings.TrimSpace(text[:idx])
		pattern = strings.TrimSpace(text[idx+1 : len(text)-1])
	}
	if tool == "" || internalName(tool) == "" {
		return Rule{}, fmt.Errorf("unknown tool %q", tool)
	}
	rule := Rule{Tool: tool, Pattern: pattern, Action: action}
	if pattern != "" {
		matcher, err := CompileMatcher(pattern)
		if err != nil {
			return Rule{}, err
		}
		rule.Matcher = matcher
	}
	return rule, nil
}

func (rs RuleSet) Match(tool, target string, isPath bool) (CheckResult, bool) {
	for _, rule := range rs.Deny {
		if ruleMatches(rule, tool, target, isPath) {
			return CheckResult{Decision: DecisionDeny, Source: "rule", Reason: "denied by permission rule " + ruleString(rule), Rule: ruleString(rule), Target: target}, true
		}
	}
	for _, rule := range rs.Allow {
		if ruleMatches(rule, tool, target, isPath) {
			return CheckResult{Decision: DecisionAllow, Source: "rule", Reason: "allowed by permission rule " + ruleString(rule), Rule: ruleString(rule), Target: target}, true
		}
	}
	return CheckResult{}, false
}

func ruleMatches(rule Rule, tool, target string, isPath bool) bool {
	if rule.Action != DecisionAllow && rule.Action != DecisionDeny {
		return false
	}
	ruleTool := internalName(rule.Tool)
	if isMCPToolGlob(ruleTool) {
		if !commandGlobMatch(ruleTool, tool) {
			return false
		}
	} else if ruleTool != tool {
		return false
	}
	if rule.Pattern == "" {
		return true
	}
	matcher := rule.Matcher
	if matcher == nil {
		compiled, err := CompileMatcher(rule.Pattern)
		if err != nil {
			return false
		}
		matcher = compiled
	}
	return matcher.Match(target, isPath)
}

func commandGlobMatch(pattern, target string) bool {
	pattern = strings.ReplaceAll(pattern, "**", "*")
	re := globRegexp(pattern, false)
	return re.MatchString(target)
}

func pathGlobMatch(pattern, target string) bool {
	pattern = filepath.ToSlash(filepath.Clean(pattern))
	target = filepath.ToSlash(filepath.Clean(target))
	re := globRegexp(pattern, true)
	return re.MatchString(target)
}

func globRegexp(pattern string, pathMode bool) *regexp.Regexp {
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(pattern); {
		switch {
		case pathMode && strings.HasPrefix(pattern[i:], "**/"):
			b.WriteString("(?:.*/)?")
			i += 3
		case pathMode && strings.HasPrefix(pattern[i:], "**"):
			b.WriteString(".*")
			i += 2
		case pattern[i] == '*':
			if pathMode {
				b.WriteString("[^/]*")
			} else {
				b.WriteString(".*")
			}
			i++
		case pattern[i] == '?':
			if pathMode {
				b.WriteString("[^/]")
			} else {
				b.WriteString(".")
			}
			i++
		default:
			b.WriteString(regexp.QuoteMeta(string(pattern[i])))
			i++
		}
	}
	b.WriteString("$")
	return regexp.MustCompile(b.String())
}

func ruleString(rule Rule) string {
	if rule.Pattern == "" {
		return rule.Tool
	}
	return rule.Tool + "(" + rule.Pattern + ")"
}
