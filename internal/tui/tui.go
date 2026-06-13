package tui

import (
	"context"
	"strings"
	"time"

	"PseudoClaude/internal/config"
	"PseudoClaude/internal/conversation"
	"PseudoClaude/internal/llm"
	"PseudoClaude/internal/prompt"

	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/glamour/v2"
)

const Version = "0.1.0"

type sessionState int

const (
	stateSelecting sessionState = iota
	stateIdle
	stateStreaming
)

type Model struct {
	state     sessionState
	textarea  textarea.Model
	spinner   spinner.Model
	list      list.Model
	renderer  *glamour.TermRenderer
	providers []config.ProviderConfig
	provider  llm.Provider
	conv      *conversation.Conversation
	events    <-chan llm.StreamEvent
	cancel    context.CancelFunc
	curReply  strings.Builder
	turnStart time.Time
	elapsed   time.Duration
	width     int
	height    int
	initErr   error
	cwd       string
}

func New(providers []config.ProviderConfig, cwd string) Model {
	ta := textarea.New()
	ta.Prompt = "❯ "
	ta.Placeholder = "Send a message..."
	ta.ShowLineNumbers = false
	ta.DynamicHeight = true
	ta.MinHeight = 1
	ta.MaxHeight = 6
	ta.SetWidth(80)

	sp := spinner.New(spinner.WithSpinner(spinner.Points))
	renderer, err := glamour.NewTermRenderer(glamour.WithWordWrap(80))

	m := Model{
		state:     stateIdle,
		textarea:  ta,
		spinner:   sp,
		renderer:  renderer,
		providers: providers,
		conv:      &conversation.Conversation{},
		width:     80,
		height:    24,
		initErr:   err,
		cwd:       cwd,
	}

	if len(providers) > 1 {
		m.state = stateSelecting
		m.list = newProviderList(providers, 80, 12)
	} else if len(providers) == 1 {
		provider, err := llm.New(providers[0])
		if err != nil {
			m.initErr = err
		}
		m.provider = provider
	}

	return m
}

func (m Model) Init() tea.Cmd {
	if m.state == stateSelecting {
		return tea.Batch(m.list.StartSpinner(), tea.Println(prompt.RenderBanner(Version, m.cwd)))
	}
	return tea.Batch(m.textarea.Focus(), tea.Println(prompt.RenderBanner(Version, m.cwd)))
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.initErr != nil {
		return m, tea.Quit
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m.handleResize(msg), nil
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			m.stopStream()
			return m, tea.Quit
		}
	}

	switch m.state {
	case stateSelecting:
		return m.updateSelecting(msg)
	case stateStreaming:
		return m.updateStreaming(msg)
	default:
		return m.updateIdle(msg)
	}
}

func (m Model) View() tea.View {
	return tea.NewView(m.view())
}

func (m Model) Run() error {
	_, err := tea.NewProgram(m).Run()
	return err
}

func (m Model) handleResize(msg tea.WindowSizeMsg) Model {
	m.width = msg.Width
	m.height = msg.Height
	contentWidth := max(20, msg.Width-4)
	m.textarea.SetWidth(contentWidth)
	if m.state == stateSelecting {
		m.list.SetSize(contentWidth, max(8, msg.Height-8))
	}
	renderer, err := glamour.NewTermRenderer(glamour.WithWordWrap(contentWidth))
	if err == nil {
		m.renderer = renderer
	}
	return m
}

func (m *Model) stopStream() {
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
}
