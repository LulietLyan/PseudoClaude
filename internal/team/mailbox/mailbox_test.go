package mailbox

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestWriteReadUnreadMarkRead(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Write(context.Background(), "agent-a", Message{From: "lead", To: "agent-a", Summary: "hello", Content: "body"}); err != nil {
		t.Fatal(err)
	}
	msgs, err := store.Read("agent-a")
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 || msgs[0].Timestamp.IsZero() || msgs[0].Read {
		t.Fatalf("messages = %#v", msgs)
	}
	unread, err := store.ReadUnread("agent-a")
	if err != nil {
		t.Fatal(err)
	}
	if len(unread) != 1 || unread[0].Index != 0 {
		t.Fatalf("unread = %#v", unread)
	}
	if err := store.MarkRead(context.Background(), "agent-a", []int{0}); err != nil {
		t.Fatal(err)
	}
	unread, err = store.ReadUnread("agent-a")
	if err != nil {
		t.Fatal(err)
	}
	if len(unread) != 0 {
		t.Fatalf("unread after mark = %#v", unread)
	}
}

func TestConcurrentWrites(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if err := store.Write(context.Background(), "agent-a", Message{From: "lead", To: "agent-a", Summary: "msg"}); err != nil {
				t.Errorf("write %d: %v", i, err)
			}
		}(i)
	}
	wg.Wait()
	msgs, err := store.Read("agent-a")
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 10 {
		t.Fatalf("message count = %d, want 10", len(msgs))
	}
	data, err := os.ReadFile(filepath.Join(store.dir, "agent-a.json"))
	if err != nil {
		t.Fatal(err)
	}
	var box Box
	if err := json.Unmarshal(data, &box); err != nil {
		t.Fatalf("mailbox JSON invalid: %v", err)
	}
}

func TestStaleLockIsCleared(t *testing.T) {
	dir := t.TempDir()
	store, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(dir, "agent-a.json.lock")
	if err := os.WriteFile(lockPath, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Minute)
	if err := os.Chtimes(lockPath, old, old); err != nil {
		t.Fatal(err)
	}
	if err := store.Write(context.Background(), "agent-a", Message{From: "lead", To: "agent-a", Summary: "after stale"}); err != nil {
		t.Fatal(err)
	}
}
