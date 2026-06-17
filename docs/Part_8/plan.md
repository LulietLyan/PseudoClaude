# PseudoClaude 项目记忆与会话恢复 Plan

## 架构概览

本阶段在现有 Go 项目上增加三组持久化能力，并对已有主链路做窄幅接入：

| 模块 | 职责 |
| ---- | ---- |
| `internal/instructions` | 加载三层 `PSEUDOCLAUDE.md` 指令文件，展开受控 `@include`，输出可注入系统提示的 Markdown 文本 |
| `internal/session` | 管理 JSONL 会话存档：会话 ID、写入器、扫描列表、恢复解析、过期清理 |
| `internal/memory` | 管理长期记忆：Markdown 笔记、`MEMORY.md` 索引、索引注入、异步 LLM 更新 |
| `internal/conversation` | 增加可选持久化 hook，并为整体替换增加替换原因 |
| `internal/compact` | 改成新 session ID 格式，并允许恢复时打开已有会话目录 |
| `internal/prompt` | `BuildSystemPrompt` 接受自定义指令和长期记忆两个可选输入 |
| `internal/agent` | 每次自然完成 Agent Loop 后触发异步记忆更新；每次 Run 前读取最新记忆索引 |
| `internal/tui` | 增加 `/resume` 命令、会话选择列表、恢复状态切换和恢复后的运行时替换 |
| `cmd/PseudoClaude` | 启动时串联指令加载、记忆初始化、后台清理和 TUI 注入 |

增量交互命令：

| 命令 | 行为 |
| ---- | ---- |
| `/memory` | 刷新并展示当前长期记忆索引，不进入模型、不写入会话 |
| `/resume` | 使用类似权限 allow 的轻量选择面板展示历史会话，支持上下键/数字键/Enter/Esc |

整体原则是让“记忆”成为请求组装前已经准备好的上下文，而不是模型运行中再检索。项目指令和记忆索引在 `Runner` 组装系统提示时注入；会话 JSONL 通过 `Conversation` hook 追加写；自动笔记只在 Agent 自然停下后异步更新，不修改当前对话历史。

## 核心数据结构

### `internal/instructions`

```go
const (
	FileName        = "PSEUDOCLAUDE.md"
	DefaultMaxDepth = 5
)

type Layer struct {
	Name     string // "project-root" / "project-config" / "user"
	Path     string
	Boundary string
}

type LoadResult struct {
	Content  string
	Loaded   []string
	Warnings []string
}

type Loader struct {
	ProjectRoot string
	UserHome    string
	MaxDepth    int
}

func NewLoader(projectRoot string) Loader
func (l Loader) Load() LoadResult
func (l Loader) Layers() []Layer
```

`Loader.Load` 按固定顺序读取：

1. `<project_root>/PSEUDOCLAUDE.md`
2. `<project_root>/.PseudoClaude/PSEUDOCLAUDE.md`
3. `~/.PseudoClaude/PSEUDOCLAUDE.md`

每层使用同一个 include 展开器：

```go
type expander struct {
	maxDepth int
}

func (e expander) expand(path, boundary string, depth int, visited map[string]struct{}) (string, []string)
func isIncludeLine(line string) (relativePath string, ok bool)
func insideBoundary(path, boundary string) bool
func looksBinary(sample []byte) bool
```

展开规则：

- 只有整行形如 `@include <relative_path>` 才处理。
- `visited` 是当前展开链路集合，不是全局集合；同一个文件可被不同顶层文件引用，但不能在一条链上成环。
- 路径先相对当前文件目录解析，再取绝对/clean 路径，最后用 `insideBoundary` 校验。
- 警告以 Markdown 注释形式写回内容，例如 `<!-- @include 检测到环路，已跳过: rules.md -->`。

### `internal/session`

```go
const (
	SessionsDirName      = "sessions"
	ConversationFileName = "conversation.jsonl"
	ToolResultsDirName   = "tool-results"
	SessionIDLayout       = "20060102-150405"
	ExpiryAge            = 30 * 24 * time.Hour
)

type Context struct {
	ID        string
	Dir       string
	JSONLPath string
	SpillDir  string
}

func NewID(now time.Time) string
func ParseID(id string) (time.Time, bool)
func NewContext(workspace string, now time.Time) (Context, error)
func OpenContext(workspace, id string) (Context, error)
```

JSONL 行结构：

