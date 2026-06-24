package worktree

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"regexp"
	"time"
)

var temporaryNamePattern = regexp.MustCompile(`^agent-a[0-9a-f]{7}$`)

func RandomAgentName() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("agent-a%07x", time.Now().UnixNano()&0xfffffff)
	}
	n := (uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])) & 0x0fffffff
	return fmt.Sprintf("agent-a%07x", n)
}

func isTemporaryName(name string) bool {
	return temporaryNamePattern.MatchString(name)
}

func (m *Manager) SweepStale(ctx context.Context, cutoff time.Time) []SweepResult {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	items := make([]*Worktree, 0, len(m.active))
	current := ""
	if m.current != nil {
		current = m.current.WorktreeName
	}
	for _, wt := range m.active {
		cp := *wt
		items = append(items, &cp)
	}
	m.mu.Unlock()
	var out []SweepResult
	for _, wt := range items {
		result := SweepResult{Name: wt.Name, Path: wt.Path}
		switch {
		case !isTemporaryName(wt.Name):
			result.Reason = "not a temporary worktree"
		case wt.Name == current:
			result.Reason = "active session"
		default:
			info, err := os.Stat(wt.Path)
			if err != nil {
				result.Err = err
				result.Reason = "stat failed"
				break
			}
			if !info.ModTime().Before(cutoff) {
				result.Reason = "not stale"
				break
			}
			if protected, reason := hasProtectedChanges(ctx, m.git, wt); protected {
				result.Reason = reason
				break
			}
			if _, err := m.Remove(ctx, wt.Name, RemoveOptions{Discard: true}); err != nil {
				result.Err = err
				result.Reason = err.Error()
				break
			}
			result.Removed = true
			result.Reason = "removed"
		}
		out = append(out, result)
	}
	return out
}
