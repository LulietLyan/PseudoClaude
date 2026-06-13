package conversation

import "testing"

func TestConversationMessagesPreserveOrderAndRoles(t *testing.T) {
	var c Conversation
	c.AddUser("hello")
	c.AddAssistant("hi")

	msgs := c.Messages()
	if len(msgs) != 2 {
		t.Fatalf("message count = %d, want 2", len(msgs))
	}
	if msgs[0].Role != "user" || msgs[0].Content != "hello" {
		t.Fatalf("first message = %+v", msgs[0])
	}
	if msgs[1].Role != "assistant" || msgs[1].Content != "hi" {
		t.Fatalf("second message = %+v", msgs[1])
	}

	msgs[0].Content = "mutated"
	if c.Messages()[0].Content != "hello" {
		t.Fatal("Messages did not return a copy")
	}
}
