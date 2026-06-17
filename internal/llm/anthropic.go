package llm

import (
	"context"
	"encoding/json"
	"strings"

	"PseudoClaude/internal/config"
	"PseudoClaude/internal/tools"

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

func (p anthropicProvider) Stream(ctx context.Context, req Request) <-chan StreamEvent {
	ch := make(chan StreamEvent)
	go func() {
		defer close(ch)

		anthropicTools := toAnthropicTools(req.Tools)
		msgs := appendAnthropicReminder(toAnthropicMessages(req.Messages), req.Reminder)
		params := anthropic.MessageNewParams{
			Model:     anthropic.Model(p.cfg.Model),
			MaxTokens: anthropicMaxTokens,
			System:    toAnthropicSystem(req.System),
			Messages:  msgs,
			Tools:     anthropicTools,
		}
		if p.cfg.Thinking {
			params.Thinking = anthropic.ThinkingConfigParamOfEnabled(anthropicThinkingBudgetTokens)
		}

		stream := p.client.Messages.NewStreaming(ctx, params)
		message := anthropic.Message{}
		for stream.Next() {
			select {
			case <-ctx.Done():
				return
			default:
			}

			current := stream.Current()
			_ = message.Accumulate(current)
			switch event := current.AsAny().(type) {
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
			sendStreamEvent(ctx, ch, StreamEvent{Err: wrapPromptTooLong(err)})
			return
		}
		sendStreamEvent(ctx, ch, StreamEvent{Usage: anthropicUsage(message.Usage)})
		for _, block := range message.Content {
			if toolUse, ok := block.AsAny().(anthropic.ToolUseBlock); ok {
				sendStreamEvent(ctx, ch, StreamEvent{ToolCall: &ToolCall{
					ID:        toolUse.ID,
					Name:      toolUse.Name,
					Arguments: json.RawMessage(toolUse.Input),
				}})
			}
		}
		sendStreamEvent(ctx, ch, StreamEvent{Done: true})
	}()
	return ch
}

func toAnthropicSystem(sys System) []anthropic.TextBlockParam {
	var out []anthropic.TextBlockParam
	if stable := strings.TrimSpace(sys.Stable); stable != "" {
		out = append(out, anthropic.TextBlockParam{
			Text: stable,
			CacheControl: anthropic.CacheControlEphemeralParam{
				TTL: anthropic.CacheControlEphemeralTTLTTL5m,
			},
		})
	}
	if environment := strings.TrimSpace(sys.Environment); environment != "" {
		out = append(out, anthropic.TextBlockParam{Text: environment})
	}
	return out
}

func toAnthropicMessages(msgs []Message) []anthropic.MessageParam {
	out := make([]anthropic.MessageParam, 0, len(msgs))
	for _, msg := range msgs {
		if msg.ToolResult != nil {
			out = append(out, anthropic.NewUserMessage(anthropic.NewToolResultBlock(msg.ToolResult.CallID, msg.ToolResult.Content, msg.ToolResult.IsError)))
			continue
		}
		if len(msg.ToolCalls) > 0 {
			blocks := make([]anthropic.ContentBlockParamUnion, 0, len(msg.ToolCalls))
			if msg.Content != "" {
				blocks = append(blocks, anthropic.NewTextBlock(msg.Content))
			}
			for _, call := range msg.ToolCalls {
				blocks = append(blocks, anthropic.NewToolUseBlock(call.ID, json.RawMessage(call.Arguments), call.Name))
			}
			out = append(out, anthropic.NewAssistantMessage(blocks...))
			continue
		}
		block := anthropic.NewTextBlock(msg.Content)
		if msg.Role == "assistant" {
			out = append(out, anthropic.NewAssistantMessage(block))
		} else {
			out = append(out, anthropic.NewUserMessage(block))
		}
	}
	return out
}

func appendAnthropicReminder(msgs []anthropic.MessageParam, reminder string) []anthropic.MessageParam {
	reminder = strings.TrimSpace(reminder)
	if reminder == "" {
		return msgs
	}
	out := append([]anthropic.MessageParam(nil), msgs...)
	block := anthropic.NewTextBlock(reminder)
	last := len(out) - 1
	if last >= 0 && out[last].Role == anthropic.MessageParamRoleUser {
		out[last].Content = append(out[last].Content, block)
		return out
	}
	return append(out, anthropic.NewUserMessage(block))
}

func toAnthropicTools(defs []tools.Definition) []anthropic.ToolUnionParam {
	if len(defs) == 0 {
		return nil
	}
	out := make([]anthropic.ToolUnionParam, 0, len(defs))
	for _, def := range defs {
		schema := anthropic.ToolInputSchemaParam{}
		if def.InputSchema != nil {
			if props, ok := def.InputSchema["properties"]; ok {
				schema.Properties = props
			}
			if required, ok := stringSlice(def.InputSchema["required"]); ok {
				schema.Required = required
			}
			extras := make(map[string]any)
			for key, value := range def.InputSchema {
				if key != "type" && key != "properties" && key != "required" {
					extras[key] = value
				}
			}
			if len(extras) > 0 {
				schema.ExtraFields = extras
			}
		}
		tool := anthropic.ToolParam{
			Name:        def.Name,
			Description: anthropic.String(def.Description),
			InputSchema: schema,
		}
		out = append(out, anthropic.ToolUnionParam{OfTool: &tool})
	}
	return out
}

func anthropicUsage(usage anthropic.Usage) *Usage {
	total := usage.InputTokens + usage.OutputTokens + usage.CacheCreationInputTokens + usage.CacheReadInputTokens
	return &Usage{
		InputTokens:  usage.InputTokens,
		OutputTokens: usage.OutputTokens,
		TotalTokens:  total,
		CacheWrite:   usage.CacheCreationInputTokens,
		CacheRead:    usage.CacheReadInputTokens,
	}
}

func stringSlice(value any) ([]string, bool) {
	switch v := value.(type) {
	case []string:
		return v, true
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			s, ok := item.(string)
			if !ok {
				return nil, false
			}
			out = append(out, s)
		}
		return out, true
	default:
		return nil, false
	}
}
