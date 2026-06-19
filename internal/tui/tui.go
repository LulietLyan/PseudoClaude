package tui

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"PseudoClaude/internal/agent"
	"PseudoClaude/internal/command"
	"PseudoClaude/internal/compact"
	"PseudoClaude/internal/config"
	"PseudoClaude/internal/conversation"
	"PseudoClaude/internal/llm"
	"PseudoClaude/internal/memory"
	"PseudoClaude/internal/permission"
	"PseudoClaude/internal/session"
	"PseudoClaude/internal/tools"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/glamour/v2"
	"charm.land/lipgloss/v2"
)

const Version = "0.1.0"

type sessionState int

const (
	stateSelecting sessionState = iota
	stateIdle
	stateStreaming
	stateApproving
	stateResuming
)

type Model struct {
	state            sessionState
	textarea         textarea.Model
	spinner          spinner.Model
	list             list.Model
	viewport         viewport.Model
	renderer         *glamour.TermRenderer
	providers        []config.ProviderConfig
	provider         llm.Provider
	conv             *conversation.Conversation
	runner           agent.Runner
	compactRuntime   *compact.Runtime
	sessionCtx       session.Context
	sessionWriter    *session.Writer
	instructions     string
	memory           *memory.Manager
	events           <-chan agent.Event
	cancel           context.CancelFunc
	curReply         string
	curTool          *toolStatus
	transcript       []transcriptEntry
	progress         string
	usage            *llm.Usage
	lastStop         *agent.Stop
	lastPlan         *planState
	planMode         bool
	permissionMode   permission.Mode
	permissionEngine *permission.Engine
	pendingApproval  *agent.ApprovalRequest
	approvalCursor   int
	resumeChoices    []resumeChoice
	resumeCursor     int
	showBanner       bool
	turnStart        time.Time
	elapsed          time.Duration
	width            int
	height           int
	initErr          error
	cwd              string
	startupStatus    []string
	registry         *tools.Registry
	commandRegistry  *command.Registry
	completion       completionState
	toolEnv          tools.Env
	stickToBottom    bool
}

type toolStatus struct {
	call    llm.ToolCall
	started time.Time
	result  *tools.Result
}

type planState struct {
	task string
	text string
}

type transcriptKind int

const (
	transcriptUser transcriptKind = iota
	transcriptAssistant
	transcriptTool
	transcriptStatus
	transcriptError
	transcriptHelp
	transcriptStop
)

type transcriptEntry struct {
	kind    transcriptKind
	text    string
	elapsed time.Duration
	result  tools.Result
	stop    agent.Stop
}

