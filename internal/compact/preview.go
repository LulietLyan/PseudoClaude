package compact

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

func previewHead(content string) string {
	lines := strings.SplitAfter(content, "\n")
	if len(lines) > PreviewHeadLines {
		lines = lines[:PreviewHeadLines]
	}
	head := strings.Join(lines, "")
	if len(head) <= PreviewHeadBytes {
		return head
	}
	cut := PreviewHeadBytes
	for cut > 0 && !utf8.ValidString(head[:cut]) {
		cut--
	}
	return head[:cut]
}

func buildPreview(originalBytes int, head string, spillPath string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[content offloaded] original size: %d bytes\n", originalBytes)
	fmt.Fprintf(&b, "[saved to] %s\n", spillPath)
	b.WriteString("[head preview]\n")
	b.WriteString(head)
	if head != "" && !strings.HasSuffix(head, "\n") {
		b.WriteByte('\n')
	}
	b.WriteString("\n完整内容已保存到上述路径；如需完整内容，请使用文件读取工具读取该路径。不要凭头部预览猜测全文。")
	return b.String()
}
