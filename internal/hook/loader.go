package hook

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"PseudoClaude/internal/permission"

	"gopkg.in/yaml.v3"
)

const defaultTimeout = 30 * time.Second

type LoadOptions struct {
	ProjectRoot string
	HomeDir     string
	UserPath    string
	ProjectPath string
	Logf        func(format string, args ...any)
}

type yamlFile struct {
	Hooks []yamlRule `yaml:"hooks"`
}

type yamlRule struct {
	Name     string        `yaml:"name"`
	Event    string        `yaml:"event"`
	If       yamlCondition `yaml:"if"`
	Action   yamlAction    `yaml:"action"`
	OnlyOnce bool          `yaml:"only_once"`
	Async    bool          `yaml:"async"`
	Timeout  string        `yaml:"timeout"`
}

type yamlCondition struct {
	AllOf *[]yamlAtom `yaml:"all_of"`
	AnyOf *[]yamlAtom `yaml:"any_of"`
}

type yamlAtom struct {
	Field string        `yaml:"field"`
	Match yamlMatchSpec `yaml:"match"`
}

type yamlMatchSpec struct {
	Type  permission.MatchKind `yaml:"type"`
	Value string               `yaml:"value"`
	Inner *yamlMatchSpec       `yaml:"inner"`
}

type yamlAction struct {
	Type      ActionType        `yaml:"type"`
	Command   string            `yaml:"command"`
	Text      string            `yaml:"text"`
	URL       string            `yaml:"url"`
	Method    string            `yaml:"method"`
	Headers   map[string]string `yaml:"headers"`
	Body      string            `yaml:"body"`
	AgentName string            `yaml:"agent_name"`
	Prompt    string            `yaml:"prompt"`
}

func Load(opts LoadOptions) *Engine {
	opts = normalizeLoadOptions(opts)
	var rules []Rule
	sources := make([]string, 0, 2)
	seen := map[string]bool{}
	for _, path := range []string{opts.UserPath, opts.ProjectPath} {
		fileRules, ok := readHookFile(path, opts.Logf)
		if !ok {
			continue
		}
		sources = append(sources, path)
		for _, raw := range fileRules {
			rule, err := compileRule(raw, path, len(rules))
			if err != nil {
				logf(opts.Logf, "%s: rule %q skipped: %v", path, displayRuleName(raw.Name), err)
				continue
			}
			if seen[rule.Name] {
				logf(opts.Logf, "%s: rule %q skipped: duplicate rule name", path, rule.Name)
				continue
			}
			seen[rule.Name] = true
			rules = append(rules, rule)
		}
	}
	return NewEngine(rules, Executor{Logf: opts.Logf}, sources)
}

func normalizeLoadOptions(opts LoadOptions) LoadOptions {
	if opts.ProjectRoot == "" {
		opts.ProjectRoot = "."
	}
	if opts.HomeDir == "" {
		if home, err := os.UserHomeDir(); err == nil {
			opts.HomeDir = home
		}
	}
	if opts.UserPath == "" && opts.HomeDir != "" {
		opts.UserPath = filepath.Join(opts.HomeDir, ".PseudoClaude", "hooks.yaml")
	}
	if opts.ProjectPath == "" {
		opts.ProjectPath = filepath.Join(opts.ProjectRoot, ".PseudoClaude", "hooks.yaml")
	}
	return opts
}

func readHookFile(path string, logfFn func(format string, args ...any)) ([]yamlRule, bool) {
	if path == "" {
		return nil, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, false
		}
		logf(logfFn, "%s: %v", path, err)
		return nil, false
	}
	var file yamlFile
	if err := yaml.Unmarshal(data, &file); err != nil {
		logf(logfFn, "%s: %v", path, err)
		return nil, false
	}
	return file.Hooks, true
}

