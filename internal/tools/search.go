package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type findFilesTool struct{}
type searchCodeTool struct{}

func NewFindFilesTool() Tool  { return findFilesTool{} }
func NewSearchCodeTool() Tool { return searchCodeTool{} }

func (findFilesTool) Definition() Definition {
	return Definition{
		Name:        "find_files",
		Description: "Dedicated tool for finding files by glob pattern relative to the current workspace. Prefer this over shell commands for locating files.",
		Safety:      SafetyReadOnly,
		InputSchema: objectSchema(map[string]any{
			"pattern": stringProp("Glob pattern, such as *.go or **/*.md."),
		}, "pattern"),
	}
}

func (findFilesTool) Execute(ctx context.Context, input json.RawMessage, env Env) Result {
	var args struct {
		Pattern string `json:"pattern"`
	}
	if err := decodeArgs(input, &args); err != nil {
		return invalidArgs("find_files", err)
	}
	args.Pattern = strings.TrimSpace(args.Pattern)
	if args.Pattern == "" {
		return invalidArgs("find_files", errors.New("pattern is required"))
	}
	matches, err := findMatches(ctx, env, args.Pattern)
	if err != nil {
		if ctx.Err() != nil {
			return timeoutResult("find_files", ctx)
		}
		return Failure("find_files", "io_error", err.Error(), nil)
	}
	sort.Strings(matches)
	limited, truncated := limitStrings(matches, env.MaxSearchResults)
	return Success("find_files", strings.Join(limited, "\n"), map[string]any{
		"matches":   limited,
		"count":     len(limited),
		"truncated": truncated,
	})
}

func (searchCodeTool) Definition() Definition {
	return Definition{
		Name:        "search_code",
		Description: "Dedicated tool for searching text or regex patterns in local text files and returning file, line, and summary matches. Prefer this over shell commands for code or text search.",
		Safety:      SafetyReadOnly,
		InputSchema: objectSchema(map[string]any{
			"pattern": stringProp("Text or regular expression to search for."),
			"regex": map[string]any{
				"type":        "boolean",
				"description": "Treat pattern as a Go regular expression.",
			},
			"path": stringProp("Optional file or directory to search. Defaults to workspace root."),
		}, "pattern"),
	}
}

func (searchCodeTool) Execute(ctx context.Context, input json.RawMessage, env Env) Result {
	var args struct {
		Pattern string `json:"pattern"`
		Regex   bool   `json:"regex"`
		Path    string `json:"path"`
	}
	if err := decodeArgs(input, &args); err != nil {
		return invalidArgs("search_code", err)
	}
	if args.Pattern == "" {
		return invalidArgs("search_code", errors.New("pattern is required"))
	}
	var re *regexp.Regexp
	if args.Regex {
		compiled, err := regexp.Compile(args.Pattern)
		if err != nil {
			return invalidArgs("search_code", err)
		}
		re = compiled
	}
	root := env.CWD
	if strings.TrimSpace(args.Path) != "" {
		path, err := resolvePath(env, args.Path)
		if err != nil {
			return invalidArgs("search_code", err)
		}
		root = path
	}
	matches, err := searchPath(ctx, root, args.Pattern, re, env)
	if err != nil {
		if ctx.Err() != nil {
			return timeoutResult("search_code", ctx)
		}
		return Failure("search_code", "io_error", err.Error(), map[string]any{"path": root})
	}
	limited, truncated := limitStrings(matches, env.MaxSearchResults)
	return Success("search_code", strings.Join(limited, "\n"), map[string]any{
		"matches":   limited,
		"count":     len(limited),
		"truncated": truncated,
	})
}

func findMatches(ctx context.Context, env Env, pattern string) ([]string, error) {
	fullPattern := pattern
	if !filepath.IsAbs(fullPattern) {
		fullPattern = filepath.Join(env.CWD, fullPattern)
	}
	if !strings.Contains(pattern, "**") {
		return filepath.Glob(fullPattern)
	}
	base, suffix := splitRecursivePattern(fullPattern)
	re, err := recursiveGlobRegexp(fullPattern)
	if err != nil {
		return nil, err
	}
	var matches []string
	err = filepath.WalkDir(base, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if d.IsDir() {
			return nil
		}
		if re.MatchString(filepath.ToSlash(path)) {
			matches = append(matches, path)
			return nil
		}
		ok, err := filepath.Match(suffix, path)
		if err == nil && ok {
			matches = append(matches, path)
		}
		return nil
	})
	return matches, err
}

func splitRecursivePattern(pattern string) (string, string) {
	idx := strings.Index(pattern, "**")
	base := filepath.Clean(pattern[:idx])
	if base == "" || base == string(filepath.Separator) {
		base = string(filepath.Separator)
	}
	suffix := pattern
	return base, suffix
}

func recursiveGlobRegexp(pattern string) (*regexp.Regexp, error) {
	p := filepath.ToSlash(pattern)
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(p); {
		switch {
		case strings.HasPrefix(p[i:], "**/"):
			b.WriteString("(?:.*/)?")
			i += 3
		case strings.HasPrefix(p[i:], "**"):
			b.WriteString(".*")
			i += 2
		case p[i] == '*':
			b.WriteString("[^/]*")
			i++
		case p[i] == '?':
			b.WriteString("[^/]")
			i++
		default:
			b.WriteString(regexp.QuoteMeta(string(p[i])))
			i++
		}
	}
	b.WriteString("$")
	return regexp.Compile(b.String())
}

func searchPath(ctx context.Context, root string, pattern string, re *regexp.Regexp, env Env) ([]string, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	var matches []string
	searchFile := func(path string) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		fileMatches, err := searchFileLines(path, pattern, re, env)
		if err != nil {
			return nil
		}
		matches = append(matches, fileMatches...)
		return nil
	}
	if !info.IsDir() {
		return matches, searchFile(root)
	}
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		return searchFile(path)
	})
	sort.Strings(matches)
	return matches, err
}

func searchFileLines(path, pattern string, re *regexp.Regexp, env Env) ([]string, error) {
	data, truncated, err := readTextFile(path, env.MaxReadBytes)
	if err != nil {
		return nil, err
	}
	var matches []string
	scanner := bufio.NewScanner(strings.NewReader(data))
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := scanner.Text()
		found := strings.Contains(line, pattern)
		if re != nil {
			found = re.MatchString(line)
		}
		if found {
			summary, _ := truncateString(strings.TrimSpace(line), 200)
			matches = append(matches, fmt.Sprintf("%s:%d:%s", path, lineNo, summary))
		}
	}
	if err := scanner.Err(); err != nil {
		return matches, err
	}
	if truncated && len(matches) == 0 {
		return matches, nil
	}
	return matches, nil
}
