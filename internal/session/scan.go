package session

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

type Info struct {
	ID           string
	Title        string
	Model        string
	MessageCount int
	ModifiedAt   time.Time
	Size         int64
	Dir          string
}

func List(workspace string) ([]Info, error) {
	root := filepath.Join(workspace, ".PseudoClaude", SessionsDirName)
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var infos []Info
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		id := entry.Name()
		if _, ok := ParseID(id); !ok {
			continue
		}
		ctx, err := OpenContext(workspace, id)
		if err != nil {
			continue
		}
		info, ok := scanInfo(ctx)
		if ok {
			infos = append(infos, info)
		}
	}
	sort.SliceStable(infos, func(i, j int) bool {
		return infos[i].ModifiedAt.After(infos[j].ModifiedAt)
	})
	return infos, nil
}

func scanInfo(ctx Context) (Info, bool) {
	stat, err := os.Stat(ctx.JSONLPath)
	if err != nil {
		return Info{}, false
	}
	file, err := os.Open(ctx.JSONLPath)
	if err != nil {
		return Info{}, false
	}
	defer file.Close()

	info := Info{
		ID:         ctx.ID,
		Title:      "Untitled session",
		ModifiedAt: stat.ModTime(),
		Size:       stat.Size(),
		Dir:        ctx.Dir,
	}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	for scanner.Scan() {
		var entry Entry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}
		if entry.Type != EntryMessage && entry.Type != "" {
			continue
		}
		info.MessageCount++
		if info.Model == "" {
			info.Model = entry.Model
		}
		if info.Title == "Untitled session" && entry.Role == "user" && strings.TrimSpace(entry.Content) != "" {
			info.Title = truncateTitle(entry.Content, 80)
		}
	}
	return info, true
}

func truncateTitle(text string, maxRunes int) string {
	title := strings.Join(strings.Fields(text), " ")
	if utf8.RuneCountInString(title) <= maxRunes {
		return title
	}
	runes := []rune(title)
	return string(runes[:maxRunes]) + "..."
}
