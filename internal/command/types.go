package command

type Kind int

const (
	KindLocal Kind = iota
	KindUI
	KindPrompt
	KindSkill
)

type WorkMode string

const (
	WorkModeDefault WorkMode = "default"
	WorkModePlan    WorkMode = "plan"
)

type MessageKind string

const (
	MessageInfo  MessageKind = "info"
	MessageError MessageKind = "error"
	MessageHelp  MessageKind = "help"
)

type Command struct {
	Name        string
	Aliases     []string
	Description string
	Usage       string
	Kind        Kind
	ArgHint     string
	Hidden      bool
	Skill       bool
	Handler     Handler
}

type Handler func(ctx Context, ctl Controller) error

type Context struct {
	Input   string
	Name    string
	Args    string
	Command *Command
}

type Usage struct {
	InputTokens  int64
	OutputTokens int64
	TotalTokens  int64
}

type SessionInfo struct {
	ID           string
	JSONLPath    string
	MessageCount int
	Model        string
}

type StatusInfo struct {
	WorkMode       WorkMode
	PermissionMode string
	Model          string
	Usage          Usage
	SessionID      string
	CWD            string
	RuntimeState   string
}

type Controller interface {
	Show(kind MessageKind, text string)
	IsIdle() bool

	WorkMode() WorkMode
	SetWorkMode(mode WorkMode)
	PermissionMode() string
	Status() StatusInfo
	Session() SessionInfo
	MemorySummary() string

	TriggerCompact()
	ClearScreen()
	SendPresetUserMessage(displayLabel, prompt string)
	RefreshStatus()
	ListSkills() []SkillSummary
	ListHooks() []HookSummary
	HookSources() []string
	ListAgents() []AgentSummary
	DescribeAgent(name string) (AgentDetail, bool)
	ReloadAgents()
	RunSkill(name, args string) error
	ReloadSkills()
	ClearActiveSkills()
}

type WorktreeController interface {
	WorktreeAvailable() bool
	CreateWorktree(name string) (WorktreeSummary, error)
	ListWorktrees() []WorktreeSummary
	EnterWorktree(name string) (WorktreeSummary, error)
	ExitWorktree(remove bool, discard bool) (WorktreeSummary, error)
	RemoveWorktree(name string, discard bool) (WorktreeSummary, error)
}

type SkillSummary struct {
	Name        string
	Description string
	Source      string
	Mode        string
}

type HookSummary struct {
	Name   string
	Event  string
	Action string
	Flags  []string
	Source string
}

type AgentSummary struct {
	Name            string
	Description     string
	Source          string
	Model           string
	MaxTurns        int
	Background      bool
	Isolation       string
	Tools           []string
	DisallowedTools []string
}

type AgentDetail struct {
	Active     AgentSummary
	Overridden []AgentSummary
	Prompt     string
}

type WorktreeSummary struct {
	Name       string
	Path       string
	Branch     string
	Manual     bool
	Active     bool
	Dirty      bool
	DirtyError string
	Removed    bool
}
