package conversation

import (
	"sync"

	"PseudoClaude/internal/llm"
)

type Conversation struct {
	mu       sync.Mutex
	messages []llm.Message
	hooks    Hooks
}

type ReplaceReason string

const (
	ReplaceReasonSnapshot ReplaceReason = "snapshot"
	ReplaceReasonCompact  ReplaceReason = "compact"
)

type Hooks struct {
	OnAppend  func(llm.Message)
	OnReplace func(ReplaceReason, []llm.Message)
}

func New(hooks Hooks) *Conversation {
	return &Conversation{hooks: hooks}
}

func NewFromMessages(messages []llm.Message, hooks Hooks) *Conversation {
	return &Conversation{messages: copyMessages(messages), hooks: hooks}
}

func (c *Conversation) SetHooks(hooks Hooks) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.hooks = hooks
}

func (c *Conversation) AddUser(text string) {
	msg := llm.Message{Role: "user", Content: text}
	c.mu.Lock()
	c.messages = append(c.messages, msg)
	hook := c.hooks.OnAppend
	c.mu.Unlock()
	callAppendHook(hook, msg)
}

func (c *Conversation) AddAssistant(text string) {
	msg := llm.Message{Role: "assistant", Content: text}
	c.mu.Lock()
	c.messages = append(c.messages, msg)
	hook := c.hooks.OnAppend
	c.mu.Unlock()
	callAppendHook(hook, msg)
}

func (c *Conversation) AddAssistantToolCall(call llm.ToolCall) {
	c.AddAssistantToolCalls([]llm.ToolCall{call})
}

func (c *Conversation) AddAssistantToolCalls(calls []llm.ToolCall) {
	if len(calls) == 0 {
		return
	}
	msg := llm.Message{Role: "assistant", ToolCalls: append([]llm.ToolCall(nil), calls...)}
	c.mu.Lock()
	c.messages = append(c.messages, msg)
	hook := c.hooks.OnAppend
	c.mu.Unlock()
	callAppendHook(hook, msg)
}

func (c *Conversation) AddToolResult(result llm.ToolResult) {
	copyResult := result
	msg := llm.Message{Role: "user", ToolResult: &copyResult}
	c.mu.Lock()
	c.messages = append(c.messages, msg)
	hook := c.hooks.OnAppend
	c.mu.Unlock()
	callAppendHook(hook, msg)
}

func (c *Conversation) Messages() []llm.Message {
	c.mu.Lock()
	defer c.mu.Unlock()
	return copyMessages(c.messages)
}

func (c *Conversation) ReplaceMessages(reason ReplaceReason, messages []llm.Message) {
	copied := copyMessages(messages)
	c.mu.Lock()
	c.messages = copied
	hook := c.hooks.OnReplace
	c.mu.Unlock()
	callReplaceHook(hook, reason, copied)
}

func (c *Conversation) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.messages)
}

func copyMessages(messages []llm.Message) []llm.Message {
	out := make([]llm.Message, len(messages))
	for i, msg := range messages {
		out[i] = msg
		if msg.ToolCalls != nil {
			out[i].ToolCalls = append([]llm.ToolCall(nil), msg.ToolCalls...)
		}
		if msg.ToolResult != nil {
			result := *msg.ToolResult
			out[i].ToolResult = &result
		}
	}
	return out
}

func callAppendHook(hook func(llm.Message), msg llm.Message) {
	if hook == nil {
		return
	}
	hook(copyMessages([]llm.Message{msg})[0])
}

func callReplaceHook(hook func(ReplaceReason, []llm.Message), reason ReplaceReason, messages []llm.Message) {
	if hook == nil {
		return
	}
	hook(reason, copyMessages(messages))
}
