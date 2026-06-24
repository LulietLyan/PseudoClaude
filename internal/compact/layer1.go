package compact

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"

	"PseudoClaude/internal/llm"
)

type Layer1Output struct {
	Messages       []llm.Message
	Changed        bool
	OffloadedCount int
	Err            error
}

type toolCandidate struct {
	index int
	id    string
	size  int
}

var unsafeFileName = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

func OffloadToolResults(messages []llm.Message, rt *Runtime) Layer1Output {
	out := cloneMessages(messages)
	if rt == nil {
		return Layer1Output{Messages: out}
	}

	var firstErr error
	failed := make(map[string]bool)
	for i := range out {
		if out[i].ToolResult == nil {
			continue
		}
		result := out[i].ToolResult
		id := toolResultID(*result, i)
		decision, preview, ok := rt.existingDecision(id)
		if ok {
			if decision == DecisionReplace {
				result.Content = preview
			}
			continue
		}
		if len(result.Content) > SingleToolResultLimitBytes {
			next, err := replaceToolResult(rt, id, result.Content)
			if err != nil {
				firstErr = keepFirst(firstErr, err)
				failed[id] = true
				continue
			}
			result.Content = next
		}
	}

	for _, group := range toolResultGroups(out) {
		total := 0
		var candidates []toolCandidate
		for _, idx := range group {
			result := out[idx].ToolResult
			if result == nil {
				continue
			}
			id := toolResultID(*result, idx)
			decision, _, ok := rt.existingDecision(id)
			if ok && decision == DecisionReplace {
				continue
			}
			size := len(result.Content)
			total += size
			if !ok {
				candidates = append(candidates, toolCandidate{index: idx, id: id, size: size})
			}
		}
		sort.SliceStable(candidates, func(i, j int) bool {
			return candidates[i].size > candidates[j].size
		})
		for _, candidate := range candidates {
			if total <= ToolRoundAggregateLimitBytes {
				break
			}
			result := out[candidate.index].ToolResult
			next, err := replaceToolResult(rt, candidate.id, result.Content)
			if err != nil {
				firstErr = keepFirst(firstErr, err)
				failed[candidate.id] = true
				continue
			}
			result.Content = next
			total -= candidate.size
		}
	}

	changed := false
	offloaded := 0
	for i := range out {
		if out[i].ToolResult == nil {
			continue
		}
		original := ""
		if i < len(messages) && messages[i].ToolResult != nil {
			original = messages[i].ToolResult.Content
		}
		if out[i].ToolResult.Content != original {
			changed = true
			offloaded++
		}
		id := toolResultID(*out[i].ToolResult, i)
		if failed[id] {
			continue
		}
		if _, _, ok := rt.existingDecision(id); !ok {
			rt.markKeep(id)
		}
	}

	return Layer1Output{Messages: out, Changed: changed, OffloadedCount: offloaded, Err: firstErr}
}

func replaceToolResult(rt *Runtime, id, content string) (string, error) {
	path, err := spillToolResult(rt.Snapshot().Session, id, content)
	if err != nil {
		return content, err
	}
	preview := buildPreview(len(content), previewHead(content), path)
	rt.markReplace(id, preview)
	return preview, nil
}

func spillToolResult(session Session, id, content string) (string, error) {
	name := safeFileName(id)
	path := filepath.Join(session.SpillDir, name+".txt")
	if _, err := os.Stat(path); err == nil {
		return path, nil
	} else if !os.IsNotExist(err) {
		return path, err
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return path, err
	}
	return path, nil
}

func safeFileName(id string) string {
	if id == "" {
		return "tool-result"
	}
	name := unsafeFileName.ReplaceAllString(id, "_")
	if name == "" {
		return "tool-result"
	}
	return name
}

func toolResultID(result llm.ToolResult, index int) string {
	if result.CallID != "" {
		return result.CallID
	}
	return fmt.Sprintf("tool-result-%d", index)
}

func toolResultGroups(messages []llm.Message) [][]int {
	var groups [][]int
	for i := 0; i < len(messages); i++ {
		if len(messages[i].ToolCalls) == 0 {
			continue
		}
		ids := make(map[string]bool)
		for _, call := range messages[i].ToolCalls {
			ids[call.ID] = true
		}
		var group []int
		for j := i + 1; j < len(messages); j++ {
			if messages[j].ToolResult == nil {
				break
			}
			if ids[messages[j].ToolResult.CallID] {
				group = append(group, j)
			}
		}
		if len(group) > 0 {
			groups = append(groups, group)
		}
	}
	return groups
}

func keepFirst(existing, next error) error {
	if existing != nil {
		return existing
	}
	return next
}

func (r *Runtime) existingDecision(id string) (ReplacementDecision, string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.replacements.Existing(id)
}

func (r *Runtime) markKeep(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.replacements.MarkKeep(id)
}

func (r *Runtime) markReplace(id string, preview string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.replacements.MarkReplace(id, preview)
}

func cloneMessages(messages []llm.Message) []llm.Message {
	out := make([]llm.Message, len(messages))
	for i, msg := range messages {
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
