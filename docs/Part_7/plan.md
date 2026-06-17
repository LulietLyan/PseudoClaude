# PseudoClaude 上下文管理 Plan

## 架构概览

本阶段新增 `internal/compact/` 包作为上下文管理的核心模块。它负责两层压缩、token 估算、会话落盘目录、摘要 prompt、近期原文选择和自动摘要熔断。`compact` 只依赖 `conversation`、`llm`、`tools` 等底层包，不依赖 `agent` 或 `tui`，避免 UI 和主循环细节污染压缩逻辑。

`agent.Runner` 在每次发起普通模型请求前调用 `compact.ManageContext`。调用顺序固定为：先执行第 1 层工具结果落盘与预览替换，再基于替换后的历史重新估算 token；如果达到自动阈值并且熔断器未触发，再执行第 2 层摘要。模型请求成功后，Runner 用 provider 返回的 usage 更新压缩运行时的 token 锚点。

TUI 持有一个跨会话生命周期的 `*compact.Runtime`。当前 `Runner` 是每轮调用时通过 `Model.runner` 复用的值对象，不能把压缩状态放在单次请求的局部变量里；替换决策、usage 锚点、会话目录和熔断计数必须由 TUI Model 在整个进程会话中持有，并注入 Runner。Provider 选择发生时，TUI 同步把当前 provider 的 `context_window` 写入 Runtime。

手动压缩由 TUI 内置命令 `/compact` 触发。命令不进入 conversation，不作为用户消息发送给模型。TUI 在空闲状态下调用 `compact.ForceCompact`，复用与自动压缩相同的摘要、近期原文和边界消息构造逻辑；成功或失败都以 transcript 状态消息展示给用户。

配置层在 `ProviderConfig` 上新增可选字段 `context_window`。配置为正数时使用配置值；未配置时按协议推导默认窗口：Anthropic 200000，OpenAI 128000。该值不放进 provider 接口，而是在 `cmd/PseudoClaude` / TUI 的 provider 选择路径传给 Runtime。

## 核心数据结构

### `compact.Runtime`

```go
type Runtime struct {
    mu sync.Mutex

    Session       Session
    Replacements  ReplacementLedger
    AutoBreaker   AutoBreaker
    UsageAnchor   UsageAnchor
    ContextWindow int64
}
```

用途：保存当前进程会话内的上下文管理状态。它由 TUI Model 创建并长期持有，再注入 Runner。所有字段通过 Runtime 方法读写，避免 TUI 命令路径和 Runner 主循环并发访问时出现数据竞争。

核心方法：

```go
func NewRuntime(workspace string, contextWindow int64) (*Runtime, error)
func (r *Runtime) SetContextWindow(tokens int64)
func (r *Runtime) Snapshot() RuntimeSnapshot
func (r *Runtime) UpdateUsageAnchor(usage *llm.Usage, messageCount int)
func (r *Runtime) ResetUsageAnchor(tokens int64, messageCount int)
func (r *Runtime) RecordAutoSuccess()
func (r *Runtime) RecordAutoFailure()
func (r *Runtime) AutoTripped() bool
```

### `compact.Session`

```go
type Session struct {
    ID      string
    RootDir string
    SpillDir string
}
```

用途：描述当前会话的落盘位置。`SpillDir` 固定为 `<workspace>/.PseudoClaude/sessions/<session_id>/tool-results`。会话 ID 在 Runtime 创建时生成一次，格式为 `<unix_ts>-<short_random>`。

### `compact.ReplacementLedger`

```go
type ReplacementLedger struct {
    seen         map[string]ReplacementDecision
    replacements map[string]string
}

type ReplacementDecision string

const (
    DecisionKeep    ReplacementDecision = "keep"
    DecisionReplace ReplacementDecision = "replace"
)
```

用途：记录每个工具结果的稳定决策。键优先使用 `ToolResult.CallID`；如果未来出现没有 call id 的工具结果，再退化为消息位置派生 ID。`seen` 中的 `keep` 表示已决定保留原文，后续不再评估；`replace` 表示后续必须复用 `replacements[id]` 中保存的同一份预览字符串。

核心方法：

```go
func (l *ReplacementLedger) Existing(id string) (ReplacementDecision, string, bool)
func (l *ReplacementLedger) MarkKeep(id string)
func (l *ReplacementLedger) MarkReplace(id string, preview string)
```

这些方法只在 Runtime 锁内调用。

### `compact.UsageAnchor`

```go
type UsageAnchor struct {
    Tokens       int64
    MessageCount int
}
```