func New(providers []config.ProviderConfig, cwd string, registry *tools.Registry, engine ...*permission.Engine) Model {
	ta := textarea.New()
	ta.Prompt = "❯ "
	ta.Placeholder = "Send a message..."
	ta.ShowLineNumbers = false
	ta.DynamicHeight = true
	ta.MinHeight = 1
	ta.MaxHeight = 6
	ta.SetWidth(80)
	ta.Focus()

	sp := spinner.New(spinner.WithSpinner(spinner.Points))
	vp := viewport.New(viewport.WithWidth(80), viewport.WithHeight(10))
	vp.SoftWrap = true
	vp.FillHeight = true
	vp.MouseWheelEnabled = true
	vp.KeyMap.PageDown = key.NewBinding(key.WithKeys("pgdown"))
	vp.KeyMap.PageUp = key.NewBinding(key.WithKeys("pgup"))
	vp.KeyMap.HalfPageDown = key.NewBinding(key.WithKeys("ctrl+d"))
	vp.KeyMap.HalfPageUp = key.NewBinding(key.WithKeys("ctrl+u"))
	vp.KeyMap.Down = key.NewBinding()
	vp.KeyMap.Up = key.NewBinding()
	vp.KeyMap.Left = key.NewBinding()
	vp.KeyMap.Right = key.NewBinding()
	renderer, err := glamour.NewTermRenderer(glamour.WithWordWrap(80))

	if registry == nil {
		var err error
		registry, err = tools.NewRegistry()
		if err != nil {
			// NewRegistry without tools cannot fail, but keep construction defensive.
			registry = &tools.Registry{}
		}
	}
	var permissionEngine *permission.Engine
	if len(engine) > 0 {
		permissionEngine = engine[0]
	}
	permissionMode := permission.ModeDefault
	if permissionEngine != nil {
		permissionMode = permissionEngine.StartMode()
	}

	m := Model{
		state:            stateIdle,
		textarea:         ta,
		spinner:          sp,
		viewport:         vp,
		renderer:         renderer,
		providers:        providers,
		width:            80,
		height:           24,
		initErr:          err,
		cwd:              cwd,
		registry:         registry,
		commandRegistry:  command.NewBuiltinRegistry(),
		toolEnv:          tools.DefaultEnv(cwd),
		permissionMode:   permissionMode,
		permissionEngine: permissionEngine,
		showBanner:       true,
		stickToBottom:    true,
	}
	runtime, compactErr := compact.NewRuntime(cwd, 0)
	if compactErr != nil {
		m.appendTranscript(transcriptEntry{kind: transcriptError, text: "compact runtime 初始化失败: " + compactErr.Error()})
	} else {
		m.compactRuntime = runtime
		snapshot := runtime.Snapshot()
		m.sessionCtx = session.Context{
			ID:        snapshot.Session.ID,
			Dir:       snapshot.Session.RootDir,
			JSONLPath: filepath.Join(snapshot.Session.RootDir, session.ConversationFileName),
			SpillDir:  snapshot.Session.SpillDir,
		}
		writer, err := session.NewWriter(m.sessionCtx, "", func(err error) {
			m.appendTranscript(transcriptEntry{kind: transcriptError, text: "会话写入失败: " + err.Error()})
		})
		if err != nil {
			m.appendTranscript(transcriptEntry{kind: transcriptError, text: "会话写入器初始化失败: " + err.Error()})
			m.conv = &conversation.Conversation{}
		} else {
			m.sessionWriter = writer
			m.conv = conversation.New(writer.Hooks())
		}
	}
	if m.conv == nil {
		m.conv = &conversation.Conversation{}
	}
	m.runner = agent.Runner{Registry: registry, Env: m.toolEnv, Config: agent.DefaultConfig(), Version: Version, Permission: permissionEngine, Compact: runtime}

	if len(providers) > 1 {
		m.state = stateSelecting
		m.list = newProviderList(providers, 80, 12)
	} else if len(providers) == 1 {
		provider, err := llm.New(providers[0])
		if err != nil {
			m.initErr = err
		}
		m.provider = provider
		m.runner.Provider = provider
		if m.sessionWriter != nil {
			_ = m.sessionWriter.Close()
			writer, err := session.OpenWriter(m.sessionCtx, provider.Model(), func(err error) {
				m.appendTranscript(transcriptEntry{kind: transcriptError, text: "会话写入失败: " + err.Error()})
			})
			if err == nil {
				m.sessionWriter = writer
				m.conv.SetHooks(writer.Hooks())
			}
		}
		if m.compactRuntime != nil {
			m.compactRuntime.SetContextWindow(providers[0].EffectiveContextWindow())
		}
	}

	return m
}

func (m Model) WithPersistentContext(instructions string, memoryManager *memory.Manager) Model {
	m.instructions = instructions
	m.memory = memoryManager
	m.runner.Instructions = instructions
	m.runner.Memory = memoryManager
	if memoryManager != nil && m.provider != nil {
		memoryManager.SetProvider(m.provider)
	}
	return m
}

func (m Model) WithStartupStatus(messages ...string) Model {
	for _, message := range messages {
		if strings.TrimSpace(message) != "" {
			m.startupStatus = append(m.startupStatus, message)
		}
	}
	return m
}

