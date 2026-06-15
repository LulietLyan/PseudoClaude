package llm

import (
	"context"
	"encoding/json"
	"strings"

	"PseudoClaude/internal/config"
	"PseudoClaude/internal/tools"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/shared"
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

func (p openAIProvider) Stream(ctx context.Context, req Request) <-chan StreamEvent {
	ch := make(chan StreamEvent)
	go func() {
		defer close(ch)

		stream := p.client.Chat.Completions.NewStreaming(ctx, openai.ChatCompletionNewParams{
			Model:         openai.ChatModel(p.cfg.Model),
			Messages:      toOpenAIMessages(req),
			Tools:         toOpenAITools(req.Tools),
			StreamOptions: openai.ChatCompletionStreamOptionsParam{IncludeUsage: openai.Bool(true)},
		})
		acc := openai.ChatCompletionAccumulator{}
		sentTools := make(map[string]bool)
		for stream.Next() {
			select {
			case <-ctx.Done():
				return
			default:
			}

			evt := stream.Current()
			acc.AddChunk(evt)
			if usage := openAIUsageFromCompletionUsage(acc.Usage); usage != nil {
				sendStreamEvent(ctx, ch, StreamEvent{Usage: usage})
			}
			if len(evt.Choices) == 0 {
				continue
			}
			if text := evt.Choices[0].Delta.Content; text != "" {
				sendStreamEvent(ctx, ch, StreamEvent{Text: text})
			}
			if toolCall, ok := acc.JustFinishedToolCall(); ok {
				sendOpenAIToolCall(ctx, ch, toolCall.ID, toolCall.Name, toolCall.Arguments)
				sentTools[toolCall.ID] = true
			}
		}
		if err := stream.Err(); err != nil {
			sendStreamEvent(ctx, ch, StreamEvent{Err: err})
			return
		}
		if len(acc.Choices) > 0 {
			for _, call := range acc.Choices[0].Message.ToolCalls {
				if call.ID == "" || sentTools[call.ID] {
					continue
				}
				sendOpenAIToolCall(ctx, ch, call.ID, call.Function.Name, call.Function.Arguments)
			}
		}
		if usage := openAIUsageFromCompletionUsage(acc.Usage); usage != nil {
			sendStreamEvent(ctx, ch, StreamEvent{Usage: usage})
		}
		sendStreamEvent(ctx, ch, StreamEvent{Done: true})
	}()
	return ch
}

func toOpenAIMessages(req Request) []openai.ChatCompletionMessageParamUnion {
	out := make([]openai.ChatCompletionMessageParamUnion, 0, len(req.Messages)+2)
	system := strings.TrimSpace(req.System.Stable)
	if environment := strings.TrimSpace(req.System.Environment); environment != "" {
		if system != "" {
			system += "\n\n" + environment
		} else {
			system = environment
		}
	}
	if system != "" {
		out = append(out, openai.SystemMessage(system))
	}
	for _, msg := range req.Messages {
		if msg.ToolResult != nil {
			out = append(out, openai.ToolMessage(msg.ToolResult.Content, msg.ToolResult.CallID))
			continue
		}
		if len(msg.ToolCalls) > 0 {
			assistant := openai.ChatCompletionAssistantMessageParam{}
			if msg.Content != "" {
				assistant.Content.OfString = openai.String(msg.Content)
			}
			for _, call := range msg.ToolCalls {
				assistant.ToolCalls = append(assistant.ToolCalls, openai.ChatCompletionMessageToolCallUnionParam{
					OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
						ID: call.ID,
						Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
							Name:      call.Name,
							Arguments: string(call.Arguments),
						},
					},
				})
			}
			out = append(out, openai.ChatCompletionMessageParamUnion{OfAssistant: &assistant})
			continue
		}
		if msg.Role == "assistant" {
			out = append(out, openai.AssistantMessage(msg.Content))
			continue
		}
		out = append(out, openai.UserMessage(msg.Content))
	}
	if reminder := strings.TrimSpace(req.Reminder); reminder != "" {
		out = append(out, openai.UserMessage(reminder))
	}
	return out
}

func toOpenAITools(defs []tools.Definition) []openai.ChatCompletionToolUnionParam {
	if len(defs) == 0 {
		return nil
	}
	out := make([]openai.ChatCompletionToolUnionParam, 0, len(defs))
	for _, def := range defs {
		out = append(out, openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
			Name:        def.Name,
			Description: openai.String(def.Description),
			Parameters:  openai.FunctionParameters(def.InputSchema),
		}))
	}
	return out
}

func sendOpenAIToolCall(ctx context.Context, ch chan<- StreamEvent, id, name, arguments string) {
	sendStreamEvent(ctx, ch, StreamEvent{ToolCall: &ToolCall{
		ID:        id,
		Name:      name,
		Arguments: json.RawMessage(arguments),
	}})
}

func openAIUsageFromCompletionUsage(usage openai.CompletionUsage) *Usage {
	if usage.PromptTokens == 0 && usage.CompletionTokens == 0 && usage.TotalTokens == 0 && usage.PromptTokensDetails.CachedTokens == 0 {
		return nil
	}
	return &Usage{
		InputTokens:  usage.PromptTokens,
		OutputTokens: usage.CompletionTokens,
		TotalTokens:  usage.TotalTokens,
		CacheRead:    usage.PromptTokensDetails.CachedTokens,
	}
}