用途：保存最近一次可靠估算锚点。普通模型请求成功后，`Tokens` 使用该次 provider usage 的有效总量，`MessageCount` 记录当时 conversation 已包含的消息数。之后估算只对锚点之后新增消息按字符数比例增量计算。

当第 1 层压缩改写了锚点之前的历史，旧 usage 已不再对应当前消息，因此必须用压缩后的全量近似值重置锚点，直到下一次真实 provider usage 到来。

### `compact.ManageInput` / `ManageOutput`

```go
type TriggerKind string

const (
    TriggerAuto   TriggerKind = "auto"
    TriggerManual TriggerKind = "manual"
)

type ManageInput struct {
    Conversation *conversation.Conversation
    Runtime      *Runtime
    Provider     llm.Provider
    Trigger      TriggerKind
}

type ManageOutput struct {
    TriggeredLayer1 bool
    TriggeredLayer2 bool
    BeforeTokens    int64
    AfterTokens     int64
    OffloadedCount  int
}
```

用途：`ManageContext` 的输入输出。自动路径由 Runner 使用；手动路径由 TUI 使用。`Provider` 只在第 2 层摘要时使用，第 1 层不触碰模型。

### `compact.SummaryResult`

```go
type SummaryResult struct {
    SummaryText string
    Recent      []llm.Message
    Boundary    llm.Message
    Messages    []llm.Message
}
```

用途：承载摘要正式内容、近期原文、边界消息和最终替换 conversation 的消息列表。`SummaryText` 只包含正式摘要，不包含分析草稿。

### `config.ProviderConfig`

```go
type ProviderConfig struct {
    Name          string `yaml:"name"`
    Protocol      string `yaml:"protocol"`
    BaseURL       string `yaml:"base_url"`
    APIKey        string `yaml:"api_key"`
    Model         string `yaml:"model"`
    Thinking      bool   `yaml:"thinking"`
    ContextWindow int64  `yaml:"context_window"`
}
```

新增方法：

```go
func (p ProviderConfig) EffectiveContextWindow() int64
```

`ContextWindow <= 0` 时按 `Protocol` 返回默认值。

## 模块设计

### `internal/compact/constants.go`

职责：集中定义本阶段硬编码阈值。

```go
const (
    SingleToolResultLimitBytes = 50000
    ToolRoundAggregateLimitBytes = 200000
    SummaryReserveTokens = 20000
    AutoSafetyMarginTokens = 13000
    ManualSafetyMarginTokens = 3000
    RecentKeepTokens = 10000
    RecentKeepMessages = 5
    AutoFailureLimit = 3
    EstimateCharsPerToken = 3.5
    PreviewHeadBytes = 2048
    PreviewHeadLines = 20
    SummaryRetryLimit = 3
)
```

### `internal/compact/runtime.go`

职责：创建 Runtime、会话目录、线程安全状态访问、usage 锚点与熔断器管理。

关键行为：

- `NewRuntime` 创建 `.PseudoClaude/sessions/<session_id>/tool-results`。
- `SetContextWindow` 在 provider 选择变化时更新上下文窗口。
- `UpdateUsageAnchor` 只在 usage 非 nil 时更新；usage 缺失时保持旧锚点。
- `RecordAutoFailure` 连续失败达到 3 次后让 `AutoTripped` 返回 true。
- `RecordAutoSuccess` 清零连续失败计数。

### `internal/compact/layer1.go`

职责：执行轻量预防，把超大工具结果落盘并替换为稳定预览。

核心接口：

```go
type Layer1Output struct {
    Messages []llm.Message
    Changed bool
    OffloadedCount int
}

func OffloadToolResults(messages []llm.Message, rt *Runtime) (Layer1Output, error)
```

处理规则：

1. 遍历 `llm.Message`。只有 `ToolResult != nil` 的消息参与候选。
2. 对每个工具结果先查 Runtime ledger。已有 `keep` 决策则保持原文；已有 `replace` 决策则复用保存的预览字符串。
3. 未决策结果如果自身超过 `SingleToolResultLimitBytes`，写入 `Session.SpillDir/<call_id>`，构造预览并标记 `replace`。
4. 对同一轮工具结果做聚合判断：以一条带 `ToolCalls` 的 assistant 消息和其后连续的 matching tool result 消息为一组，剔除已替换项后统计剩余 content 字节数；如果超过 `ToolRoundAggregateLimitBytes`，按字节数从大到小继续落盘，直到剩余体积达标。
5. 未被替换的未决策结果标记 `keep`。
6. 落盘失败时不写 ledger，保留原文并继续处理其他结果。

