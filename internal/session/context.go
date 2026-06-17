package session

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

const (
	SessionsDirName      = "sessions"
	ConversationFileName = "conversation.jsonl"
	ToolResultsDirName   = "tool-results"
	SessionIDLayout      = "20060102-150405"
	ExpiryAge            = 30 * 24 * time.Hour
)

var sessionIDPattern = regexp.MustCompile(`^[0-9]{8}-[0-9]{6}-[0-9a-f]{4}$`)

type Context struct {
	ID        string
	Dir       string
	JSONLPath string
	SpillDir  string
}

func NewID(now time.Time) string {
	var b [2]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%s-%04x", now.Format(SessionIDLayout), now.UnixNano()&0xffff)
	}
	return now.Format(SessionIDLayout) + "-" + hex.EncodeToString(b[:])
}

func ParseID(id string) (time.Time, bool) {
	if !sessionIDPattern.MatchString(id) {
		return time.Time{}, false
	}
	t, err := time.ParseInLocation(SessionIDLayout, id[:15], time.Local)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

func NewContext(workspace string, now time.Time) (Context, error) {
	for i := 0; i < 10; i++ {
		ctx := contextForID(workspace, NewID(now))
		if _, err := os.Stat(ctx.Dir); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return Context{}, err
		}
		if err := os.MkdirAll(ctx.SpillDir, 0o755); err != nil {
			return Context{}, err
		}
		return ctx, nil
	}
	return Context{}, fmt.Errorf("could not allocate unique session id")
}

func OpenContext(workspace, id string) (Context, error) {
	if _, ok := ParseID(id); !ok {
		return Context{}, fmt.Errorf("invalid session id: %s", id)
	}
	ctx := contextForID(workspace, id)
	info, err := os.Stat(ctx.Dir)
	if err != nil {
		return Context{}, err
	}
	if !info.IsDir() {
		return Context{}, fmt.Errorf("session path is not a directory: %s", ctx.Dir)
	}
	if err := os.MkdirAll(ctx.SpillDir, 0o755); err != nil {
		return Context{}, err
	}
	return ctx, nil
}

func contextForID(workspace, id string) Context {
	dir := filepath.Join(workspace, ".PseudoClaude", SessionsDirName, id)
	return Context{
		ID:        id,
		Dir:       dir,
		JSONLPath: filepath.Join(dir, ConversationFileName),
		SpillDir:  filepath.Join(dir, ToolResultsDirName),
	}
}
