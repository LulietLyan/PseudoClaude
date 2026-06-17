package session

import (
	"os"
	"path/filepath"
	"time"
)

func CleanExpired(workspace string, now time.Time) []error {
	root := filepath.Join(workspace, ".PseudoClaude", SessionsDirName)
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return []error{err}
	}
	var errs []error
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		created, ok := ParseID(entry.Name())
		if !ok {
			continue
		}
		if now.Sub(created) <= ExpiryAge {
			continue
		}
		if err := os.RemoveAll(filepath.Join(root, entry.Name())); err != nil {
			errs = append(errs, err)
		}
	}
	return errs
}