预览格式稳定，包含：

- 原始字节数；
- 头部预览，先取前 20 行，再限制到 2048 字节；
- 完整落盘路径；
- 重读提示。

### `internal/compact/estimate.go`

职责：实现近似 token 估算。

核心接口：

```go
func EstimateMessages(messages []llm.Message) int64
func EstimateWithAnchor(messages []llm.Message, anchor UsageAnchor) int64
func UsageTokens(usage *llm.Usage) (int64, bool)
```

估算规则：

- `EstimateMessages` 对所有消息序列化后的可见内容按 `chars / 3.5` 估算。
- `EstimateWithAnchor` 使用 `anchor.Tokens + anchor.MessageCount 之后新增消息的估算值`。
- `UsageTokens` 优先使用 `usage.TotalTokens`；如果为 0，则使用 `InputTokens + OutputTokens + CacheRead + CacheWrite`。

### `internal/compact/summary.go`

职责：构造摘要请求、调用 provider、解析正式摘要、裁剪过大的摘要输入。

核心接口：

```go
func ForceCompact(ctx context.Context, in ManageInput) (ManageOutput, error)
func summarize(ctx context.Context, provider llm.Provider, messages []llm.Message, safetyMargin int64, contextWindow int64) (string, error)
```

摘要请求：

- `Tools` 必须为 nil。
- `System.Stable` 写明这是上下文压缩任务，并明确禁止工具调用。
- 用户消息包含待摘要历史和输出要求。
- Prompt 要求模型先输出 `<analysis>...</analysis>` 草稿，再输出 `<summary>...</summary>` 正式摘要。
- 解析时只保留 `<summary>` 内容；没有 summary 标签时返回错误。

正式摘要固定 9 个部分：

1. 主要请求和意图
2. 关键技术概念
3. 文件和代码位置
4. 错误与修复
5. 问题解决过程
6. 用户消息原文记录
7. 待办任务
8. 当前工作状态
9. 可能的下一步

摘要请求自身过大时，按较早历史优先裁剪。初版实现最多重试 3 次，每次丢弃最早的一组对话片段；仍失败则返回错误，并由自动路径记录一次失败。

### `internal/compact/recent.go`

职责：选择摘要后保留的近期原文，并保证工具调用与工具结果不被拆开。

核心接口：

```go
func SelectRecent(messages []llm.Message) []llm.Message
func ExpandToToolBoundary(messages []llm.Message, start int) int
```

选择规则：

- 从尾部向前累加，直到估算 token 大于等于 10000 且消息数大于等于 5。
- 如果起点落在工具结果消息上，向前扩展到包含对应 assistant tool call 的位置。
- 如果起点落在一组工具结果中间，向前扩展到该组工具调用之前。

### `internal/compact/manage.go`

职责：编排自动和手动压缩。

核心接口：

```go
func ManageContext(ctx context.Context, in ManageInput) (ManageOutput, error)
```

自动路径：

1. 读取压缩前估算值。
2. 调用 `OffloadToolResults`。
3. 如果第 1 层改变了历史，调用 `Conversation.ReplaceMessages`，并用压缩后的全量估算重置 Runtime usage 锚点。
4. 再次估算 token。
5. 如果未达到 `context_window - SummaryReserveTokens - AutoSafetyMarginTokens`，返回。
6. 如果自动熔断已触发，返回。
7. 触发摘要；成功后替换 conversation，重置 usage 锚点并清零自动失败计数；失败则记录自动失败并返回错误。

手动路径：

1. 跳过自动阈值判断。
2. 直接调用摘要路径。
3. 使用 `ManualSafetyMarginTokens` 判断摘要输入是否需要裁剪。
4. 成功后替换 conversation，重置 usage 锚点。
5. 失败时返回错误，不增加自动失败计数。

### `internal/conversation/conversation.go`

新增方法：

```go
func (c *Conversation) ReplaceMessages(messages []llm.Message)
func (c *Conversation) Len() int
```

`ReplaceMessages` 必须像 `Messages()` 一样深拷贝 `ToolCalls` 和 `ToolResult`，避免外部切片修改 conversation 内部状态。

### `internal/agent/runner.go`

新增字段：

```go
type Runner struct {
    Provider      llm.Provider
    Registry      *tools.Registry
    Env           tools.Env
    Config        Config
    Version       string
    Permission    *permission.Engine
    CompactRuntime *compact.Runtime
}
```

主循环改动：

