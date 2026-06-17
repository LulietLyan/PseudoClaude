package conversation

import (
	"sync"

	"PseudoClaude/internal/llm"
)

type Conversation struct {
	mu       sync.Mutex
	messages []llm.Message
}

func (c *Conversation) AddUser(text string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.messages = append(c.messages, llm.Message{Role: "user", Content: text})
}

func (c *Conversation) AddAssistant(text string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.messages = append(c.messages, llm.Message{Role: "assistant", Content: text})
}

func (c *Conversation) AddAssistantToolCall(call llm.ToolCall) {
	c.AddAssistantToolCalls([]llm.ToolCall{call})
}

func (c *Conversation) AddAssistantToolCalls(calls []llm.ToolCall) {
	if len(calls) == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	copyCalls := append([]llm.ToolCall(nil), calls...)
	c.messages = append(c.messages, llm.Message{Role: "assistant", ToolCalls: copyCalls})
}

func (c *Conversation) AddToolResult(result llm.ToolResult) {
	c.mu.Lock()
	defer c.mu.Unlock()
	copyResult := result
	c.messages = append(c.messages, llm.Message{Role: "user", ToolResult: &copyResult})
}

func (c *Conversation) Messages() []llm.Message {
	c.mu.Lock()
	defer c.mu.Unlock()
	return copyMessages(c.messages)
}

func (c *Conversation) ReplaceMessages(messages []llm.Message) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.messages = copyMessages(messages)
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
