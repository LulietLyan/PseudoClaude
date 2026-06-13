package llm

import (
	"context"

	"PseudoClaude/internal/config"
	"PseudoClaude/internal/prompt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

const anthropicMaxTokens = 4096
const anthropicThinkingBudgetTokens = 1024

type anthropicProvider struct {
	cfg    config.ProviderConfig
	client anthropic.Client
}

func newAnthropicProvider(cfg config.ProviderConfig) Provider {
	opts := []option.RequestOption{option.WithAPIKey(cfg.APIKey)}
	if cfg.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(cfg.BaseURL))
	}
	return anthropicProvider{
		cfg:    cfg,
		client: anthropic.NewClient(opts...),
	}
}

func (p anthropicProvider) Name() string {
	return p.cfg.Name
}

func (p anthropicProvider) Model() string {
	return p.cfg.Model
}

func (p anthropicProvider) Stream(ctx context.Context, msgs []Message) <-chan StreamEvent {
	ch := make(chan StreamEvent)
	go func() {
		defer close(ch)

		params := anthropic.MessageNewParams{
			Model:     anthropic.Model(p.cfg.Model),
			MaxTokens: anthropicMaxTokens,
			System: []anthropic.TextBlockParam{
				{Text: prompt.SystemPrompt},
			},
			Messages: toAnthropicMessages(msgs),
		}
		if p.cfg.Thinking {
			params.Thinking = anthropic.ThinkingConfigParamOfEnabled(anthropicThinkingBudgetTokens)
		}

		stream := p.client.Messages.NewStreaming(ctx, params)
		for stream.Next() {
			select {
			case <-ctx.Done():
				return
			default:
			}

			switch event := stream.Current().AsAny().(type) {
			case anthropic.ContentBlockDeltaEvent:
				switch delta := event.Delta.AsAny().(type) {
				case anthropic.TextDelta:
					sendStreamEvent(ctx, ch, StreamEvent{Text: delta.Text})
				case anthropic.ThinkingDelta:
					continue
				}
			}
		}
		if err := stream.Err(); err != nil {
			sendStreamEvent(ctx, ch, StreamEvent{Err: err})
			return
		}
		sendStreamEvent(ctx, ch, StreamEvent{Done: true})
	}()
	return ch
}

func toAnthropicMessages(msgs []Message) []anthropic.MessageParam {
	out := make([]anthropic.MessageParam, 0, len(msgs))
	for _, msg := range msgs {
		block := anthropic.NewTextBlock(msg.Content)
		if msg.Role == "assistant" {
			out = append(out, anthropic.NewAssistantMessage(block))
			continue
		}
		out = append(out, anthropic.NewUserMessage(block))
	}
	return out
}
