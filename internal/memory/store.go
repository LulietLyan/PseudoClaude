package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

type Store struct {
	Level Level
	Dir   string
	mu    sync.Mutex
}

var unsafeSlug = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

func NewStore(level Level, dir string) *Store {
	return &Store{Level: level, Dir: dir}
}

func (s *Store) LoadIndex() string {
	if s == nil {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(s.Dir, IndexFileName))
	if err != nil {
		return ""
	}
	return string(data)
}

func (s *Store) Apply(ops []Operation, now time.Time) error {
	if s == nil || len(ops) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return err
	}
	for _, op := range ops {
		if op.Level != "" && op.Level != s.Level {
			continue
		}
		switch op.Action {
		case "create":
			if err := s.create(op, now); err != nil {
				return err
			}
		case "update":
			if err := s.update(op, now); err != nil {
				return err
			}
		case "delete":
			if err := s.delete(op); err != nil {
				return err
			}
		case "", "noop":
			continue
		default:
			return fmt.Errorf("unsupported memory action: %s", op.Action)
		}
	}
	return nil
}

func (s *Store) NotePath(filename string) (string, error) {
	if strings.TrimSpace(filename) == "" {
		return "", fmt.Errorf("empty memory filename")
	}
	if filename != filepath.Base(filename) {
		return "", fmt.Errorf("memory filename must not contain path separators")
	}
	root, err := filepath.Abs(s.Dir)
	if err != nil {
		return "", err
	}
	path, err := filepath.Abs(filepath.Join(root, filename))
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(root, path)
	if err != nil || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return "", fmt.Errorf("memory path escapes store")
	}
	return path, nil
}

func (s *Store) create(op Operation, now time.Time) error {
	filename := string(op.Type) + "_" + safeSlug(firstNonEmpty(op.Slug, op.Title, "note")) + ".md"
	path, err := s.NotePath(filename)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(renderNote(op, now, now)), 0o644); err != nil {
		return err
	}
	return s.upsertIndex(op, filename)
}

func (s *Store) update(op Operation, now time.Time) error {
	path, err := s.NotePath(op.Filename)
	if err != nil {
		return err
	}
	created := now
	if data, err := os.ReadFile(path); err == nil {
		created = parseCreated(data, now)
	}
	if err := os.WriteFile(path, []byte(renderNote(op, created, now)), 0o644); err != nil {
		return err
	}
	return s.upsertIndex(op, op.Filename)
}

func (s *Store) delete(op Operation) error {
	path, err := s.NotePath(op.Filename)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return s.removeIndex(op.Filename)
}

func (s *Store) upsertIndex(op Operation, filename string) error {
	title := strings.TrimSpace(op.Title)
	if title == "" {
		title = filename
	}
	summary := strings.TrimSpace(op.Summary)
	if summary == "" {
		summary = "No summary provided"
	}
	line := fmt.Sprintf("- [%s] %s — %s (%s)", op.Type, title, summary, filename)
	lines := indexLines(s.LoadIndex())
	replaced := false
	for i := range lines {
		if strings.Contains(lines[i], "("+filename+")") {
			lines[i] = line
			replaced = true
			break
		}
	}
	if !replaced {
		lines = append(lines, line)
	}
	return os.WriteFile(filepath.Join(s.Dir, IndexFileName), []byte(strings.Join(lines, "\n")+"\n"), 0o644)
}

func (s *Store) removeIndex(filename string) error {
	lines := indexLines(s.LoadIndex())
	out := lines[:0]
	for _, line := range lines {
		if !strings.Contains(line, "("+filename+")") {
			out = append(out, line)
		}
	}
	return os.WriteFile(filepath.Join(s.Dir, IndexFileName), []byte(strings.Join(out, "\n")+"\n"), 0o644)
}

func renderNote(op Operation, created, updated time.Time) string {
	return fmt.Sprintf("---\ntype: %s\ntitle: %q\ncreated: %s\nupdated: %s\n---\n\n%s\n",
		op.Type, op.Title, created.Format(time.RFC3339), updated.Format(time.RFC3339), strings.TrimSpace(op.Content))
}

func parseCreated(data []byte, fallback time.Time) time.Time {
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "created: ") {
			if t, err := time.Parse(time.RFC3339, strings.TrimSpace(strings.TrimPrefix(line, "created: "))); err == nil {
				return t
			}
		}
	}
	return fallback
}

func indexLines(index string) []string {
	var lines []string
	for _, line := range strings.Split(index, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func safeSlug(text string) string {
	slug := strings.Trim(unsafeSlug.ReplaceAllString(strings.ToLower(strings.TrimSpace(text)), "-"), "-._")
	if slug == "" {
		return "note"
	}
	if len(slug) > 80 {
		slug = slug[:80]
	}
	return slug
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
