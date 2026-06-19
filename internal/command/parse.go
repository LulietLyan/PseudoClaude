package command

import (
	"strings"
	"unicode"
)

type ParseResult struct {
	Empty   bool
	IsSlash bool
	Token   string
	Args    string
	Input   string
}

func Parse(input string) ParseResult {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return ParseResult{Empty: true}
	}
	result := ParseResult{Input: trimmed}
	if !strings.HasPrefix(trimmed, "/") {
		return result
	}
	result.IsSlash = true
	split := len(trimmed)
	for i, r := range trimmed {
		if unicode.IsSpace(r) {
			split = i
			break
		}
	}
	result.Token = strings.ToLower(trimmed[:split])
	if split < len(trimmed) {
		result.Args = strings.TrimLeftFunc(trimmed[split:], unicode.IsSpace)
	}
	return result
}
