package skills

import (
	"archive/zip"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestInstallLocalZip(t *testing.T) {
	zipPath := makeZip(t, map[string]string{"myskill/SKILL.md": "x"})
	result, err := Install(context.Background(), zipPath, filepath.Join(t.TempDir(), "skills"))
	if err != nil {
		t.Fatal(err)
	}
	if result.Name != "myskill" {
		t.Fatalf("result = %+v", result)
	}
	if _, err := os.Stat(filepath.Join(result.Path, "SKILL.md")); err != nil {
		t.Fatal(err)
	}
}

func TestInstallRejectsZipSlip(t *testing.T) {
	zipPath := makeZip(t, map[string]string{"myskill/../../evil": "x"})
	if _, err := Install(context.Background(), zipPath, filepath.Join(t.TempDir(), "skills")); err == nil {
		t.Fatal("expected error")
	}
}

func TestInstallRejectsMultipleTopDirs(t *testing.T) {
	zipPath := makeZip(t, map[string]string{"one/SKILL.md": "x", "two/SKILL.md": "x"})
	if _, err := Install(context.Background(), zipPath, filepath.Join(t.TempDir(), "skills")); err == nil {
		t.Fatal("expected error")
	}
}

func TestInstallHTTPZip(t *testing.T) {
	zipPath := makeZip(t, map[string]string{"remote/SKILL.md": "x"})
	data, err := os.ReadFile(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(data)
	}))
	defer server.Close()
	result, err := Install(context.Background(), server.URL, filepath.Join(t.TempDir(), "skills"))
	if err != nil {
		t.Fatal(err)
	}
	if result.Name != "remote" {
		t.Fatalf("result = %+v", result)
	}
}

func makeZip(t *testing.T, files map[string]string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "skill.zip")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}
