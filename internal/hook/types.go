package hook

import (
	"fmt"
	"time"

	"PseudoClaude/internal/permission"
)

type Event string

const (
	EventSessionStart     Event = "SessionStart"
	EventSessionEnd       Event = "SessionEnd"
	EventSessionResume    Event = "SessionResume"
	EventPreUserMessage   Event = "PreUserMessage"
	EventStop             Event = "Stop"
	EventUserPromptSubmit Event = "UserPromptSubmit"
	EventPreToolUse       Event = "PreToolUse"
	EventPostToolUse      Event = "PostToolUse"
	EventPreCompact       Event = "PreCompact"
	EventPostCompact      Event = "PostCompact"
	EventNotification     Event = "Notification"
)

func ParseEvent(value string) (Event, error) {
	event := Event(value)
	for _, known := range AllEvents() {
		if event == known {
			return event, nil
		}
	}
	return "", fmt.Errorf("unknown event %q", value)
}

func IsBlocking(event Event) bool {
	return event == EventPreToolUse || event == EventUserPromptSubmit
}

func AllEvents() []Event {
	return []Event{
		EventSessionStart,
		EventSessionEnd,
		EventSessionResume,
		EventPreUserMessage,
		EventStop,
		EventUserPromptSubmit,
		EventPreToolUse,
		EventPostToolUse,
		EventPreCompact,
		EventPostCompact,
		EventNotification,
	}
}

type CombineMode string

const (
	CombineAllOf CombineMode = "all_of"
	CombineAnyOf CombineMode = "any_of"
)

type Rule struct {
	Name     string
	Event    Event
	If       *Condition
	Action   Action
	OnlyOnce bool
	Async    bool
	Timeout  time.Duration
	Source   string
	Index    int
}

type Condition struct {
	Mode  CombineMode
	Atoms []Atom
}

type Atom struct {
	Field   string
	Matcher permission.Matcher
}

type ActionType string

const (
	ActionShell    ActionType = "shell"
	ActionPrompt   ActionType = "prompt"
	ActionHTTP     ActionType = "http"
	ActionSubagent ActionType = "subagent"
)

type Action struct {
	Type     ActionType
	Shell    *ShellAction
	Prompt   *PromptAction
	HTTP     *HTTPAction
	Subagent *SubagentAction
}

type ShellAction struct {
	Command string
}

type PromptAction struct {
	Text string
}

type HTTPAction struct {
	URL     string
	Method  string
	Headers map[string]string
	Body    string
}

type SubagentAction struct {
	AgentName string
	Prompt    string
}

type Summary struct {
	Name   string
	Event  string
	Action string
	Flags  []string
	Source string
}
