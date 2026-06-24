package permission

import (
	"fmt"
	"path/filepath"
	"strings"

	"PseudoClaude/internal/llm"
	"PseudoClaude/internal/tools"
)

type Engine struct {
	root       string
	session    RuleSet
	local      RuleSet
	project    RuleSet
	user       RuleSet
	localPath  string
	startMode  Mode
	loadIssues []LoadIssue
}

type CheckResult struct {
	Decision Decision
	Source   string
	Reason   string
	Rule     string
	Category Category
	Target   string
	CWD      string
}

type CheckContext struct {
	CWD string
}

func NewEngine(root string, opts Options) (*Engine, error) {
	resolvedRoot, err := resolveRoot(root)
	if err != nil {
		return nil, err
	}
	if opts.UserPath == "" && opts.ProjectPath == "" && opts.LocalPath == "" {
		opts = DefaultOptions(resolvedRoot)
	}
	userSettings, userIssues := loadSettings(opts.UserPath)
	projectSettings, projectIssues := loadSettings(opts.ProjectPath)
	localSettings, localIssues := loadSettings(opts.LocalPath)
	userRules, userRuleIssues := settingsToRuleSet(userSettings)
	projectRules, projectRuleIssues := settingsToRuleSet(projectSettings)
	localRules, localRuleIssues := settingsToRuleSet(localSettings)

	issues := append([]LoadIssue{}, userIssues...)
	issues = append(issues, projectIssues...)
	issues = append(issues, localIssues...)
	issues = appendRuleIssues(issues, opts.UserPath, userRuleIssues)
	issues = appendRuleIssues(issues, opts.ProjectPath, projectRuleIssues)
	issues = appendRuleIssues(issues, opts.LocalPath, localRuleIssues)

	return &Engine{
		root:       resolvedRoot,
		local:      localRules,
		project:    projectRules,
		user:       userRules,
		localPath:  opts.LocalPath,
		startMode:  chooseStartMode(localSettings, projectSettings, userSettings),
		loadIssues: issues,
	}, nil
}

func appendRuleIssues(dst []LoadIssue, path string, issues []LoadIssue) []LoadIssue {
	for _, issue := range issues {
		issue.Path = path
		dst = append(dst, issue)
	}
	return dst
}

func (e *Engine) StartMode() Mode {
	if e == nil {
		return ModeDefault
	}
	return e.startMode
}

func (e *Engine) LoadIssues() []LoadIssue {
	if e == nil {
		return nil
	}
	return append([]LoadIssue(nil), e.loadIssues...)
}

func (e *Engine) Check(mode Mode, call llm.ToolCall, safety tools.Safety) CheckResult {
	if e == nil {
		return CheckResult{Decision: DecisionAllow, Source: "mode", Reason: "permission engine is disabled"}
	}
	return e.CheckWithContext(mode, call, safety, CheckContext{CWD: e.root})
}

func (e *Engine) CheckWithContext(mode Mode, call llm.ToolCall, safety tools.Safety, ctx CheckContext) CheckResult {
	if e == nil {
		return CheckResult{Decision: DecisionAllow, Source: "mode", Reason: "permission engine is disabled"}
	}
	root := strings.TrimSpace(ctx.CWD)
	if root == "" {
		root = e.root
	}
	if abs, err := filepath.Abs(root); err == nil {
		root = abs
	}
	category, knownCategory := classify(call, safety)
	friendly := friendlyName(call.Name)
	if !knownCategory || friendly == "" {
		return CheckResult{Decision: DecisionAsk, Source: "unknown", Reason: fmt.Sprintf("tool %q is not covered by permission rules", call.Name), Category: category, CWD: root}
	}

	var target string
	var matchTarget string
	var isPath bool
	if category == CategoryExec {
		command, ok := commandText(call)
		if !ok {
			return CheckResult{Decision: DecisionDeny, Source: "unknown", Reason: "command arguments could not be parsed", Category: category, CWD: root}
		}
		target = command
		matchTarget = command
		if ok, pattern := hitsBlacklist(command); ok {
			return CheckResult{Decision: DecisionDeny, Source: "blacklist", Reason: "command matches dangerous blacklist pattern", Rule: pattern, Category: category, Target: target, CWD: root}
		}
	} else if isMCPToolName(call.Name) {
		target = call.Name
		matchTarget = call.Name
	} else {
		rawTarget, rawMatchTarget, ok := pathTarget(call)
		if !ok {
			return CheckResult{Decision: DecisionDeny, Source: "unknown", Reason: "file tool path arguments could not be parsed", Category: category, CWD: root}
		}
		if pathToolRequiresExactPath(call.Name) && pathContainsGlob(rawTarget) {
			return CheckResult{Decision: DecisionDeny, Source: "unknown", Reason: call.Name + " does not accept glob patterns; use find_files first", Category: category, Target: rawTarget, CWD: root}
		}
		resolved, inside, err := sandboxTarget(root, rawTarget)
		if err != nil {
			return CheckResult{Decision: DecisionDeny, Source: "sandbox", Reason: "path could not be resolved for sandbox check", Category: category, Target: rawTarget, CWD: root}
		}
		rel, relErr := filepath.Rel(root, resolved)
		if relErr != nil {
			rel = rawTarget
		}
		target = filepath.ToSlash(filepath.Clean(rel))
		matchTarget = normalizeRulePath(root, rawMatchTarget)
		isPath = true
		if !inside {
			return CheckResult{Decision: DecisionDeny, Source: "sandbox", Reason: "path is outside the project root", Category: category, Target: target, CWD: root}
		}
	}

	for _, rules := range []RuleSet{e.session, e.local, e.project, e.user} {
		if result, ok := rules.Match(call.Name, matchTarget, isPath); ok {
			result.Category = category
			if result.Target == "" {
				result.Target = target
			}
			result.CWD = root
			return result
		}
	}
	decision := modeFallback(mode, category)
	return CheckResult{Decision: decision, Source: "mode", Reason: fmt.Sprintf("%s mode requires %s for %s tools", ParseMode(mode.String()), decision, category), Category: category, Target: target, CWD: root}
}

func normalizeRulePath(root, raw string) string {
	if strings.TrimSpace(raw) == "" {
		return "."
	}
	target := raw
	if !filepath.IsAbs(target) {
		target = filepath.Join(root, target)
	}
	abs, err := filepath.Abs(target)
	if err != nil {
		return filepath.ToSlash(filepath.Clean(raw))
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return filepath.ToSlash(filepath.Clean(raw))
	}
	return filepath.ToSlash(filepath.Clean(rel))
}