```go
type Entry struct {
	Type      string          `json:"type,omitempty"`   // "message" / "replace"
	Reason    string          `json:"reason,omitempty"` // "snapshot" / "compact"
	Role      string          `json:"role,omitempty"`
	Content   string          `json:"content,omitempty"`
	ToolCalls []llm.ToolCall  `json:"tool_calls,omitempty"`
	ToolResult *llm.ToolResult `json:"tool_result,omitempty"`
	TS        int64           `json:"ts"`
	Model     string          `json:"model,omitempty"`
}

const (
	EntryMessage = "message"
	EntryReplace = "replace"

	ReplaceSnapshot = "snapshot"
	ReplaceCompact  = "compact"
)
```

`replace` 记录解决 append-only 与内存整体替换之间的张力：Part 7 的工具结果落盘会把历史中大工具结果改成预览，这不是摘要压缩；真正的重量压缩会把早期历史替换成摘要。两者都不能重写旧 JSONL，所以统一追加 `replace` 标记和替换后的完整消息快照。恢复时从最后一个 `replace` 标记之后开始加载；如果 `reason=compact`，它同时满足“可识别压缩边界”的要求。

写入器：

```go
type Writer struct {
	mu       sync.Mutex
	file     *os.File
	model    string
	wroteMsg bool
	onError  func(error)
}

func NewWriter(ctx Context, model string, onError func(error)) (*Writer, error)
func OpenWriter(ctx Context, model string, onError func(error)) (*Writer, error)
func (w *Writer) AppendMessage(msg llm.Message)
func (w *Writer) AppendReplace(reason string, msgs []llm.Message)
func (w *Writer) Close() error
```

`AppendMessage` 和 `AppendReplace` 不向调用方返回错误，因为它们会作为 `Conversation` hook 调用；错误通过 `onError` 记录到日志或 TUI 状态。每次成功写入完整行后调用 `Sync`，保证崩溃最多影响最后一行。

扫描和恢复：

```go
type Info struct {
	ID          string
	Title       string
	Model       string
	MessageCount int
	ModifiedAt  time.Time
	Size        int64
	Dir         string
}

type LoadResult struct {
	ID          string
	Messages    []llm.Message
	LastMessage time.Time
	BadLines    int
	Truncated   bool
}

func List(workspace string) ([]Info, error)
func Load(ctx Context) (LoadResult, error)
func CleanExpired(workspace string, now time.Time) []error
```

恢复规则：

- 逐行扫描，坏行计数后跳过。
- 遇到 `type=replace` 时清空当前累积消息，并从该行之后重新累积。
- 只把合法 `message` 行转成 `llm.Message`。
- 加载完成后调用 `truncateDanglingToolCalls`，如果最后一条 assistant tool call 没有完整 tool result，就截断到该 assistant 之前。

### `internal/memory`

```go
type Level string
type NoteType string

const (
	LevelProject Level = "project"
	LevelUser    Level = "user"

	TypeUserPreference     NoteType = "user_preference"
	TypeCorrectionFeedback NoteType = "correction_feedback"
	TypeProjectKnowledge   NoteType = "project_knowledge"
	TypeReferenceMaterial  NoteType = "reference_material"

	IndexFileName = "MEMORY.md"
	MaxIndexLines = 200
	MaxIndexBytes = 25 * 1024
)
```

笔记与操作：

```go
type Note struct {
	Type    NoteType
	Title   string
	Content string
	Created time.Time
	Updated time.Time
}

type Operation struct {
	Action   string `json:"action"`   // "create" / "update" / "delete"
	Level    Level  `json:"level"`    // "project" / "user"
	Type     NoteType `json:"type,omitempty"`
	Title    string `json:"title,omitempty"`
	Summary  string `json:"summary,omitempty"`
	Slug     string `json:"slug,omitempty"`
	Filename string `json:"filename,omitempty"`
	Content  string `json:"content,omitempty"`
}

type Store struct {
	Level Level
	Dir   string
	mu    sync.Mutex
}

func NewStore(level Level, dir string) *Store
func (s *Store) LoadIndex() string
func (s *Store) Apply(ops []Operation, now time.Time) error
func (s *Store) NotePath(filename string) (string, error)
```

文件安全：

- create 文件名由 `type + "_" + safeSlug(slug) + ".md"` 生成。
- update/delete 只接受普通文件名，不接受路径分隔符。
- 所有目标路径都必须通过 `NotePath` 验证仍在对应 memory 目录内。