func (m Model) Init() tea.Cmd {
	if m.state == stateSelecting {
		return m.list.StartSpinner()
	}
	return m.textarea.Focus()
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.initErr != nil {
		return m, tea.Quit
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m.handleResize(msg), nil
	case tea.MouseWheelMsg:
		m = m.syncViewport()
		return m.updateViewport(msg)
	case tea.KeyPressMsg:
		m = m.syncViewport()
		if next, ok := m.handleScrollKey(msg); ok {
			return next, nil
		}
		if msg.String() == "ctrl+c" {
			if m.state == stateApproving {
				m.stopStream()
				next, cmd := m.finishApproval(permission.ApprovalDenyOnce)
				return next, cmd
			}
			m.stopStream()
			return m, tea.Quit
		}
	}

	switch m.state {
	case stateSelecting:
		return m.updateSelecting(msg)
	case stateStreaming:
		return m.updateStreaming(msg)
	case stateApproving:
		return m.updateApproving(msg)
	case stateResuming:
		return m.updateResuming(msg)
	default:
		return m.updateIdle(msg)
	}
}

func (m Model) View() tea.View {
	view := tea.NewView(m.view())
	view.MouseMode = tea.MouseModeCellMotion
	return view
}

func (m Model) Run() error {
	defer func() {
		if m.sessionWriter != nil {
			_ = m.sessionWriter.Close()
		}
	}()
	_, err := tea.NewProgram(m).Run()
	return err
}

func (m Model) handleResize(msg tea.WindowSizeMsg) Model {
	m.width = msg.Width
	m.height = msg.Height
	contentWidth := m.contentWidth()
	m.textarea.SetWidth(contentWidth)
	m = m.syncViewport()
	if m.state == stateSelecting {
		m.list.SetSize(contentWidth, max(8, msg.Height-8))
	}
	if m.state == stateResuming {
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

func (m Model) handleScrollKey(msg tea.KeyPressMsg) (Model, bool) {
	switch msg.String() {
	case "ctrl+t":
		m.stickToBottom = false
		m.viewport.GotoTop()
		return m, true
	case "ctrl+b":
		m.stickToBottom = true
		m.viewport.GotoBottom()
		return m, true
	}
	switch msg.Code {
	case tea.KeyPgUp:
		m.stickToBottom = false
		m.viewport.PageUp()
		return m, true
	case tea.KeyPgDown:
		m.viewport.PageDown()
		m.stickToBottom = m.viewport.AtBottom()
		return m, true
	case tea.KeyHome:
		m.stickToBottom = false
		m.viewport.GotoTop()
		return m, true
	case tea.KeyEnd:
		m.stickToBottom = true
		m.viewport.GotoBottom()
		return m, true
	default:
		switch msg.String() {
		case "ctrl+u":
			m.stickToBottom = false
			m.viewport.HalfPageUp()
			return m, true
		case "ctrl+d":
			m.viewport.HalfPageDown()
			m.stickToBottom = m.viewport.AtBottom()
			return m, true
		}
		return m, false
	}
}

func (m Model) updateViewport(msg tea.Msg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	m.stickToBottom = m.viewport.AtBottom()
	return m, cmd
}

func (m Model) syncViewport() Model {
	m.viewport.SetWidth(max(20, m.width))
	columnWidth := m.contentWidth()
	bottomParts := make([]string, 0, 5)
	if m.state == stateApproving {
		bottomParts = append(bottomParts, m.centeredColumn(m.approvalBlock(columnWidth)))
	}
	bottomParts = append(bottomParts, m.centeredColumn(inputBoxStyle.Width(columnWidth).Render(m.textarea.View())))
	if completion := m.completionView(columnWidth); completion != "" {
		bottomParts = append(bottomParts, m.centeredColumn(completion))
	}
	bottomParts = append(bottomParts, m.centeredColumn(m.statusBar(columnWidth)))
	bottom := strings.Join(bottomParts, "\n")
	if m.height > 0 {
		m.viewport.SetHeight(max(1, m.height-lipgloss.Height(bottom)-1))
	} else {
		m.viewport.SetHeight(10)
	}
	m.updateViewportContent()
	return m
}
