package subagent

import "strings"

type Source int

const (
	SourcePlugin Source = iota
	SourceBuiltin
	SourceUser
	SourceProject
)

func (s Source) String() string {
	switch s {
	case SourcePlugin:
		return "plugin"
	case SourceBuiltin:
		return "builtin"
	case SourceUser:
		return "user"
	case SourceProject:
		return "project"
	default:
		return "unknown"
	}
}

func (s Source) Priority() int {
	switch s {
	case SourceProject:
		return 40
	case SourceUser:
		return 30
	case SourceBuiltin:
		return 20
	case SourcePlugin:
		return 10
	default:
		return 0
	}
}

type ModelRef string

const (
	ModelInherit ModelRef = "inherit"
	ModelHaiku   ModelRef = "haiku"
	ModelSonnet  ModelRef = "sonnet"
	ModelOpus    ModelRef = "opus"
)

func ParseModelRef(value string) (ModelRef, Warning) {
	switch strings.TrimSpace(value) {
	case "", string(ModelInherit):
		return ModelInherit, Warning{}
	case string(ModelHaiku):
		return ModelHaiku, Warning{}
	case string(ModelSonnet):
		return ModelSonnet, Warning{}
	case string(ModelOpus):
		return ModelOpus, Warning{}
	default:
		return ModelInherit, Warning{
			Field:   "model",
			Message: "unknown model reference, falling back to inherit",
		}
	}
}

type PermissionRef string

const (
	PermissionInherit           PermissionRef = "inherit"
	PermissionStrict            PermissionRef = "strict"
	PermissionDefault           PermissionRef = "default"
	PermissionAcceptEdits       PermissionRef = "acceptEdits"
	PermissionBypassPermissions PermissionRef = "bypassPermissions"
	PermissionDontAsk           PermissionRef = "dontAsk"
	PermissionPlan              PermissionRef = "plan"
)

func ParsePermissionRef(value string) (PermissionRef, Warning) {
	switch strings.TrimSpace(value) {
	case "", string(PermissionInherit):
		return PermissionInherit, Warning{}
	case string(PermissionStrict):
		return PermissionStrict, Warning{}
	case string(PermissionDefault):
		return PermissionDefault, Warning{}
	case string(PermissionAcceptEdits):
		return PermissionAcceptEdits, Warning{}
	case string(PermissionBypassPermissions):
		return PermissionBypassPermissions, Warning{}
	case string(PermissionDontAsk):
		return PermissionDontAsk, Warning{}
	case string(PermissionPlan):
		return PermissionPlan, Warning{}
	default:
		return PermissionInherit, Warning{
			Field:   "permissionMode",
			Message: "unknown permission mode, falling back to inherit",
		}
	}
}

type Isolation string

const (
	IsolationNone     Isolation = ""
	IsolationWorktree Isolation = "worktree"
)

func ParseIsolation(value string) (Isolation, Warning) {
	switch strings.TrimSpace(value) {
	case "":
		return IsolationNone, Warning{}
	case string(IsolationWorktree):
		return IsolationWorktree, Warning{}
	default:
		return IsolationNone, Warning{
			Field:   "isolation",
			Message: "unknown isolation mode, falling back to none",
		}
	}
}

type Definition struct {
	Name            string
	Description     string
	Tools           []string
	DisallowedTools []string
	Model           ModelRef
	MaxTurns        int
	Permission      PermissionRef
	Background      bool
	Isolation       Isolation
	SystemPrompt    string
	Source          Source
	Path            string
	Warnings        []Warning
}