管理器：

```go
type Manager struct {
	project *Store
	user    *Store
	provider llm.Provider
	mu       sync.Mutex
	index    string
}

type UpdateInput struct {
	Messages []llm.Message
}

func NewManager(projectDir, userDir string) *Manager
func (m *Manager) SetProvider(provider llm.Provider)
func (m *Manager) IndexText() string
func (m *Manager) RefreshIndex()
func (m *Manager) UpdateAsync(ctx context.Context, input UpdateInput)
```

为 TUI 可见性命令增加只读接口：

```go
func (m *Manager) RefreshAndIndexText() string
```

该接口同步刷新项目级和用户级 `MEMORY.md` 索引，然后返回与系统提示注入一致的文本。

`IndexText` 返回项目级索引在前、用户级索引在后的拼接文本。`RefreshIndex` 读取两个 `MEMORY.md`，按 200 行 / 25KB 限制裁剪。`UpdateAsync` 内部持锁，避免两次记忆更新并发改同一组文件。

LLM 更新请求使用现有 `llm.Provider.Stream`，不传 tools：

```go
func BuildUpdatePrompt(turn []llm.Message, projectIndex, userIndex string) []llm.Message
func collectJSONOperations(ctx context.Context, provider llm.Provider, messages []llm.Message) ([]Operation, error)
```

prompt 要求模型只输出 JSON 数组，空数组代表无变化；去重、合并和淘汰由模型根据现有索引判断。

### `internal/conversation`

```go
type ReplaceReason string

const (
	ReplaceReasonSnapshot ReplaceReason = "snapshot"
	ReplaceReasonCompact  ReplaceReason = "compact"
)

type Hooks struct {
	OnAppend  func(llm.Message)
	OnReplace func(ReplaceReason, []llm.Message)
}

type Conversation struct {
	mu       sync.Mutex
	messages []llm.Message
	hooks    Hooks
}

func New(hooks Hooks) *Conversation
func NewFromMessages(messages []llm.Message, hooks Hooks) *Conversation
func (c *Conversation) SetHooks(hooks Hooks)
func (c *Conversation) ReplaceMessages(reason ReplaceReason, messages []llm.Message)
```

### `internal/tui` 增量状态

`/resume` 不再使用完整 list 组件作为主界面，而是在 `stateResuming` 下维护轻量选择状态：

```go
type resumeChoice struct {
    info session.Info
}

type Model struct {
    resumeChoices []resumeChoice
    resumeCursor  int
}
```

视图层复用权限审批面板样式，展示最近会话并接受上下键、数字键、Enter 和 Esc。`/memory` 不新增状态，只向 transcript 追加一个只读状态块。

现有 `AddUser`、`AddAssistant`、`AddAssistantToolCalls`、`AddToolResult` 在 append 后触发 `OnAppend`。`ReplaceMessages` 深拷贝新历史后触发 `OnReplace`。未设置 hook 时行为与现在一致。

### `internal/compact`

```go
type Session struct {
	ID       string
	RootDir  string
	SpillDir string
}

func NewRuntime(workspace string, contextWindow int64) (*Runtime, error)
func OpenRuntime(workspace, sessionID string, contextWindow int64) (*Runtime, error)
func (r *Runtime) SwitchSession(session Session)
```

改动点：

- `newSessionID()` 改为 `YYYYMMDD-HHMMSS-xxxx`。
- `NewRuntime` 使用 `session.NewContext` 创建目录。
- `OpenRuntime` 使用 `session.OpenContext` 打开已有会话。
- Layer 1 工具结果落盘调用 `Conversation.ReplaceMessages(ReplaceReasonSnapshot, ...)`。
- Layer 2 摘要压缩调用 `Conversation.ReplaceMessages(ReplaceReasonCompact, ...)`。

### `internal/prompt`

```go
type PromptInputs struct {
	Instructions string
	Memory       string
}

func BuildSystemPrompt(inputs PromptInputs) string
```

`BuildSystemPrompt` 继续使用已有 fixed modules 和 optional modules。`Instructions` 非空时填入 `Custom Instructions`，`Memory` 非空时填入 `Long-Term Memory`。优先级沿用当前常量：自定义指令 priority 80，长期记忆 priority 100。

### `internal/agent`