1. `prepareRequest` 得到 tool definitions 后、每次构造 `llm.Request` 前，调用 `compact.ManageContext` 自动路径。
2. 如果返回 `TriggeredLayer2`，发送 progress 事件，例如 `正在压缩上下文...` 和 `已压缩，token 从 X 降至 Y`。
3. 使用压缩后的 `Conversation.Messages()` 构造普通模型请求。
4. 模型流收集完成后，在把 assistant 文本和 tool calls 写入 conversation 后，用 usage 更新 Runtime 锚点。
5. usage 更新时的 message count 应包含本次 assistant 输出或 tool calls，因为这些内容已经由 provider 输出 usage 覆盖；后续工具结果仍作为新增内容估算。

### `internal/agent/event.go`

复用 `EventProgress` 展示自动压缩状态，不新增事件类型。Progress 消息约定：

- `正在压缩上下文...`
- `已压缩上下文，token X -> Y`
- `上下文压缩失败: <error>`

### `internal/tui/stream.go`

职责：接入 `/compact` 命令。

改动：

- 把当前的 `/exit`、`/plan`、`/chat`、`/exit-plan`、`/do` 逻辑收敛到一个内置命令分发函数。
- `/compact` 只在 idle 状态执行。
- 执行时清空输入框，追加 transcript status `正在压缩上下文...`。
- 通过 `tea.Cmd` 调用 `compact.ForceCompact`，完成后回传结果消息。
- 成功时追加 `已压缩上下文，token X -> Y`。
- 失败时追加 error transcript。

### `internal/tui/tui.go` 和 `internal/tui/select.go`

职责：创建和更新 Runtime。

改动：

- `Model` 新增 `compactRuntime *compact.Runtime`。
- `New` 在已知 cwd 时创建 Runtime。若只有一个 provider，使用该 provider 的 `EffectiveContextWindow()` 初始化；若需要用户选择 provider，先用默认窗口初始化，选择后再 `SetContextWindow`。
- provider 选择成功后，把所选 provider cfg 的有效窗口写入 runtime，并把 runtime 注入 runner。

### `internal/config/config.go`

职责：新增 `context_window` 配置字段与默认推导。

新增常量：

```go
const (
    DefaultAnthropicContextWindow int64 = 200000
    DefaultOpenAIContextWindow int64 = 128000
)
```

新增校验：

- `context_window < 0` 报错。
- `context_window == 0` 表示使用默认值。

### `.PseudoClaude/config.yaml.example`

职责：展示可选字段。

新增注释：

```yaml
# context_window 单位 token，可选；未配置时按 protocol 默认
# anthropic 默认 200000，openai 默认 128000
context_window: 200000
```

## 模块交互

### 自动路径

```text
TUI submit
  -> Runner.Run
    -> Conversation.AddUser
    -> prepareRequest
    -> loop iteration
      -> compact.ManageContext(auto)
        -> OffloadToolResults
        -> EstimateWithAnchor
        -> summarize if threshold reached
        -> Conversation.ReplaceMessages if changed
      -> Provider.Stream(normal request)
      -> collect usage/text/tool calls
      -> Conversation.AddAssistant / AddAssistantToolCalls
      -> Runtime.UpdateUsageAnchor
      -> execute tools
      -> Conversation.AddToolResult
      -> next iteration
```

### 手动路径

```text
TUI idle enter "/compact"
  -> builtin command dispatch
    -> compact.ForceCompact(manual)
      -> summarize with no tools
      -> SelectRecent
      -> build boundary message
      -> Conversation.ReplaceMessages
      -> Runtime.ResetUsageAnchor
    -> transcript status or error
```

### 第 1 层落盘路径

```text
Conversation.Messages
  -> find ToolResult messages
  -> check Runtime.ReplacementLedger
  -> spill to .PseudoClaude/sessions/<session_id>/tool-results/<call_id>
  -> replace ToolResult.Content with stable preview
  -> Conversation.ReplaceMessages
```

### 第 2 层摘要后消息形态

```text
user: [Context summary]
assistant: structured summary text
user: boundary reminder
...recent original messages...
```

边界消息使用普通 user role，确保下次 provider 请求时模型能直接读到约束。

## 文件组织

