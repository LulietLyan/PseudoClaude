package mcp

import (
	"strings"
	"unicode"
)

const toolPrefix = "mcp__"

func FullToolName(server, tool string) string {
	return toolPrefix + server + "__" + tool
}

func ValidToolName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		if r > unicode.MaxASCII {
			return false
		}
		if r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' || r == '-' {
			continue
		}
		return false
	}
	return true
}

func SplitToolName(name string) (server, tool string, ok bool) {
	rest, ok := strings.CutPrefix(name, toolPrefix)
	if !ok {
		return "", "", false
	}
	server, tool, ok = strings.Cut(rest, "__")
	if !ok || server == "" || tool == "" || strings.Contains(tool, "__") {
		return "", "", false
	}
	return server, tool, true
}
