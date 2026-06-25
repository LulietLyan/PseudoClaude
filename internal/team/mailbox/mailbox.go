package mailbox

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"PseudoClaude/internal/team/filelock"
)

type MessageType string

const (
	MessageText                 MessageType = "text"
	MessageShutdownRequest      MessageType = "shutdown_request"
	MessageShutdownResponse     MessageType = "shutdown_response"
	MessagePlanApprovalResponse MessageType = "plan_approval_response"
)

type Message struct {
	From      string         `json:"from"`
	To        string         `json:"to"`
	Type      MessageType    `json:"type"`
	Summary   string         `json:"summary"`
	Content   string         `json:"content,omitempty"`
	Payload   map[string]any `json:"payload,omitempty"`
	Timestamp time.Time      `json:"timestamp"`
	Read      bool           `json:"read"`
}

type IndexedMessage struct {
	Index   int     `json:"index"`
	Message Message `json:"message"`
}

type Box struct {
	Messages []Message `json:"messages"`
}

type Store struct {
	dir string
}

func New(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &Store{dir: dir}, nil
}

func (s *Store) Write(ctx context.Context, agentID string, msg Message) error {
	if msg.Type == "" {
		msg.Type = MessageText
	}
	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now()
	}
	msg.Read = false
	return s.withLock(ctx, agentID, func(box *Box) error {
		box.Messages = append(box.Messages, msg)
		return nil
	})
}

func (s *Store) Read(agentID string) ([]Message, error) {
	box, err := s.readBox(agentID)
	if err != nil {
		return nil, err
	}
	return append([]Message(nil), box.Messages...), nil
}

func (s *Store) ReadUnread(agentID string) ([]IndexedMessage, error) {
	box, err := s.readBox(agentID)
	if err != nil {
		return nil, err
	}
	out := []IndexedMessage{}
	for i, msg := range box.Messages {
		if !msg.Read {
			out = append(out, IndexedMessage{Index: i, Message: msg})
		}
	}
	return out, nil
}

func (s *Store) MarkRead(ctx context.Context, agentID string, indices []int) error {
	if len(indices) == 0 {
		return nil
	}
	set := map[int]bool{}
	for _, idx := range indices {
		set[idx] = true
	}
	return s.withLock(ctx, agentID, func(box *Box) error {
		for idx := range set {
			if idx >= 0 && idx < len(box.Messages) {
				box.Messages[idx].Read = true
			}
		}
		return nil
	})
}

func (s *Store) withLock(ctx context.Context, agentID string, mutate func(*Box) error) error {
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return err
	}
	path := s.path(agentID)
	release, err := filelock.Acquire(ctx, path+".lock", filelock.Options{})
	if err != nil {
		return err
	}
	defer release()
	box, err := s.readBox(agentID)
	if err != nil {
		return err
	}
	if err := mutate(box); err != nil {
		return err
	}
	return atomicWriteJSON(path, box)
}

func (s *Store) readBox(agentID string) (*Box, error) {
	data, err := os.ReadFile(s.path(agentID))
	if err != nil {
		if os.IsNotExist(err) {
			return &Box{}, nil
		}
		return nil, err
	}
	var box Box
	if err := json.Unmarshal(data, &box); err != nil {
		return nil, err
	}
	return &box, nil
}

func (s *Store) path(agentID string) string {
	return filepath.Join(s.dir, agentID+".json")
}

func atomicWriteJSON(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*.json")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	ok := false
	defer func() {
		if !ok {
			_ = os.Remove(tmpPath)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	ok = true
	return nil
}
