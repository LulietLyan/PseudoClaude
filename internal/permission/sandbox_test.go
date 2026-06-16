package permission

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSandbox(t *testing.T) {
	root := t.TempDir()
	resolvedRoot, err := resolveRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, inside, err := sandboxTarget(resolvedRoot, "src/main.go"); err != nil || !inside {
		t.Fatalf("project path inside=%v err=%v", inside, err)
	}
	if _, inside, err := sandboxTarget(resolvedRoot, filepath.Join(root, "..", "outside.txt")); err != nil || inside {
		t.Fatalf("../outside inside=%v err=%v", inside, err)
	}
	if _, inside, err := sandboxTarget(resolvedRoot, "/etc/passwd"); err != nil || inside {
		t.Fatalf("/etc/passwd inside=%v err=%v", inside, err)
	}
	if _, inside, err := sandboxTarget(resolvedRoot, "new/a/b.txt"); err != nil || !inside {
		t.Fatalf("new path inside=%v err=%v", inside, err)
	}

	outside := t.TempDir()
	link := filepath.Join(root, "link")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, inside, err := sandboxTarget(resolvedRoot, "link/escape.txt"); err != nil || inside {
		t.Fatalf("symlink escape inside=%v err=%v", inside, err)
	}
}