func compileRule(raw yamlRule, source string, index int) (Rule, error) {
	name := strings.TrimSpace(raw.Name)
	if name == "" {
		return Rule{}, fmt.Errorf("missing name")
	}
	event, err := ParseEvent(strings.TrimSpace(raw.Event))
	if err != nil {
		return Rule{}, err
	}
	action, err := compileAction(raw.Action)
	if err != nil {
		return Rule{}, err
	}
	timeout := defaultTimeout
	if strings.TrimSpace(raw.Timeout) != "" {
		timeout, err = time.ParseDuration(strings.TrimSpace(raw.Timeout))
		if err != nil || timeout <= 0 {
			return Rule{}, fmt.Errorf("invalid timeout %q", raw.Timeout)
		}
	}
	if raw.Async && IsBlocking(event) {
		return Rule{}, fmt.Errorf("blocking event does not allow async")
	}
	cond, err := compileCondition(raw.If)
	if err != nil {
		return Rule{}, err
	}
	return Rule{
		Name:     name,
		Event:    event,
		If:       cond,
		Action:   action,
		OnlyOnce: raw.OnlyOnce,
		Async:    raw.Async,
		Timeout:  timeout,
		Source:   source,
		Index:    index,
	}, nil
}

func compileCondition(raw yamlCondition) (*Condition, error) {
	hasAll := raw.AllOf != nil
	hasAny := raw.AnyOf != nil
	if !hasAll && !hasAny {
		return nil, nil
	}
	if hasAll && hasAny {
		return nil, fmt.Errorf("if cannot declare both all_of and any_of")
	}
	mode := CombineAllOf
	var atoms []yamlAtom
	if hasAny {
		mode = CombineAnyOf
		atoms = *raw.AnyOf
	} else {
		atoms = *raw.AllOf
	}
	if len(atoms) == 0 {
		return nil, fmt.Errorf("condition must contain at least one atom")
	}
	compiled := make([]Atom, 0, len(atoms))
	for _, atom := range atoms {
		field := strings.TrimSpace(atom.Field)
		if field == "" {
			return nil, fmt.Errorf("condition atom missing field")
		}
		matcher, err := permission.CompileMatchSpec(toPermissionSpec(atom.Match))
		if err != nil {
			return nil, fmt.Errorf("condition %q: %w", field, err)
		}
		compiled = append(compiled, Atom{Field: field, Matcher: matcher})
	}
	return &Condition{Mode: mode, Atoms: compiled}, nil
}

func toPermissionSpec(spec yamlMatchSpec) permission.MatchSpec {
	out := permission.MatchSpec{Type: spec.Type, Value: spec.Value}
	if spec.Inner != nil {
		inner := toPermissionSpec(*spec.Inner)
		out.Inner = &inner
	}
	return out
}

func compileAction(raw yamlAction) (Action, error) {
	switch raw.Type {
	case ActionShell:
		if strings.TrimSpace(raw.Command) == "" {
			return Action{}, fmt.Errorf("shell action requires command")
		}
		return Action{Type: raw.Type, Shell: &ShellAction{Command: raw.Command}}, nil
	case ActionPrompt:
		if strings.TrimSpace(raw.Text) == "" {
			return Action{}, fmt.Errorf("prompt action requires text")
		}
		return Action{Type: raw.Type, Prompt: &PromptAction{Text: raw.Text}}, nil
	case ActionHTTP:
		if strings.TrimSpace(raw.URL) == "" {
			return Action{}, fmt.Errorf("http action requires url")
		}
		return Action{Type: raw.Type, HTTP: &HTTPAction{URL: raw.URL, Method: raw.Method, Headers: raw.Headers, Body: raw.Body}}, nil
	case ActionSubagent:
		if strings.TrimSpace(raw.AgentName) == "" || strings.TrimSpace(raw.Prompt) == "" {
			return Action{}, fmt.Errorf("subagent action requires agent_name and prompt")
		}
		return Action{Type: raw.Type, Subagent: &SubagentAction{AgentName: raw.AgentName, Prompt: raw.Prompt}}, nil
	case "":
		return Action{}, fmt.Errorf("missing action type")
	default:
		return Action{}, fmt.Errorf("unknown action type %q", raw.Type)
	}
}

func displayRuleName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "(unnamed)"
	}
	return name
}

func logf(logfFn func(format string, args ...any), format string, args ...any) {
	if logfFn != nil {
		logfFn(format, args...)
	}
}
