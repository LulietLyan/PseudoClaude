package llm

import (
	"context"
	"encoding/json"
	"fmt"

	"PseudoClaude/internal/config"
	"PseudoClaude/internal/tools"
)

type Message struct {
	Role       string
	Content    string
	ToolCalls  []ToolCall
	ToolResult *ToolResult
}

type StreamEvent struct {
	Text     string
	ToolCall *ToolCall
	Usage    *Usage
	Done     bool
	Err      error
}

type Usage struct {
	InputTokens  int
	OutputTokens int
	TotalTokens  int
}

type ToolCall struct {
	ID        string
	Name      string
	Arguments json.RawMessage
}

type ToolResult struct {
	CallID  string
	Name    string
	Content string
	IsError bool
}

type Provider interface {
	Name() string
	Model() string
	Stream(ctx context.Context, msgs []Message, defs []tools.Definition) <-chan StreamEvent
}

func New(cfg config.ProviderConfig) (Provider, error) {
	switch cfg.Protocol {
	case "anthropic":
		return newAnthropicProvider(cfg), nil
	case "openai":
		return newOpenAIProvider(cfg), nil
	default:
		return nil, fmt.Errorf("unsupported protocol: %s", cfg.Protocol)
	}
}
