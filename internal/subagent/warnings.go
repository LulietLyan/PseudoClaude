package subagent

import (
	"fmt"
	"strings"
)

type Warning struct {
	Path    string
	Agent   string
	Field   string
	Message string
}

func (w Warning) Empty() bool {
	return w.Path == "" && w.Agent == "" && w.Field == "" && w.Message == ""
}

func (w Warning) WithContext(path, agent string) Warning {
	if w.Path == "" {
		w.Path = path
	}
	if w.Agent == "" {
		w.Agent = agent
	}
	return w
}

func FormatWarning(w Warning) string {
	parts := make([]string, 0, 4)
	if w.Path != "" {
		parts = append(parts, w.Path)
	}
	if w.Agent != "" {
		parts = append(parts, "agent="+w.Agent)
	}
	if w.Field != "" {
		parts = append(parts, "field="+w.Field)
	}
	message := strings.TrimSpace(w.Message)
	if message == "" {
		message = "invalid subagent definition"
	}
	if len(parts) == 0 {
		return message
	}
	return fmt.Sprintf("%s: %s", strings.Join(parts, " "), message)
}

func FormatWarnings(warnings []Warning) []string {
	out := make([]string, 0, len(warnings))
	for _, warning := range warnings {
		out = append(out, FormatWarning(warning))
	}
	return out
}
