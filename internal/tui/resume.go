package tui

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"PseudoClaude/internal/compact"
	"PseudoClaude/internal/conversation"
	"PseudoClaude/internal/llm"
	"PseudoClaude/internal/session"

	tea "charm.land/bubbletea/v2"
)

const staleResumeAfter = 6 * time.Hour
const maxResumeChoices = 9

type resumeChoice struct {
	info session.Info
}

func (m Model) startResume() (tea.Model, tea.Cmd) {
	if m.state != stateIdle {
		m.appendTranscript(transcriptEntry{kind: transcriptError, text: "当前任务完成后才能恢复会话"})
		return m, nil
	}
	infos, err := session.List(m.cwd)
	if err != nil {
		m.textarea.Reset()
		m.appendTranscript(transcriptEntry{kind: transcriptError, text: "读取会话列表失败: " + err.Error()})
		return m, nil
	}
	infos = m.filterResumeInfos(infos)
	if len(infos) == 0 {
		m.textarea.Reset()
		m.appendTranscript(transcriptEntry{kind: transcriptStatus, text: "暂无可恢复的历史会话"})
		return m, nil
	}
	if len(infos) > maxResumeChoices {
		infos = infos[:maxResumeChoices]
	}
	choices := make([]resumeChoice, 0, len(infos))
	for _, info := range infos {
		choices = append(choices, resumeChoice{info: info})
	}
	m.resumeChoices = choices
	m.resumeCursor = 0
	m.state = stateResuming
	m.textarea.Reset()
	return m, nil
}

func (m Model) updateResuming(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "esc", "ctrl+c":
			m.state = stateIdle
			m.resumeChoices = nil
			m.resumeCursor = 0
			m.appendTranscript(transcriptEntry{kind: transcriptStatus, text: "已取消恢复会话"})
			return m, m.textarea.Focus()
		case "up", "k":
			if m.resumeCursor > 0 {
				m.resumeCursor--
			}
			return m, nil
		case "down", "j":
			if m.resumeCursor < len(m.resumeChoices)-1 {
				m.resumeCursor++
			}
			return m, nil
		case "enter":
			if len(m.resumeChoices) == 0 || m.resumeCursor < 0 || m.resumeCursor >= len(m.resumeChoices) {
				return m, nil
			}
			return m.resumeSession(m.resumeChoices[m.resumeCursor].info)
		default:
			if idx, ok := numericChoice(msg.String(), len(m.resumeChoices)); ok {
				m.resumeCursor = idx
				return m.resumeSession(m.resumeChoices[idx].info)
			}
		}
	}
	return m, nil
}

func (m Model) filterResumeInfos(infos []session.Info) []session.Info {
	out := infos[:0]
	for _, info := range infos {
		if info.MessageCount == 0 || info.ID == m.sessionCtx.ID {
			continue
		}
		out = append(out, info)
	}
	return out
}

func (m Model) resumeSession(info session.Info) (tea.Model, tea.Cmd) {
	ctx, err := session.OpenContext(m.cwd, info.ID)
	if err != nil {
		m.state = stateIdle
		m.appendTranscript(transcriptEntry{kind: transcriptError, text: "打开会话失败: " + err.Error()})
		return m, m.textarea.Focus()
	}
	loaded, err := session.Load(ctx)
	if err != nil {
		m.state = stateIdle
		m.appendTranscript(transcriptEntry{kind: transcriptError, text: "恢复会话失败: " + err.Error()})
		return m, m.textarea.Focus()
	}
	messages := loaded.Messages
	if !loaded.LastMessage.IsZero() && time.Since(loaded.LastMessage) > staleResumeAfter {
		messages = append(messages, staleResumeMessage(loaded.LastMessage))
	}

	contextWindow := int64(0)
	if m.compactRuntime != nil {
		contextWindow = m.compactRuntime.Snapshot().ContextWindow
	}
	rt, err := compact.OpenRuntime(m.cwd, info.ID, contextWindow)
	if err != nil {
		m.state = stateIdle
		m.appendTranscript(transcriptEntry{kind: transcriptError, text: "恢复压缩运行时失败: " + err.Error()})
		return m, m.textarea.Focus()
	}
	model := ""
	if m.provider != nil {
		model = m.provider.Model()
	}
	writer, err := session.OpenWriter(ctx, model, func(err error) {
		m.appendTranscript(transcriptEntry{kind: transcriptError, text: "会话写入失败: " + err.Error()})
	})
	if err != nil {
		m.state = stateIdle
		m.appendTranscript(transcriptEntry{kind: transcriptError, text: "打开会话写入器失败: " + err.Error()})
		return m, m.textarea.Focus()
	}
	if m.sessionWriter != nil {
		_ = m.sessionWriter.Close()
	}
	m.sessionCtx = ctx
	m.sessionWriter = writer
	m.compactRuntime = rt
	m.conv = conversation.NewFromMessages(messages, writer.Hooks())
	m.runner.Compact = rt
	m.runner.Memory = m.memory
	m.runner.Instructions = m.instructions
	m.state = stateIdle
	m.resumeChoices = nil
	m.resumeCursor = 0
	m.appendTranscript(transcriptEntry{kind: transcriptStatus, text: fmt.Sprintf("已恢复会话 %s：%d 条消息，跳过 %d 条坏行", info.ID, len(loaded.Messages), loaded.BadLines)})
	return m, m.textarea.Focus()
}

func staleResumeMessage(last time.Time) llm.Message {
	return llm.Message{Role: "user", Content: fmt.Sprintf("会话已暂停超过 6 小时（最后消息时间：%s）。部分上下文可能已过时；需要时请重新读取文件或重新确认外部状态。", last.Format(time.RFC3339))}
}

func relativeTime(t time.Time) string {
	if t.IsZero() {
		return "unknown time"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

func formatBytes(size int64) string {
	switch {
	case size >= 1024*1024:
		return fmt.Sprintf("%.1f MB", float64(size)/(1024*1024))
	case size >= 1024:
		return fmt.Sprintf("%.1f KB", float64(size)/1024)
	default:
		return fmt.Sprintf("%d B", size)
	}
}

func numericChoice(key string, total int) (int, bool) {
	if len(key) != 1 || key[0] < '1' || key[0] > '9' {
		return 0, false
	}
	idx := int(key[0] - '1')
	return idx, idx >= 0 && idx < total
}

func resumeMeta(info session.Info) string {
	parts := []string{
		relativeTime(info.ModifiedAt),
		fmt.Sprintf("%d messages", info.MessageCount),
		formatBytes(info.Size),
	}
	if strings.TrimSpace(info.Model) != "" {
		parts = append(parts, info.Model)
	}
	return strings.Join(parts, " · ")
}

func contextFromSnapshot(snapshot compact.RuntimeSnapshot) session.Context {
	return session.Context{
		ID:        snapshot.Session.ID,
		Dir:       snapshot.Session.RootDir,
		JSONLPath: filepath.Join(snapshot.Session.RootDir, session.ConversationFileName),
		SpillDir:  snapshot.Session.SpillDir,
	}
}