```go
type MemoryUpdater interface {
	IndexText() string
	UpdateAsync(ctx context.Context, input memory.UpdateInput)
}

type Runner struct {
	Provider     llm.Provider
	Registry     *tools.Registry
	Env          tools.Env
	Config       Config
	Version      string
	Permission   *permission.Engine
	Compact      *compact.Runtime
	Instructions string
	Memory       MemoryUpdater
}
```

Run 流程调整：

- 在 `AddUser` 前记录 `startLen := req.Conversation.Len()`。
- 每次 Run 开始时调用 `prompt.BuildSystemPrompt(prompt.PromptInputs{Instructions: r.Instructions, Memory: r.Memory.IndexText()})`。
- 当模型最终无工具调用并 `StopCompleted` 时，截取 `req.Conversation.Messages()[startLen:]` 作为本轮片段，调用 `r.Memory.UpdateAsync(context.Background(), memory.UpdateInput{Messages: recent})`。
- 只有自然完成才触发记忆更新；取消、流错误、最大迭代、未知工具过多都不触发。

## 模块设计

### `internal/instructions`

**职责：** 从固定三层位置加载用户手写 Markdown，展开安全 include，输出稳定指令文本。

**对外接口：** `NewLoader(projectRoot).Load()`。

**依赖：** 标准库 `os`、`path/filepath`、`strings`。不依赖 prompt、TUI 或 LLM。

**满足需求：** F1-F11、N1、N6、N7、N9。

### `internal/session`

**职责：** 会话身份、目录布局、JSONL append-only 写入、扫描列表、恢复解析、过期清理。

**对外接口：** `NewContext`、`OpenContext`、`Writer`、`List`、`Load`、`CleanExpired`。

**依赖：** 标准库和 `internal/llm`。不依赖 TUI、agent 或 compact，避免循环。

**满足需求：** F12-F34、F55、N2、N3、N6、N7、N9。

### `internal/memory`

**职责：** 两级笔记目录、索引加载和裁剪、异步 LLM 更新、结构化操作执行。

**对外接口：** `Manager.IndexText`、`Manager.RefreshIndex`、`Manager.UpdateAsync`、`Manager.SetProvider`。

**依赖：** `internal/llm`。不依赖 agent 或 TUI。

**满足需求：** F35-F51、F56、N5、N6、N7、N8、N9。

### `internal/conversation`

**职责：** 继续作为线程安全消息容器，同时把追加和整体替换事件通知持久化层。

**对外接口：** 现有 Add/Message 方法保持；新增构造、hook 和带 reason 的替换。

**依赖：** 只依赖 `internal/llm`。

**满足需求：** F17-F19、F30、F55。

### `internal/compact`

**职责：** 保持 Part 7 的压缩能力，同时复用新的 session 目录和替换原因。

**对外接口：** `NewRuntime` 保持可用；新增 `OpenRuntime` 和 `SwitchSession`。

**依赖：** 新增依赖 `internal/session` 只用于目录和 ID。

**满足需求：** F13、F18、F28、F30、F55、AC26。

### `internal/tui`

**职责：** 提供 `/resume` 用户入口，展示会话列表，执行恢复并替换当前运行时。

**核心新增状态：**

```go
const (
	stateSelecting sessionState = iota
	stateIdle
	stateStreaming
	stateApproving
	stateResuming
)
```

**新增字段：**

```go
type Model struct {
	// existing fields...
	instructions string
	memory       *memory.Manager
	sessionCtx   session.Context
	sessionWriter *session.Writer
	resumeList   list.Model
}
```

**满足需求：** F21-F32、F52-F57、N4、N9。

### `cmd/PseudoClaude`

**职责：** 在启动时完成全局资源准备，并把指令、记忆、会话资源交给 TUI。

**启动顺序：**

1. 加载 `.PseudoClaude/config.yaml`。
2. 读取 cwd。
3. 加载 `instructions.NewLoader(cwd).Load()`。
4. 初始化 `memory.NewManager(<cwd>/.PseudoClaude/memory, ~/.PseudoClaude/memory)` 并 `RefreshIndex`。
5. 后台执行 `session.CleanExpired(cwd, time.Now())`。
6. 初始化权限、工具注册、MCP。
7. 创建 TUI 时传入 instructions、memory manager。

## 模块交互

### 新会话启动

