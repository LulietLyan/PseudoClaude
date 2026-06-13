package llm

import (
	"context"
	"fmt"

	"PseudoClaude/internal/config"
)

type Message struct {
	Role    string
	Content string
}

type StreamEvent struct {
	Text string
	Done bool
	Err  error
}

type Provider interface {
	Name() string
	Model() string
	Stream(ctx context.Context, msgs []Message) <-chan StreamEvent
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
