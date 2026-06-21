package permission

import (
	"fmt"
	"regexp"
)

type MatchKind string

const (
	MatchGlob  MatchKind = "glob"
	MatchExact MatchKind = "exact"
	MatchRegex MatchKind = "regex"
	MatchNot   MatchKind = "not"
)

type Matcher interface {
	Match(target string, isPath bool) bool
	String() string
}

type MatchSpec struct {
	Type  MatchKind
	Value string
	Inner *MatchSpec
}

func CompileMatcher(pattern string) (Matcher, error) {
	if pattern == "" {
		return nil, fmt.Errorf("empty pattern")
	}
	switch pattern[0] {
	case '=':
		if len(pattern) == 1 {
			return nil, fmt.Errorf("exact matcher requires a value")
		}
		return exactMatcher(pattern[1:]), nil
	case '~':
		if len(pattern) == 1 {
			return nil, fmt.Errorf("regex matcher requires a value")
		}
		re, err := regexp.Compile(pattern[1:])
		if err != nil {
			return nil, fmt.Errorf("invalid regex %q: %w", pattern[1:], err)
		}
		return regexMatcher{pattern: pattern[1:], re: re}, nil
	case '!':
		if len(pattern) == 1 {
			return nil, fmt.Errorf("not matcher requires an inner matcher")
		}
		inner, err := CompileMatcher(pattern[1:])
		if err != nil {
			return nil, fmt.Errorf("invalid not matcher: %w", err)
		}
		return notMatcher{inner: inner}, nil
	default:
		return globMatcher(pattern), nil
	}
}

func CompileMatchSpec(spec MatchSpec) (Matcher, error) {
	switch spec.Type {
	case MatchGlob, "":
		if spec.Value == "" {
			return nil, fmt.Errorf("glob matcher requires a value")
		}
		return globMatcher(spec.Value), nil
	case MatchExact:
		if spec.Value == "" {
			return nil, fmt.Errorf("exact matcher requires a value")
		}
		return exactMatcher(spec.Value), nil
	case MatchRegex:
		if spec.Value == "" {
			return nil, fmt.Errorf("regex matcher requires a value")
		}
		re, err := regexp.Compile(spec.Value)
		if err != nil {
			return nil, fmt.Errorf("invalid regex %q: %w", spec.Value, err)
		}
		return regexMatcher{pattern: spec.Value, re: re}, nil
	case MatchNot:
		if spec.Inner == nil {
			return nil, fmt.Errorf("not matcher requires an inner matcher")
		}
		inner, err := CompileMatchSpec(*spec.Inner)
		if err != nil {
			return nil, fmt.Errorf("invalid not matcher: %w", err)
		}
		return notMatcher{inner: inner}, nil
	default:
		return nil, fmt.Errorf("unknown matcher type %q", spec.Type)
	}
}

type exactMatcher string

func (m exactMatcher) Match(target string, _ bool) bool {
	return string(m) == target
}

func (m exactMatcher) String() string {
	return "=" + string(m)
}

type globMatcher string

func (m globMatcher) Match(target string, isPath bool) bool {
	if isPath {
		return pathGlobMatch(string(m), target)
	}
	return commandGlobMatch(string(m), target)
}

func (m globMatcher) String() string {
	return string(m)
}

type regexMatcher struct {
	pattern string
	re      *regexp.Regexp
}

func (m regexMatcher) Match(target string, _ bool) bool {
	return m.re.MatchString(target)
}

func (m regexMatcher) String() string {
	return "~" + m.pattern
}

type notMatcher struct {
	inner Matcher
}

func (m notMatcher) Match(target string, isPath bool) bool {
	return !m.inner.Match(target, isPath)
}

func (m notMatcher) String() string {
	return "!" + m.inner.String()
}
