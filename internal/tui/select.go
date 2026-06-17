package tui

import (
	"fmt"

	"PseudoClaude/internal/config"
	"PseudoClaude/internal/llm"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
)

type providerItem struct {
	cfg config.ProviderConfig
}

func (i providerItem) Title() string {
	return i.cfg.Name
}

func (i providerItem) Description() string {
	return i.cfg.Model
}

func (i providerItem) FilterValue() string {
	return i.cfg.Name + " " + i.cfg.Model
}

func newProviderList(providers []config.ProviderConfig, width, height int) list.Model {
	items := make([]list.Item, 0, len(providers))
	for _, provider := range providers {
		items = append(items, providerItem{cfg: provider})
	}
	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = true
	l := list.New(items, delegate, width, height)
	l.Title = "Select provider"
	l.SetShowStatusBar(false)
	l.SetShowHelp(true)
	l.SetFilteringEnabled(false)
	return l
}

func (m Model) updateSelecting(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		if msg.String() == "enter" {
			item, ok := m.list.SelectedItem().(providerItem)
			if !ok {
				return m, nil
			}
			provider, err := llm.New(item.cfg)
			if err != nil {
				m.state = stateIdle
				m.appendTranscript(transcriptEntry{kind: transcriptError, text: fmt.Sprintf("provider 初始化失败: %v", err)})
				return m, nil
			}
			m.provider = provider
			m.runner.Provider = provider
			if m.compactRuntime != nil {
				m.compactRuntime.SetContextWindow(item.cfg.EffectiveContextWindow())
			}
			m.state = stateIdle
			m.showBanner = true
			return m, m.textarea.Focus()
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}
