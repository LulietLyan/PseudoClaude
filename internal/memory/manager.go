package memory

import (
	"context"
	"log"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"PseudoClaude/internal/llm"
)

type Manager struct {
	project  *Store
	user     *Store
	provider llm.Provider
	mu       sync.Mutex
	updateMu sync.Mutex
	index    string
}

func NewManager(projectDir, userDir string) *Manager {
	return &Manager{
		project: NewStore(LevelProject, projectDir),
		user:    NewStore(LevelUser, userDir),
	}
}

func DefaultProjectDir(workspace string) string {
	return filepath.Join(workspace, ".PseudoClaude", "memory")
}

func DefaultUserDir(home string) string {
	return filepath.Join(home, ".PseudoClaude", "memory")
}

func (m *Manager) SetProvider(provider llm.Provider) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.provider = provider
}

func (m *Manager) RefreshIndex() {
	if m == nil {
		return
	}
	project := m.project.LoadIndex()
	user := m.user.LoadIndex()
	var parts []string
	if strings.TrimSpace(project) != "" {
		parts = append(parts, "## Project Memory\n"+strings.TrimSpace(project))
	}
	if strings.TrimSpace(user) != "" {
		parts = append(parts, "## User Memory\n"+strings.TrimSpace(user))
	}
	index := trimIndex(strings.Join(parts, "\n\n"))
	m.mu.Lock()
	m.index = index
	m.mu.Unlock()
}

func (m *Manager) IndexText() string {
	if m == nil {
		return ""
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.index
}

func (m *Manager) RefreshAndIndexText() string {
	if m == nil {
		return ""
	}
	m.RefreshIndex()
	return m.IndexText()
}

func (m *Manager) UpdateAsync(ctx context.Context, input UpdateInput) {
	if m == nil {
		return
	}
	m.mu.Lock()
	provider := m.provider
	projectIndex := m.project.LoadIndex()
	userIndex := m.user.LoadIndex()
	m.mu.Unlock()
	if provider == nil {
		return
	}
	go func() {
		m.updateMu.Lock()
		defer m.updateMu.Unlock()
		ops, err := collectJSONOperations(ctx, provider, BuildUpdatePrompt(input.Messages, projectIndex, userIndex))
		if err != nil {
			log.Printf("memory update failed: %v", err)
			return
		}
		if len(ops) == 0 {
			return
		}
		var projectOps, userOps []Operation
		for _, op := range ops {
			if err := ValidateOperation(op); err != nil {
				log.Printf("invalid memory operation ignored: %v", err)
				continue
			}
			switch op.Level {
			case LevelProject:
				projectOps = append(projectOps, op)
			case LevelUser:
				userOps = append(userOps, op)
			}
		}
		now := time.Now()
		if err := m.project.Apply(projectOps, now); err != nil {
			log.Printf("project memory update failed: %v", err)
			return
		}
		if err := m.user.Apply(userOps, now); err != nil {
			log.Printf("user memory update failed: %v", err)
			return
		}
		project := m.project.LoadIndex()
		user := m.user.LoadIndex()
		m.mu.Lock()
		m.index = trimIndex(strings.TrimSpace("## Project Memory\n"+project) + "\n\n" + strings.TrimSpace("## User Memory\n"+user))
		m.mu.Unlock()
	}()
}

func trimIndex(index string) string {
	index = strings.TrimSpace(index)
	if index == "" {
		return ""
	}
	lines := strings.Split(index, "\n")
	truncated := false
	if len(lines) > MaxIndexLines {
		lines = lines[:MaxIndexLines]
		truncated = true
	}
	out := strings.Join(lines, "\n")
	if len([]byte(out)) > MaxIndexBytes {
		out = string([]byte(out)[:MaxIndexBytes])
		truncated = true
	}
	if truncated {
		out = strings.TrimSpace(out) + "\n(index truncated)"
	}
	return out
}
