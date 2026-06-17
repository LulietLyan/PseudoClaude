package instructions

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var includeLine = regexp.MustCompile(`^\s*@include\s+(.+?)\s*$`)

type expander struct {
	maxDepth int
}

func (e expander) expand(path, boundary string, depth int, visited map[string]struct{}) (string, []string) {
	if depth > e.maxDepth {
		w := fmt.Sprintf("<!-- @include 超过最大嵌套深度，已跳过: %s -->", path)
		return w, []string{w}
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		w := fmt.Sprintf("<!-- @include 路径错误，已跳过: %s -->", path)
		return w, []string{w}
	}
	if !insideBoundary(abs, boundary) {
		w := fmt.Sprintf("<!-- @include 路径越界，已跳过: %s -->", path)
		return w, []string{w}
	}
	if _, ok := visited[abs]; ok {
		w := fmt.Sprintf("<!-- @include 检测到环路，已跳过: %s -->", path)
		return w, []string{w}
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		w := fmt.Sprintf("<!-- @include 不可读取，已跳过: %s -->", path)
		return w, []string{w}
	}
	if looksBinary(data) {
		w := fmt.Sprintf("<!-- @include 疑似二进制文件，已跳过: %s -->", path)
		return w, []string{w}
	}

	nextVisited := make(map[string]struct{}, len(visited)+1)
	for k, v := range visited {
		nextVisited[k] = v
	}
	nextVisited[abs] = struct{}{}

	var warnings []string
	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		rel, ok := isIncludeLine(line)
		if !ok {
			continue
		}
		includePath := filepath.Join(filepath.Dir(abs), rel)
		expanded, ws := e.expand(includePath, boundary, depth+1, nextVisited)
		lines[i] = expanded
		warnings = append(warnings, ws...)
	}
	return strings.Join(lines, "\n"), warnings
}

func isIncludeLine(line string) (string, bool) {
	m := includeLine.FindStringSubmatch(line)
	if len(m) != 2 {
		return "", false
	}
	rel := strings.TrimSpace(m[1])
	if rel == "" || filepath.IsAbs(rel) {
		return "", false
	}
	return rel, true
}

func insideBoundary(path, boundary string) bool {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	absBoundary, err := filepath.Abs(boundary)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(absBoundary, absPath)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel))
}

func looksBinary(sample []byte) bool {
	if len(sample) > 512 {
		sample = sample[:512]
	}
	return bytes.Contains(sample, []byte{0})
}
