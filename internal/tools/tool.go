package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

type Definition struct {
	Name        string
	Description string
	InputSchema map[string]any
	Safety      Safety
	System      bool
	Timeout     time.Duration
}

type Safety string

const (
	SafetyReadOnly   Safety = "read_only"
	SafetySideEffect Safety = "side_effect"
)

type Env struct {
	CWD              string
	Timeout          time.Duration
	MaxReadBytes     int64
	MaxOutputBytes   int
	MaxSearchResults int
	Team             *TeamEnv
}

type TeamEnv struct {
	TeamName   string
	MemberName string
	AgentID    string
	IsLead     bool
}

type Call struct {
	ID        string
	Name      string
	Arguments json.RawMessage
}

type Result struct {
	OK        bool           `json:"ok"`
	Tool      string         `json:"tool"`
	Content   string         `json:"content,omitempty"`
	ErrorType string         `json:"error_type,omitempty"`
	Error     string         `json:"error,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

type Tool interface {
	Definition() Definition
	Execute(ctx context.Context, input json.RawMessage, env Env) Result
}

func Success(tool, content string, metadata map[string]any) Result {
	return Result{OK: true, Tool: tool, Content: content, Metadata: metadata}
}

func Failure(tool, errorType, message string, metadata map[string]any) Result {
	return Result{OK: false, Tool: tool, ErrorType: errorType, Error: message, Metadata: metadata}
}

func (r Result) JSON() string {
	data, err := json.Marshal(r)
	if err == nil {
		return string(data)
	}
	fallback := Result{
		OK:        false,
		Tool:      r.Tool,
		ErrorType: "serialization_error",
		Error:     fmt.Sprintf("failed to serialize tool result: %v", err),
	}
	data, err = json.Marshal(fallback)
	if err != nil {
		return `{"ok":false,"error_type":"serialization_error","error":"failed to serialize tool result"}`
	}
	return string(data)
}

func objectSchema(properties map[string]any, required ...string) map[string]any {
	if required == nil {
		required = []string{}
	}
	return map[string]any{
		"type":       "object",
		"properties": properties,
		"required":   required,
	}
}

func stringProp(description string) map[string]any {
	return map[string]any{"type": "string", "description": description}
}