```text
cmd/PseudoClaude
  -> instructions.Load
  -> memory.NewManager + RefreshIndex
  -> session.CleanExpired in goroutine
  -> tui.New(..., WithInstructions, WithMemory)
      -> compact.NewRuntime
      -> session.NewWriter(compact session dir)
      -> conversation.New(hooks from writer)
      -> runner.Instructions = instructionText
      -> runner.Memory = memoryManager
```

Provider 选择发生在 TUI 内：

```text
provider selected
  -> llm.New(config)
  -> compactRuntime.SetContextWindow(config.EffectiveContextWindow)
  -> memoryManager.SetProvider(provider)
  -> runner.Provider = provider
```

### 普通 Agent Run

```text
TUI submit user text
  -> runner.Run(req)
      -> startLen = conv.Len()
      -> conv.AddUser(userText)
          -> writer.AppendMessage(user)
      -> stableSystem = BuildSystemPrompt(instructions, memory.IndexText())
      -> compact.ManageContext(auto)
          -> layer1? conv.ReplaceMessages(snapshot)
              -> writer.AppendReplace(snapshot, messages)
          -> layer2? conv.ReplaceMessages(compact)
              -> writer.AppendReplace(compact, messages)
      -> provider.Stream(...)
      -> assistant text
          -> conv.AddAssistant
          -> writer.AppendMessage(assistant)
      -> if no tool calls:
          -> memory.UpdateAsync(recent turn)
          -> StopCompleted
      -> if tool calls:
          -> conv.AddAssistantToolCalls
          -> writer.AppendMessage(assistant tool calls)
          -> execute tools
          -> conv.AddToolResult
          -> writer.AppendMessage(tool result)
          -> next iteration
```

### `/resume` 恢复

```text
stateIdle + "/resume"
  -> session.List(cwd)
  -> stateResuming
  -> list.Model shows title / relative time / message count / size / model
  -> Enter selected item
      -> session.OpenContext(cwd, id)
      -> session.Load(ctx)
      -> if EstimateMessages(messages) over threshold:
          -> create temporary Conversation from messages
          -> compact.ForceCompact(...)
          -> messages = compacted messages
      -> if last message older than 6h:
          -> append time-span reminder to messages
      -> close old writer
      -> session.OpenWriter(selected ctx)
      -> compact.OpenRuntime(cwd, id, current context window)
      -> conversation.NewFromMessages(messages, writer hooks)
      -> replace model conv / writer / compactRuntime / runner.Compact
      -> stateIdle
```

恢复失败时保留原来的 `conv`、writer 和 runtime，只在 transcript 中显示错误。

### 自动笔记更新

```text
Runner natural StopCompleted
  -> recent := messages since startLen
  -> go memory.UpdateAsync(context.Background(), recent)

memory.UpdateAsync
  -> lock manager
  -> read current project/user indexes
  -> build no-tools LLM request
  -> collect streamed JSON
  -> parse []Operation
  -> validate operation paths and note types
  -> projectStore.Apply(project ops)
  -> userStore.Apply(user ops)
  -> RefreshIndex
  -> unlock
```

如果 provider 未设置、返回空数组或出错，更新结束并记录错误，不影响主会话。

## 文件组织

```text
PseudoClaude/
├── cmd/PseudoClaude/main.go
├── internal/agent/
│   ├── runner.go              — 注入 instructions/memory，Run 完成后触发记忆更新
│   └── runner_test.go
├── internal/compact/
│   ├── runtime.go             — 新 session ID、OpenRuntime、SwitchSession
│   ├── summary.go             — ReplaceReasonCompact
│   ├── layer1.go              — ReplaceReasonSnapshot
│   └── compact_test.go
├── internal/conversation/
│   ├── conversation.go         — hooks、ReplaceReason、NewFromMessages
│   └── conversation_test.go
├── internal/instructions/
│   ├── loader.go               — 三层加载
│   ├── include.go              — @include 展开、安全检查
│   └── loader_test.go
├── internal/memory/
│   ├── manager.go              — Manager、索引刷新、异步更新
│   ├── prompt.go               — 记忆更新 prompt
│   ├── store.go                — Markdown 笔记与 MEMORY.md 操作
│   ├── types.go                — NoteType、Operation、常量
│   └── memory_test.go
├── internal/prompt/
│   ├── modules.go              — Optional modules 填充
│   └── prompt_test.go
├── internal/session/
│   ├── context.go              — session ID、目录布局
│   ├── writer.go               — JSONL append writer
│   ├── scan.go                 — List、Info
│   ├── load.go                 — Load、坏行跳过、截断孤立工具调用
│   ├── clean.go                — 30 天过期清理
│   └── session_test.go
└── internal/tui/
    ├── tui.go                  — Model 字段、构造参数
    ├── stream.go               — /resume 命令分发、submit 集成
    ├── resume.go               — stateResuming、列表展示、恢复执行
    ├── select.go               — provider 选择后设置 memory provider
    └── stream_test.go
```

