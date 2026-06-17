package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"PseudoClaude/internal/config"
	"PseudoClaude/internal/tools"
)

var ErrPromptTooLong = errors.New("prompt too long")

func wrapPromptTooLong(err error) error {
	if err == nil {
		return nil
	}
	msg := strings.ToLower(err.Error())
	patterns := []string{
		"prompt is too long",
		"prompt too long",
		"context length",
		"context_length",
		"maximum context",
		"max context",
		"too many tokens",
		"exceeds the context",
		"exceeded context",
	}
	for _, pattern := range patterns {
		if strings.Contains(msg, pattern) {
			return fmt.Errorf("%w: %v", ErrPromptTooLong, err)
		}
	}
	return err
}

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
	InputTokens  int64
	OutputTokens int64
	TotalTokens  int64
	CacheWrite   int64
	CacheRead    int64
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

type System struct {
	Stable      string
	Environment string
}

type Request struct {
	Messages []Message
	Tools    []tools.Definition
	System   System
	Reminder string
}

type Provider interface {
	Name() string
	Model() string
	Stream(ctx context.Context, req Request) <-chan StreamEvent
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
