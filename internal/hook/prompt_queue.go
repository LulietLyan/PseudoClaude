package hook

import (
	"strings"
	"sync"
)

type PromptQueue struct {
	mu    sync.Mutex
	items []string
}

func NewPromptQueue() *PromptQueue {
	return &PromptQueue{}
}

func (q *PromptQueue) Add(items ...string) {
	if q == nil {
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item != "" {
			q.items = append(q.items, item)
		}
	}
}

func (q *PromptQueue) Drain() []string {
	if q == nil {
		return nil
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	out := append([]string(nil), q.items...)
	q.items = nil
	return out
}

func (q *PromptQueue) Clear() {
	if q == nil {
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	q.items = nil
}
