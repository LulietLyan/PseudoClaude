package permission

import (
	"encoding/json"
	"path/filepath"
	"strconv"
	"strings"

	"PseudoClaude/internal/llm"
	"PseudoClaude/internal/tools"
)

var friendlyByInternal = map[string]string{
	"run_command": "Bash",
	"read_file":   "Read",
	"write_file":  "Write",
	"edit_file":   "Edit",
	"find_files":  "Glob",
	"search_code": "Grep",
}

var internalByFriendly = map[string]string{
	"Bash":  "run_command",
	"Read":  "read_file",
	"Write": "write_file",
	"Edit":  "edit_file",
	"Glob":  "find_files",
	"Grep":  "search_code",
}

func friendlyName(internal string) string {
	if isMCPToolName(internal) {
		return internal
	}
	return friendlyByInternal[internal]
}

func internalName(friendly string) string {
	if isMCPToolName(friendly) || isMCPToolGlob(friendly) {
		return friendly
	}
	return internalByFriendly[friendly]
}

func isMCPToolName(name string) bool {
	return strings.HasPrefix(name, "mcp__")
}

func isMCPToolGlob(name string) bool {
	return strings.HasPrefix(name, "mcp__") && strings.ContainsAny(name, "*?")
}

func classify(call llm.ToolCall, safety tools.Safety) (Category, bool) {
	switch call.Name {
	case "read_file", "find_files", "search_code":
		return CategoryRead, true
	case "write_file", "edit_file":
		return CategoryWrite, true
	case "run_command":
		return CategoryExec, true
	default:
		switch safety {
		case tools.SafetyReadOnly:
			return CategoryRead, true
		case tools.SafetySideEffect:
			return CategoryWrite, true
		default:
			return "", false
		}
	}
}

func commandText(call llm.ToolCall) (string, bool) {
	if call.Name != "run_command" {
		return "", false
	}
	var args struct {
		Command string   `json:"command"`
		Args    []string `json:"args"`
	}
	if err := json.Unmarshal(call.Arguments, &args); err != nil {
		return "", false
	}
	args.Command = strings.TrimSpace(args.Command)
	if args.Command == "" {
		return "", false
	}
	parts := []string{args.Command}
	for _, arg := range args.Args {
		if strings.ContainsAny(arg, " \t\n\"'\\") {
			parts = append(parts, strconv.Quote(arg))
		} else {
			parts = append(parts, arg)
		}
	}
	return strings.Join(parts, " "), true
}

func CommandTextForDisplay(call llm.ToolCall) (string, bool) {
	return commandText(call)
}

func pathTarget(call llm.ToolCall) (target string, matchTarget string, ok bool) {
	switch call.Name {
	case "read_file", "write_file", "edit_file":
		var args struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(call.Arguments, &args); err != nil || strings.TrimSpace(args.Path) == "" {
			return "", "", false
		}
		return args.Path, args.Path, true
	case "search_code":
		var args struct {
			Pattern string `json:"pattern"`
			Path    string `json:"path"`
		}
		if err := json.Unmarshal(call.Arguments, &args); err != nil || strings.TrimSpace(args.Pattern) == "" {
			return "", "", false
		}
		if strings.TrimSpace(args.Path) == "" {
			return ".", ".", true
		}
		return args.Path, args.Path, true
	case "find_files":
		var args struct {
			Pattern string `json:"pattern"`
		}
		if err := json.Unmarshal(call.Arguments, &args); err != nil || strings.TrimSpace(args.Pattern) == "" {
			return "", "", false
		}
		return globStaticRoot(args.Pattern), args.Pattern, true
	default:
		return "", "", false
	}
}

func TargetForDisplay(call llm.ToolCall) (string, bool) {
	target, matchTarget, ok := pathTarget(call)
	if !ok {
		return "", false
	}
	if call.Name == "find_files" {
		return matchTarget, true
	}
	return target, true
}

func pathToolRequiresExactPath(name string) bool {
	switch name {
	case "read_file", "write_file", "edit_file", "search_code":
		return true
	default:
		return false
	}
}

func pathContainsGlob(path string) bool {
	return strings.ContainsAny(path, "*?[")
}

func globStaticRoot(pattern string) string {
	pattern = filepath.Clean(pattern)
	idx := len(pattern)
	for _, marker := range []string{"*", "?", "["} {
		if i := strings.Index(pattern, marker); i >= 0 && i < idx {
			idx = i
		}
	}
	if idx == len(pattern) {
		return pattern
	}
	prefix := pattern[:idx]
	root := filepath.Dir(prefix)
	if root == "." && strings.HasPrefix(prefix, string(filepath.Separator)) {
		return string(filepath.Separator)
	}
	if root == "." && strings.Trim(prefix, string(filepath.Separator)) == "" {
		return "."
	}
	return root
}
