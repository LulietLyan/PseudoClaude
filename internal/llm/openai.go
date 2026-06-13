package llm

import (
	"context"

	"PseudoClaude/internal/config"
	"PseudoClaude/internal/prompt"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

type openAIProvider struct {
	cfg    config.ProviderConfig
	client openai.Client
}

func newOpenAIProvider(cfg config.ProviderConfig) Provider {
	opts := []option.RequestOption{option.WithAPIKey(cfg.APIKey)}
	if cfg.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(cfg.BaseURL))
	}
	return openAIProvider{
		cfg:    cfg,
		client: openai.NewClient(opts...),
	}
}

func (p openAIProvider) Name() string {
	return p.cfg.Name
}

func (p openAIProvider) Model() string {
	return p.cfg.Model
}

func (p openAIProvider) Stream(ctx context.Context, msgs []Message) <-chan StreamEvent {
	ch := make(chan StreamEvent)
	go func() {
		defer close(ch)

		stream := p.client.Chat.Completions.NewStreaming(ctx, openai.ChatCompletionNewParams{
			Model:    openai.ChatModel(p.cfg.Model),
			Messages: toOpenAIMessages(msgs),
		})
		for stream.Next() {
			select {
			case <-ctx.Done():
				return
			default:
			}

			evt := stream.Current()
			if len(evt.Choices) == 0 {
				continue
			}
			if text := evt.Choices[0].Delta.Content; text != "" {
				sendStreamEvent(ctx, ch, StreamEvent{Text: text})
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

func toOpenAIMessages(msgs []Message) []openai.ChatCompletionMessageParamUnion {
	out := make([]openai.ChatCompletionMessageParamUnion, 0, len(msgs)+1)
	out = append(out, openai.SystemMessage(prompt.SystemPrompt))
	for _, msg := range msgs {
		if msg.Role == "assistant" {
			out = append(out, openai.AssistantMessage(msg.Content))
			continue
		}
		out = append(out, openai.UserMessage(msg.Content))
	}
	return out
}
