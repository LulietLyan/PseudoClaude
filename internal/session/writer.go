package session

import (
	"encoding/json"
	"os"
	"sync"
	"time"

	"PseudoClaude/internal/conversation"
	"PseudoClaude/internal/llm"
)

const (
	EntryMessage = "message"
	EntryReplace = "replace"

	ReplaceSnapshot = "snapshot"
	ReplaceCompact  = "compact"
)

type Entry struct {
	Type       string          `json:"type,omitempty"`
	Reason     string          `json:"reason,omitempty"`
	Role       string          `json:"role,omitempty"`
	Content    string          `json:"content,omitempty"`
	ToolCalls  []llm.ToolCall  `json:"tool_calls,omitempty"`
	ToolResult *llm.ToolResult `json:"tool_result,omitempty"`
	TS         int64           `json:"ts"`
	Model      string          `json:"model,omitempty"`
}

type Writer struct {
	mu       sync.Mutex
	file     *os.File
	model    string
	wroteMsg bool
	onError  func(error)
}

func NewWriter(ctx Context, model string, onError func(error)) (*Writer, error) {
	if err := os.MkdirAll(ctx.SpillDir, 0o755); err != nil {
		return nil, err
	}
	return openWriter(ctx, model, onError)
}

func OpenWriter(ctx Context, model string, onError func(error)) (*Writer, error) {
	if err := os.MkdirAll(ctx.SpillDir, 0o755); err != nil {
		return nil, err
	}
	return openWriter(ctx, model, onError)
}

func openWriter(ctx Context, model string, onError func(error)) (*Writer, error) {
	file, err := os.OpenFile(ctx.JSONLPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	return &Writer{file: file, model: model, onError: onError}, nil
}

func (w *Writer) AppendMessage(msg llm.Message) {
	if w == nil {
		return
	}
	entry := entryFromMessage(msg)
	w.mu.Lock()
	if !w.wroteMsg && w.model != "" {
		entry.Model = w.model
	}
	err := w.writeEntryLocked(entry)
	if err == nil {
		w.wroteMsg = true
	}
	w.mu.Unlock()
	w.report(err)
}

func (w *Writer) AppendReplace(reason string, msgs []llm.Message) {
	if w == nil {
		return
	}
	w.mu.Lock()
	err := w.writeEntryLocked(Entry{Type: EntryReplace, Reason: reason, TS: time.Now().Unix()})
	for _, msg := range msgs {
		if err != nil {
			break
		}
		entry := entryFromMessage(msg)
		if !w.wroteMsg && w.model != "" {
			entry.Model = w.model
		}
		err = w.writeEntryLocked(entry)
		if err == nil {
			w.wroteMsg = true
		}
	}
	w.mu.Unlock()
	w.report(err)
}

func (w *Writer) Hooks() conversation.Hooks {
	return conversation.Hooks{
		OnAppend: w.AppendMessage,
		OnReplace: func(reason conversation.ReplaceReason, msgs []llm.Message) {
			w.AppendReplace(string(reason), msgs)
		},
	}
}

func (w *Writer) Close() error {
	if w == nil || w.file == nil {
		return nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.file.Close()
}

func entryFromMessage(msg llm.Message) Entry {
	return Entry{
		Type:       EntryMessage,
		Role:       msg.Role,
		Content:    msg.Content,
		ToolCalls:  append([]llm.ToolCall(nil), msg.ToolCalls...),
		ToolResult: msg.ToolResult,
		TS:         time.Now().Unix(),
	}
}

func (w *Writer) writeEntryLocked(entry Entry) error {
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	if _, err := w.file.Write(append(data, '\n')); err != nil {
		return err
	}
	return w.file.Sync()
}

func (w *Writer) report(err error) {
	if err != nil && w.onError != nil {
		w.onError(err)
	}
}
