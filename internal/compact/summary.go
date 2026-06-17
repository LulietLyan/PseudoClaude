package compact

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"PseudoClaude/internal/conversation"
	"PseudoClaude/internal/llm"
)

type TriggerKind string

const (
	TriggerAuto   TriggerKind = "auto"
	TriggerManual TriggerKind = "manual"
)

type ManageInput struct {
	Conversation *conversation.Conversation
	Runtime      *Runtime
	Provider     llm.Provider
	Trigger      TriggerKind
	OnProgress   func(message string)
}

type ManageOutput struct {
	TriggeredLayer1 bool
	TriggeredLayer2 bool
	BeforeTokens    int64
	AfterTokens     int64
	OffloadedCount  int
}

var ErrAutoCompactTripped = errors.New("automatic context compaction is tripped")

func ManageContext(ctx context.Context, in ManageInput) (ManageOutput, error) {
	if in.Trigger == TriggerManual {
		return ForceCompact(ctx, in)
	}
	if in.Conversation == nil || in.Runtime == nil {
		return ManageOutput{}, nil
	}
	messages := in.Conversation.Messages()
	before := EstimateWithAnchor(messages, in.Runtime.Snapshot().UsageAnchor)
	output := ManageOutput{BeforeTokens: before, AfterTokens: before}

	layer1 := OffloadToolResults(messages, in.Runtime)
	if layer1.Changed {
		in.Conversation.ReplaceMessages(layer1.Messages)
		output.TriggeredLayer1 = true
		output.OffloadedCount = layer1.OffloadedCount
		resetAnchorToEstimate(in.Runtime, in.Conversation)
	}

	messages = in.Conversation.Messages()
	current := EstimateWithAnchor(messages, in.Runtime.Snapshot().UsageAnchor)
	output.AfterTokens = current
	if !shouldAutoCompact(current, in.Runtime.Snapshot().ContextWindow) {
		return output, layer1.Err
	}
	if in.Runtime.AutoTripped() {
		return output, layer1.Err
	}

	if in.OnProgress != nil {
		in.OnProgress("正在压缩上下文...")
	}
	compactOut, err := compactConversation(ctx, in, AutoSafetyMarginTokens)
	if err != nil {
		in.Runtime.RecordAutoFailure()
		return output, err
	}
	in.Runtime.RecordAutoSuccess()
	compactOut.BeforeTokens = before
	compactOut.TriggeredLayer1 = output.TriggeredLayer1
	compactOut.OffloadedCount = output.OffloadedCount
	return compactOut, layer1.Err
}

func ForceCompact(ctx context.Context, in ManageInput) (ManageOutput, error) {
	return compactConversation(ctx, in, ManualSafetyMarginTokens)
}

func compactConversation(ctx context.Context, in ManageInput, safetyMargin int64) (ManageOutput, error) {
	if in.Conversation == nil {
		return ManageOutput{}, errors.New("conversation is nil")
	}
	if in.Runtime == nil {
		return ManageOutput{}, errors.New("compact runtime is nil")
	}
	if in.Provider == nil {
		return ManageOutput{}, errors.New("provider is nil")
	}
	before := EstimateWithAnchor(in.Conversation.Messages(), in.Runtime.Snapshot().UsageAnchor)
	summary, err := summarize(ctx, in.Provider, in.Conversation.Messages(), safetyMargin, in.Runtime.Snapshot().ContextWindow)
	if err != nil {
		return ManageOutput{BeforeTokens: before, AfterTokens: before}, err
	}
	recent := SelectRecent(in.Conversation.Messages())
	next := buildCompactedMessages(summary, recent)
	in.Conversation.ReplaceMessages(next)
	after := EstimateMessages(next)
	in.Runtime.ResetUsageAnchor(after, in.Conversation.Len())
	return ManageOutput{TriggeredLayer2: true, BeforeTokens: before, AfterTokens: after}, nil
}

func shouldAutoCompact(tokens, contextWindow int64) bool {
	if contextWindow <= 0 {
		contextWindow = defaultFallbackContextWindow
	}
	return tokens >= contextWindow-SummaryReserveTokens-AutoSafetyMarginTokens
}

func resetAnchorToEstimate(rt *Runtime, conv *conversation.Conversation) {
	messages := conv.Messages()
	rt.ResetUsageAnchor(EstimateMessages(messages), len(messages))
}

