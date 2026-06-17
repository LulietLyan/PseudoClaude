package compact

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"PseudoClaude/internal/llm"
)

type Runtime struct {
	mu sync.Mutex

	session       Session
	replacements  ReplacementLedger
	autoFailures  int
	usageAnchor   UsageAnchor
	contextWindow int64
}

type Session struct {
	ID       string
	RootDir  string
	SpillDir string
}

type RuntimeSnapshot struct {
	Session       Session
	AutoFailures  int
	UsageAnchor   UsageAnchor
	ContextWindow int64
}

type UsageAnchor struct {
	Tokens       int64
	MessageCount int
}

type ReplacementDecision string

const (
	DecisionKeep    ReplacementDecision = "keep"
	DecisionReplace ReplacementDecision = "replace"
)

type ReplacementLedger struct {
	seen         map[string]ReplacementDecision
	replacements map[string]string
}

func NewRuntime(workspace string, contextWindow int64) (*Runtime, error) {
	id := newSessionID()
	root := filepath.Join(workspace, ".PseudoClaude", "sessions", id)
	spill := filepath.Join(root, "tool-results")
	if err := os.MkdirAll(spill, 0o755); err != nil {
		return nil, err
	}
	return &Runtime{
		session:       Session{ID: id, RootDir: root, SpillDir: spill},
		replacements:  ReplacementLedger{seen: make(map[string]ReplacementDecision), replacements: make(map[string]string)},
		contextWindow: contextWindow,
	}, nil
}

func newSessionID() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d-%d", time.Now().Unix(), time.Now().UnixNano())
	}
	return fmt.Sprintf("%d-%s", time.Now().Unix(), hex.EncodeToString(b[:]))
}

func (r *Runtime) SetContextWindow(tokens int64) {
	if r == nil || tokens <= 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.contextWindow = tokens
}

func (r *Runtime) Snapshot() RuntimeSnapshot {
	if r == nil {
		return RuntimeSnapshot{}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return RuntimeSnapshot{
		Session:       r.session,
		AutoFailures:  r.autoFailures,
		UsageAnchor:   r.usageAnchor,
		ContextWindow: r.contextWindow,
	}
}

func (r *Runtime) UpdateUsageAnchor(usage *llm.Usage, messageCount int) {
	if r == nil {
		return
	}
	tokens, ok := UsageTokens(usage)
	if !ok {
		return
	}
	r.ResetUsageAnchor(tokens, messageCount)
}

func (r *Runtime) ResetUsageAnchor(tokens int64, messageCount int) {
	if r == nil {
		return
	}
	if tokens < 0 {
		tokens = 0
	}
	if messageCount < 0 {
		messageCount = 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.usageAnchor = UsageAnchor{Tokens: tokens, MessageCount: messageCount}
}

func (r *Runtime) RecordAutoSuccess() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.autoFailures = 0
}

func (r *Runtime) RecordAutoFailure() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.autoFailures++
}

func (r *Runtime) AutoTripped() bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.autoFailures >= AutoFailureLimit
}

func (l *ReplacementLedger) Existing(id string) (ReplacementDecision, string, bool) {
	if l == nil || l.seen == nil {
		return "", "", false
	}
	decision, ok := l.seen[id]
	if !ok {
		return "", "", false
	}
	return decision, l.replacements[id], true
}

func (l *ReplacementLedger) MarkKeep(id string) {
	if l.seen == nil {
		l.seen = make(map[string]ReplacementDecision)
	}
	l.seen[id] = DecisionKeep
}

func (l *ReplacementLedger) MarkReplace(id string, preview string) {
	if l.seen == nil {
		l.seen = make(map[string]ReplacementDecision)
	}
	if l.replacements == nil {
		l.replacements = make(map[string]string)
	}
	l.seen[id] = DecisionReplace
	l.replacements[id] = preview
}
