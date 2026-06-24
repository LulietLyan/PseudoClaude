package worktree

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

type Manager struct {
	repoRoot    string
	metaDir     string
	worktreeDir string
	sessionFile string
	symlinkDirs []string
	logf        func(format string, args ...any)
	git         GitRunner
	idSource    func() string
	now         func() time.Time

	mu       sync.Mutex
	active   map[string]*Worktree
	creating map[string]struct{}
	current  *Session
}

func NewManager(opts Options) (*Manager, error) {
	ctx := context.Background()
	git := opts.Git
	if git == nil {
		git = execGitRunner{}
	}
	root, err := ensureRepoRoot(ctx, git, opts.RepoRoot)
	if err != nil {
		return nil, fmt.Errorf("%w: git repository not available: %v", ErrUnavailable, err)
	}
	meta := opts.MetaDirName
	if meta == "" {
		meta = DefaultMetaDirName
	}
	logf := opts.Logf
	if logf == nil {
		logf = func(string, ...any) {}
	}
	symlinkDirs := append([]string(nil), opts.SymlinkDirs...)
	if len(symlinkDirs) == 0 {
		symlinkDirs = append([]string(nil), DefaultSymlinkDirs...)
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	m := &Manager{
		repoRoot:    root,
		metaDir:     filepath.Join(root, meta),
		worktreeDir: filepath.Join(root, meta, DefaultWorktreeDirName),
		sessionFile: filepath.Join(root, meta, DefaultSessionFileName),
		symlinkDirs: symlinkDirs,
		logf:        logf,
		git:         git,
		idSource:    opts.IDSource,
		now:         now,
		active:      map[string]*Worktree{},
		creating:    map[string]struct{}{},
	}
	if err := os.MkdirAll(m.worktreeDir, 0o755); err != nil {
		return nil, err
	}
	m.warnIfNotIgnored()
	m.loadCurrent()
	m.scanActive()
	return m, nil
}

func (m *Manager) RepoRoot() string {
	if m == nil {
		return ""
	}
	return m.repoRoot
}

func (m *Manager) WorktreeDir() string {
	if m == nil {
		return ""
	}
	return m.worktreeDir
}

func (m *Manager) List(ctx context.Context) []Summary {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	items := make([]*Worktree, 0, len(m.active))
	for _, wt := range m.active {
		cp := *wt
		items = append(items, &cp)
	}
	current := m.current
	m.mu.Unlock()
	out := make([]Summary, 0, len(items))
	for _, wt := range items {
		protected, reason := hasProtectedChanges(ctx, m.git, wt)
		out = append(out, Summary{
			Name:       wt.Name,
			Path:       wt.Path,
			Branch:     wt.Branch,
			Manual:     wt.Manual,
			Active:     current != nil && current.WorktreeName == wt.Name,
			Dirty:      protected,
			DirtyError: reason,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (m *Manager) CurrentSession() *Session {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.current == nil {
		return nil
	}
	cp := *m.current
	return &cp
}

func (m *Manager) EffectiveCWD(fallback string) string {
	if m == nil {
		return fallback
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.current != nil && m.current.WorktreePath != "" {
		return m.current.WorktreePath
	}
	if fallback != "" {
		return fallback
	}
	return m.repoRoot
}

func (m *Manager) loadCurrent() {
	s, err := loadSession(m.sessionFile)
	if err != nil {
		m.logf("worktree session damaged, clearing: %v", err)
		_ = clearSession(m.sessionFile)
		return
	}
	if s == nil {
		return
	}
	if _, err := os.Stat(s.WorktreePath); err != nil {
		m.logf("worktree session path unavailable, clearing: %v", err)
		_ = clearSession(m.sessionFile)
		return
	}
	m.current = s
}

func (m *Manager) scanActive() {
	entries, err := os.ReadDir(m.worktreeDir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		path := filepath.Join(m.worktreeDir, name)
		wt, err := fastRecover(path)
		if err != nil {
			continue
		}
		wt.FlatName = name
		wt.Name = stringsReplacePlus(name)
		wt.Path = path
		wt.CreatedAt = m.now()
		wt.Manual = !isTemporaryName(wt.Name)
		m.active[wt.Name] = wt
	}
}

func stringsReplacePlus(value string) string {
	out := []rune(value)
	for i, r := range out {
		if r == '+' {
			out[i] = '/'
		}
	}
	return string(out)
}

func (m *Manager) warnIfNotIgnored() {
	for _, path := range []string{filepath.Join(DefaultMetaDirName, DefaultWorktreeDirName) + "/", filepath.Join(DefaultMetaDirName, DefaultSessionFileName)} {
		stdout, _, err := m.git.Run(context.Background(), m.repoRoot, "check-ignore", "-q", path)
		_ = stdout
		if err != nil {
			m.logf("建议在 .gitignore 中忽略 %s", path)
		}
	}
}