运行时目录：

```text
<workspace>/.PseudoClaude/
├── PSEUDOCLAUDE.md
├── config.yaml
├── memory/
│   ├── MEMORY.md
│   ├── project_knowledge_api_conventions.md
│   └── reference_material_build_notes.md
└── sessions/
    └── 20260617-223015-a1b2/
        ├── conversation.jsonl
        └── tool-results/
            └── toolu_123.txt

~/.PseudoClaude/
├── PSEUDOCLAUDE.md
└── memory/
    ├── MEMORY.md
    ├── user_preference_concise_replies.md
    └── correction_feedback_no_unasked_refactors.md
```

## 技术决策

| 决策点 | 选择 | 理由 |
| ------ | ---- | ---- |
| 项目指令文件名 | `PSEUDOCLAUDE.md` | 与项目名一致，避免沿用参考项目名；三层路径都用同一文件名，用户容易理解 |
| 指令优先级 | 项目根 > 项目 `.PseudoClaude` > 用户 `~/.PseudoClaude` | 越靠近当前项目越具体，放在前面让模型优先遵循 |
| include 展开 | 手写逐行解析 | 语法很小，逐行解析比引入 Markdown parser 更稳定，且能保留原文 |
| 会话存储 | JSONL append-only | 追加快，崩溃最多损坏最后一行，恢复时坏行可跳过 |
| meta 信息 | 不维护 meta 文件 | 避免 JSONL 与 meta 双写不一致；列表信息通过扫描计算 |
| 内存整体替换记录 | `replace` 标记 + 完整快照 | append-only 不能重写旧行；快照让 layer1 预览替换和 layer2 摘要压缩都能恢复一致 |
| 替换原因 | `snapshot` 与 `compact` 分开 | 工具结果落盘不是摘要压缩，恢复和测试需要区分 |
| 恢复超限处理 | 复用 Part 7 `ForceCompact` | 不新增第二套摘要机制，保持摘要语义一致 |
| 自动笔记触发 | 每次自然 `StopCompleted` 后异步触发 | 符合 spec 的“每轮自然停下后”；异步避免拖慢交互 |
| 记忆去重 | 交给 LLM | 系统负责安全和格式，语义合并由模型根据索引判断 |
| 记忆注入 | 只注入 `MEMORY.md` 索引 | 控制上下文成本，需要详情时再通过文件工具读取笔记全文 |
| 记忆更新 provider | 使用当前会话 provider，tools 为空 | 不引入新模型配置；无工具调用降低副作用 |
| TUI 恢复入口 | `/resume` 仅 idle 可用 | 防止 Agent Run 中途替换 conversation 和 runtime |
| 过期清理 | 后台清理新格式 ID 且超过 30 天的目录 | 不阻塞启动；旧目录不清理，避免误删 |

## Spec 覆盖

| Spec 范围 | 设计归属 |
| --------- | -------- |
| F1-F11 项目指令 | `internal/instructions` + `prompt.BuildSystemPrompt` |
| F12-F20 会话存档 | `internal/session` + `conversation.Hooks` + `compact.Session` |
| F21-F34 会话恢复 | `internal/session.Load/List/CleanExpired` + `internal/tui/resume.go` |
| F35-F51 自动笔记 | `internal/memory` + `agent.Runner.Memory` |
| F52-F57 生命周期集成 | `cmd/PseudoClaude` + `tui.New` + `agent.Runner` |
| N1/N2 性能与可靠性 | 启动只读轻量文件；JSONL 单行 Sync；清理后台执行 |
| N3/N6/N9 错误隔离 | 坏行跳过、路径边界检查、更新失败只记录 |
| N4 兼容性 | hooks 和 optional prompt 输入为空时保持旧行为 |
| N5/N8 记忆质量与大小 | 索引限制、prompt 要求只记稳定事实和明确偏好 |
| N7 可测试性 | 新逻辑集中在无 TUI/无网络也可测的包，LLM 通过 mock provider 验证 |
