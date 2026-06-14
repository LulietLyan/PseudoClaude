package conversation

import "PseudoClaude/internal/llm"

type Conversation struct {
	messages []llm.Message
}

func (c *Conversation) AddUser(text string) {
	c.messages = append(c.messages, llm.Message{Role: "user", Content: text})
}

func (c *Conversation) AddAssistant(text string) {
	c.messages = append(c.messages, llm.Message{Role: "assistant", Content: text})
}

func (c *Conversation) AddAssistantToolCall(call llm.ToolCall) {
	c.AddAssistantToolCalls([]llm.ToolCall{call})
}

func (c *Conversation) AddAssistantToolCalls(calls []llm.ToolCall) {
	if len(calls) == 0 {
		return
	}
	copyCalls := append([]llm.ToolCall(nil), calls...)
	c.messages = append(c.messages, llm.Message{Role: "assistant", ToolCalls: copyCalls})
}

func (c *Conversation) AddToolResult(result llm.ToolResult) {
	copyResult := result
	c.messages = append(c.messages, llm.Message{Role: "user", ToolResult: &copyResult})
}

func (c *Conversation) Messages() []llm.Message {
	out := make([]llm.Message, len(c.messages))
	for i, msg := range c.messages {
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