func summarize(ctx context.Context, provider llm.Provider, messages []llm.Message, safetyMargin int64, contextWindow int64) (string, error) {
	input := cloneMessages(messages)
	var lastErr error
	for attempt := 0; attempt <= SummaryRetryLimit && len(input) > 0; attempt++ {
		summary, err := summarizeOnce(ctx, provider, input)
		if err == nil {
			return summary, nil
		}
		if !errors.Is(err, llm.ErrPromptTooLong) {
			return "", err
		}
		lastErr = err
		input = dropOldestSummaryInput(input)
	}
	if lastErr == nil {
		lastErr = errors.New("summary input is empty")
	}
	return "", lastErr
}

func summarizeOnce(ctx context.Context, provider llm.Provider, messages []llm.Message) (string, error) {
	req := llm.Request{Messages: BuildSummaryPrompt(messages)}
	var b strings.Builder
	for event := range provider.Stream(ctx, req) {
		if event.Err != nil {
			return "", event.Err
		}
		if event.Text != "" {
			b.WriteString(event.Text)
		}
		if event.Done {
			break
		}
	}
	summary := extractSummary(b.String())
	if strings.TrimSpace(summary) == "" {
		return "", errors.New("summary output is empty")
	}
	return summary, nil
}

func BuildSummaryPrompt(messages []llm.Message) []llm.Message {
	return []llm.Message{{
		Role: "user",
		Content: summaryInstruction + "\n\n[conversation]\n" +
			renderConversationForSummary(messages),
	}}
}

const summaryInstruction = `你正在执行上下文压缩任务。不要调用任何工具，只输出纯文本。

先写 <analysis>...</analysis> 包裹的分析草稿，再写 <summary>...</summary> 包裹的正式摘要。系统只会保留正式摘要。

正式摘要必须使用以下固定结构，并按顺序覆盖事实；不要编造未观察到的文件内容、错误原文或代码细节：

1. 主要请求和意图
2. 关键技术概念
3. 文件和代码位置
4. 错误与修复
5. 问题解决过程
6. 用户消息原文记录
7. 待办任务
8. 当前工作状态
9. 可能的下一步

用户消息原文记录应尽量逐条保留用户原话；如果极端长度导致无法保留，请明确标注发生了裁剪。`

func renderConversationForSummary(messages []llm.Message) string {
	var b strings.Builder
	for i, msg := range messages {
		fmt.Fprintf(&b, "## message %d role=%s\n", i+1, msg.Role)
		if msg.Content != "" {
			b.WriteString(msg.Content)
			b.WriteString("\n")
		}
		for _, call := range msg.ToolCalls {
			fmt.Fprintf(&b, "[tool_call] id=%s name=%s args=%s\n", call.ID, call.Name, string(call.Arguments))
		}
		if msg.ToolResult != nil {
			fmt.Fprintf(&b, "[tool_result] call_id=%s name=%s is_error=%v\n%s\n", msg.ToolResult.CallID, msg.ToolResult.Name, msg.ToolResult.IsError, msg.ToolResult.Content)
		}
	}
	return b.String()
}

func extractSummary(raw string) string {
	start := strings.LastIndex(raw, "<summary>")
	end := strings.LastIndex(raw, "</summary>")
	if start < 0 || end < 0 || end <= start {
		return strings.TrimSpace(raw)
	}
	return strings.TrimSpace(raw[start+len("<summary>") : end])
}

func buildCompactedMessages(summary string, recent []llm.Message) []llm.Message {
	out := []llm.Message{
		{Role: "user", Content: "## 历史会话摘要\n" + summary},
		{Role: "assistant", Content: "已记录压缩后的历史摘要。"},
		{Role: "user", Content: boundaryMessage},
	}
	out = append(out, cloneMessages(recent)...)
	return out
}

const boundaryMessage = `上下文已压缩。上方摘要不是代码、错误、工具结果或用户原话的完整原文；需要文件细节、错误细节、工具结果全文或用户原话时，请重新读取相关文件、记录或预览中给出的落盘路径，不要凭摘要脑补。`

func dropOldestSummaryInput(messages []llm.Message) []llm.Message {
	if len(messages) <= summaryPromptTooLargeDropGroups {
		return nil
	}
	return cloneMessages(messages[summaryPromptTooLargeDropGroups:])
}
