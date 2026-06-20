package skills

import "sync"

type ActiveEntry struct {
	Name string
	Body string
}

type ActiveSkills struct {
	mu      sync.RWMutex
	entries []ActiveEntry
	index   map[string]int
}

func NewActiveSkills() *ActiveSkills {
	return &ActiveSkills{index: make(map[string]int)}
}

func (a *ActiveSkills) Activate(name, body string) {
	if a == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.index == nil {
		a.index = make(map[string]int)
	}
	if i, ok := a.index[name]; ok {
		a.entries[i].Body = body
		return
	}
	a.index[name] = len(a.entries)
	a.entries = append(a.entries, ActiveEntry{Name: name, Body: body})
}

func (a *ActiveSkills) Clear() {
	if a == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.entries = nil
	a.index = make(map[string]int)
}

func (a *ActiveSkills) Snapshot() []ActiveEntry {
	if a == nil {
		return nil
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	return append([]ActiveEntry(nil), a.entries...)
}

func (a *ActiveSkills) Names() []string {
	entries := a.Snapshot()
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		out = append(out, entry.Name)
	}
	return out
}
