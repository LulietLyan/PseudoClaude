package permission

type Mode string

const (
	ModeStrict            Mode = "strict"
	ModeDefault           Mode = "default"
	ModeAcceptEdits       Mode = "acceptEdits"
	ModeBypassPermissions Mode = "bypassPermissions"
)

type Decision string

const (
	DecisionAllow Decision = "allow"
	DecisionDeny  Decision = "deny"
	DecisionAsk   Decision = "ask"
)

type Category string

const (
	CategoryRead  Category = "read"
	CategoryWrite Category = "write"
	CategoryExec  Category = "exec"
)

type ApprovalDecision string

const (
	ApprovalAllowOnce    ApprovalDecision = "allow_once"
	ApprovalAllowSession ApprovalDecision = "allow_session"
	ApprovalAllowForever ApprovalDecision = "allow_forever"
	ApprovalDenyOnce     ApprovalDecision = "deny_once"
)

func ParseMode(value string) Mode {
	switch Mode(value) {
	case ModeStrict, ModeDefault, ModeAcceptEdits, ModeBypassPermissions:
		return Mode(value)
	default:
		return ModeDefault
	}
}

func (m Mode) String() string {
	switch m {
	case ModeStrict, ModeDefault, ModeAcceptEdits, ModeBypassPermissions:
		return string(m)
	default:
		return string(ModeDefault)
	}
}

func NextMode(mode Mode) Mode {
	switch mode {
	case ModeStrict:
		return ModeDefault
	case ModeDefault:
		return ModeAcceptEdits
	case ModeAcceptEdits:
		return ModeBypassPermissions
	default:
		return ModeStrict
	}
}

func modeFallback(mode Mode, category Category) Decision {
	switch ParseMode(mode.String()) {
	case ModeStrict:
		return DecisionAsk
	case ModeDefault:
		if category == CategoryRead {
			return DecisionAllow
		}
		return DecisionAsk
	case ModeAcceptEdits:
		if category == CategoryRead || category == CategoryWrite {
			return DecisionAllow
		}
		return DecisionAsk
	case ModeBypassPermissions:
		return DecisionAllow
	default:
		return DecisionAsk
	}
}