```text
PseudoClaude/
├── .PseudoClaude/
│   └── config.yaml.example              # 增加 context_window 示例
├── internal/
│   ├── compact/
│   │   ├── constants.go                 # 阈值常量
│   │   ├── runtime.go                   # Runtime、Session、ledger、熔断、usage 锚点
│   │   ├── estimate.go                  # 近似 token 估算
│   │   ├── layer1.go                    # 工具结果落盘和稳定预览
│   │   ├── preview.go                   # 预览文本构造与头部截断
│   │   ├── summary.go                   # 摘要 prompt、provider 调用、summary 标签解析
│   │   ├── recent.go                    # 近期原文选择和工具边界扩展
│   │   ├── manage.go                    # 自动/手动压缩编排
│   │   ├── runtime_test.go
│   │   ├── estimate_test.go
│   │   ├── layer1_test.go
│   │   ├── summary_test.go
│   │   └── recent_test.go
│   ├── conversation/
│   │   ├── conversation.go              # 新增 ReplaceMessages、Len
│   │   └── conversation_test.go         # 增加深拷贝替换测试
│   ├── config/
│   │   ├── config.go                    # ProviderConfig.ContextWindow 与默认推导
│   │   └── config_test.go               # 新增 context_window 测试
│   ├── agent/
│   │   ├── runner.go                    # 自动压缩接入与 usage 锚点更新
│   │   └── runner_test.go               # 自动触发/未触发/usage 更新集成测试
│   └── tui/
│       ├── tui.go                       # Model 持有 compactRuntime
│       ├── select.go                    # provider 选择后更新 context window
│       ├── stream.go                    # /compact 命令和状态展示
│       └── stream_test.go               # 命令路由测试
└── docs/
    └── Part_7/
        ├── spec.md
        ├── plan.md
        ├── task.md
        └── checklist.md
```

## 技术决策

| 决策点 | 选择 | 理由 |
| --- | --- | --- |
| 压缩核心位置 | 新增 `internal/compact` | 让压缩逻辑脱离 TUI 和 Agent，便于单元测试和后续扩展 |
| 状态生命周期 | TUI Model 持有 `*compact.Runtime` 并注入 Runner | 当前 Runner 会跨 turn 复用但请求是异步执行，Runtime 放在 TUI 能覆盖整个进程会话 |
| 工具结果身份 | 优先使用 `ToolResult.CallID` | 与 provider tool_use id 对齐，能保证同一工具结果稳定复用 |
| 落盘目录 | `.PseudoClaude/sessions/<session_id>/tool-results/<call_id>` | 与项目现有 `.PseudoClaude` 配置目录一致，便于用户定位 |
| 自动触发顺序 | 每次请求前先第 1 层，再估算第 2 层阈值 | 工具结果是 token 大头，先落盘能减少不必要摘要 |
| 第 1 层失败策略 | 落盘失败则保留原文且不写 ledger | 保证磁盘问题不会破坏对话，也允许下次重试 |
| token 估算 | usage 锚点 + 新增消息字符数 / 3.5 | 满足本阶段“不做精确 tokenizer”的边界，同时利用 provider 真实 usage 校准 |
| 锚点重置 | 第 1 层或第 2 层改写历史后重置近似锚点 | 避免旧 usage 对应未压缩历史而持续高估 |
| 摘要请求工具 | `Tools: nil` 且 prompt 明确禁止工具 | 双重保证摘要阶段不产生工具调用 |
| 摘要保留内容 | 只解析并保留 `<summary>` | 满足“先写分析草稿，草稿用完丢弃”的要求 |
| 近期原文策略 | token 下界和消息条数下界同时满足 | 保证压缩后仍能接上当前任务，不因仅满足一个条件而保留过少 |
| 边界消息 role | 普通 user 消息 | provider 兼容性最好，能稳定进入下一次上下文 |
| 手动命令入口 | TUI idle 命令，不写 conversation | 避免 `/compact` 污染用户意图，同时符合用户主动压缩语义 |
| 熔断范围 | 只影响自动摘要 | 手动压缩是用户显式操作，不应被自动失败计数拦截 |
| provider 上下文窗口 | 配置层推导后注入 Runtime | 不扩大 `llm.Provider` 接口，减少 provider 实现改动 |

## Spec 覆盖映射

| Spec | Plan 覆盖 |
| --- | --- |
| F1-F7 | `layer1.go`、`preview.go`、`manage.go` 自动路径 |
| F8-F9 | `estimate.go`、`runtime.go`、Runner usage 更新 |
| F10-F12 | `manage.go` 自动/手动触发阈值 |
| F13-F16 | `summary.go` 摘要请求和解析 |
| F17-F19 | `recent.go`、边界消息构造 |
| F20-F22 | TUI `/compact` 命令和 progress/status 展示 |
| F23-F26 | `runtime.go` 熔断器、`summary.go` 重试裁剪 |
| F27 | `config.go`、TUI provider 选择注入 |
| F28 | `runtime.go` 会话目录 |
| F29 | compact 单元测试与 fake provider |
