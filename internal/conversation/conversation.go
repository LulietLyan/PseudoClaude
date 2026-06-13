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

func (c *Conversation) Messages() []llm.Message {
	out := make([]llm.Message, len(c.messages))
	copy(out, c.messages)
	return out
}
