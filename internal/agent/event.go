package agent

import (
	"time"

	"PseudoClaude/internal/llm"
	"PseudoClaude/internal/tools"
)

type Mode string

const (
	ModeChat Mode = "chat"
	ModePlan Mode = "plan"
	ModeDo   Mode = "do"
)

type EventType string

const (
	EventProgress      EventType = "progress"
	EventTextDelta     EventType = "text_delta"
	EventAssistantText EventType = "assistant_text"
	EventToolCallStart EventType = "tool_call_start"
	EventToolCallDone  EventType = "tool_call_done"
	EventToolResult    EventType = "tool_result"
	EventUsage         EventType = "usage"
	EventStop          EventType = "stop"
	EventError         EventType = "error"
)

type StopReason string

const (
	StopCompleted        StopReason = "completed"
	StopMaxIterations    StopReason = "max_iterations"
	StopCanceled         StopReason = "canceled"
	StopUnknownToolLimit StopReason = "unknown_tool_limit"
	StopStreamError      StopReason = "stream_error"
)

type Event struct {
	Type       EventType
	Iteration  int
	Text       string
	Message    string
	ToolCall   *llm.ToolCall
	ToolResult *ToolResult
	Usage      *llm.Usage
	Stop       *Stop
	Err        error
}

type Stop struct {
	Reason     StopReason
	Message    string
	Iterations int
}

type ToolResult struct {
	Call    llm.ToolCall
	Result  tools.Result
	Elapsed time.Duration
}
