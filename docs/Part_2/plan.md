# Agent 工具系统 Plan

## 技术栈

- 语言：Go
- LLM 通信：沿用官方 SDK
  - `github.com/anthropics/anthropic-sdk-go v1.33.0`
  - `github.com/openai/openai-go/v3 v3.32.0`
- 工具参数与结果：`encoding/json` + JSON Schema 风格的 `map[string]any`
- 文件匹配与内容搜索：优先使用 Go 标准库实现；需要递归 glob 时在本地封装目录遍历
- 命令执行：`os/exec` + `context.WithTimeout`
- TUI：沿用 Part 1 的 bubbletea/bubbles/lipgloss/glamour

## 架构概览

本阶段在 Part 1 的“终端输入 → LLM 流式回复 → 会话历史”链路中插入工具系统，但不引入自动 Agent Loop。

1. `tools` 模块：定义统一工具接口、工具元信息、执行上下文、结构化结果、注册中心和六个内置工具。它只关心本地能力，不依赖任何 LLM SDK。
2. `llm` 模块：扩展协议无关的消息、流式事件和 provider 接口，让 provider 能接收工具定义并产出统一工具调用事件。Anthropic/OpenAI 的工具定义转换、流式参数拼接、工具调用历史回灌都限制在协议适配层。
3. `conversation` 模块：从纯文本消息历史升级为可承载普通文本、assistant 工具调用记录、user 工具结果记录的历史容器。
4. `tui` 模块：在用户提交时把工具定义传给 provider；当流式事件中出现工具调用完成时，调用注册中心执行工具，把结果追加回历史并显示状态；本轮到此结束并回到可输入状态。
5. `cmd/PseudoClaude` 入口：创建默认工具注册中心并注入 TUI。

## 核心数据结构

### `tools.Tool`

```go
type Tool interface {
    Definition() Definition
    Execute(ctx context.Context, input json.RawMessage, env Env) Result
}
```

职责：
- `Definition` 提供模型可见的名称、描述和参数 schema。
- `Execute` 负责解析自己的参数、执行业务并返回结构化结果。
- 所有工具内部自行做参数语义校验；JSON 语法错误由执行器统一拦截后也以 `Result` 返回。

### `tools.Definition`

```go
type Definition struct {
    Name        string
    Description string
    InputSchema map[string]any
}
```

职责：
- 作为注册中心的唯一键来源。
- 作为 Anthropic `ToolParam` 与 OpenAI `ChatCompletionFunctionToolParam` 的中间表示。

### `tools.Env`

```go
type Env struct {
    CWD              string
    Timeout          time.Duration
    MaxReadBytes     int64
    MaxOutputBytes   int
    MaxSearchResults int
}
```

职责：
- 统一注入当前工作目录、超时和输出限制。
- 文件工具、搜索工具、命令工具共享同一组执行边界。

### `tools.Result`

```go
type Result struct {
    OK        bool           `json:"ok"`
    Tool      string         `json:"tool"`
    Content   string         `json:"content,omitempty"`
    ErrorType string         `json:"error_type,omitempty"`
    Error     string         `json:"error,omitempty"`
    Metadata  map[string]any `json:"metadata,omitempty"`
}
```

职责：
- 成功时 `OK=true`，`Content` 放模型最需要的结果文本，`Metadata` 放路径、截断状态、退出码等补充信息。
- 失败时 `OK=false`，`ErrorType` 使用稳定短码，如 `invalid_arguments`、`not_found`、`not_unique`、`timeout`、`command_failed`、`io_error`。
- `String()` 或 `Marshal()` 生成可放入 tool result 的 JSON 字符串。

### `tools.Call`

```go
type Call struct {
    ID        string
    Name      string
    Arguments json.RawMessage
}
```

职责：
- 表达工具执行层需要的最小调用信息。
- 避免 `tools` 包引用 `llm.ToolCall`，保持工具模块不依赖 LLM 层。

### `tools.Registry`

```go
type Registry struct {
    tools map[string]Tool
}

func NewRegistry(toolList ...Tool) (*Registry, error)
func DefaultRegistry() (*Registry, error)
func (r *Registry) Register(tool Tool) error
func (r *Registry) Get(name string) (Tool, bool)
func (r *Registry) Definitions() []Definition
func (r *Registry) Execute(ctx context.Context, call Call, env Env) Result
```

职责：
- 集中登记和按名查找工具。
- 拒绝空名称和重复名称。
- 为 provider 提供工具定义列表。
- 对未知工具名、非法 JSON 参数、工具超时进行统一错误包装。

### `llm.Message`

```go
type Message struct {
    Role      string
    Content   string
    ToolCalls []ToolCall
    ToolResult *ToolResult
}
```

职责：
- 兼容 Part 1 的纯文本 user/assistant 消息。
- 当 `Role=="assistant"` 且 `ToolCalls` 非空时，表示模型请求工具。
- 当 `Role=="user"` 且 `ToolResult` 非空时，表示工具执行结果回灌。
- 协议适配层把该统一结构转换成各自 SDK 需要的 message param。

### `llm.ToolCall`

```go
type ToolCall struct {
    ID        string
    Name      string
    Arguments json.RawMessage
}
```

职责：
- 表达一次模型请求的工具调用。
- `ID` 用于把结果对应回原调用。
- `Arguments` 保存拼接后的原始 JSON 参数，由工具执行层解析。

### `llm.ToolResult`

```go
type ToolResult struct {
    CallID  string
    Name    string
    Content string
    IsError bool
}
```

职责：
- 表达一次工具结果。
- `Content` 存放 `tools.Result` 序列化后的 JSON。
- `IsError` 让 Anthropic 的 `tool_result` 与 TUI 展示能区分成功/失败。

### `llm.StreamEvent`

```go
type StreamEvent struct {
    Text     string
    ToolCall *ToolCall
    Done     bool
    Err      error
}
```

职责：
- 保持 Part 1 的文本增量、完成和错误行为。
- 新增 `ToolCall`，表示一个完整工具调用已经从流式响应中拼接完成，可以执行。
- 本阶段上层只在 `ToolCall` 完整时收到事件，不暴露半截 JSON 参数。

### `llm.Provider`

```go
type Provider interface {
    Name() string
    Model() string
    Stream(ctx context.Context, msgs []Message, defs []tools.Definition) <-chan StreamEvent
}
```

职责：
- 每次请求都接收当前工具定义列表。
- 没有工具定义时仍可按 Part 1 行为纯文本流式回复。
- 协议适配层负责把 `defs` 转成 API 工具列表，并把流式工具调用转成 `StreamEvent.ToolCall`。

## 模块设计

### `tools` 模块

**职责：** 提供工具抽象、注册中心、公共执行限制和六个内置工具。

**对外接口：**
- `Tool`、`Definition`、`Env`、`Call`、`Result`
- `Registry` 及 `DefaultRegistry`
- 六个工具构造函数：`NewReadFileTool`、`NewWriteFileTool`、`NewEditFileTool`、`NewRunCommandTool`、`NewFindFilesTool`、`NewSearchCodeTool`

**内置工具：**
- `read_file`
  - 参数：`path`
  - 行为：读取文本文件，受 `MaxReadBytes` 限制；目录、缺失、不可读返回错误。
- `write_file`
  - 参数：`path`、`content`
  - 行为：创建父目录并写入完整内容；写入失败返回错误。
- `edit_file`
  - 参数：`path`、`old_text`、`new_text`
  - 行为：读取文本，统计 `old_text` 出现次数；正好一次才替换并写回。
- `run_command`
  - 参数：`command`、可选 `args`
  - 行为：在 `Env.CWD` 执行命令，捕获 stdout/stderr/exit code；非零退出返回 `OK=false` 但携带输出。
- `find_files`
  - 参数：`pattern`
  - 行为：按 glob 风格模式匹配路径；递归场景通过遍历目录实现；结果数受限。
- `search_code`
  - 参数：`pattern`、可选 `regex`、可选 `path`
  - 行为：在目标目录或文件中搜索内容，返回文件、行号和行摘要；跳过目录和不可读/二进制文件。

**依赖：** 标准库 `context`、`encoding/json`、`errors`、`fmt`、`io/fs`、`os`、`os/exec`、`path/filepath`、`regexp`、`strings`、`time`。

### `llm` 模块

**职责：** 扩展 provider 抽象，隐藏 Anthropic/OpenAI 工具调用差异。

**对外接口：**
- 扩展后的 `Message`、`ToolCall`、`ToolResult`、`StreamEvent`、`Provider`
- `New(cfg config.ProviderConfig) (Provider, error)`

**Anthropic 适配：**
- 请求：
  - `MessageNewParams.Tools` 使用 `[]anthropic.ToolUnionParam`。
  - 每个 `tools.Definition` 转为 `anthropic.ToolParam{Name, Description, InputSchema}`；`InputSchema` 从通用 schema 中提取 `properties`、`required` 和 `type=object` 填入 `anthropic.ToolInputSchemaParam`。
  - 文本 user/assistant 仍转为 `NewUserMessage(NewTextBlock(...))` / `NewAssistantMessage(...)`。
  - assistant 工具调用历史转为 `anthropic.NewAssistantMessage(anthropic.NewToolUseBlock(id, input, name))`。
  - user 工具结果历史转为 `anthropic.NewUserMessage(anthropic.NewToolResultBlock(callID, content, isError))`。
- 流式：
  - 继续解析 `TextDelta` 为 `StreamEvent.Text`。
  - 忽略 `ThinkingDelta`。
  - 使用 SDK 的 `message.Accumulate(event)` 累积完整 message；流结束后扫描 `message.Content` 中的 `ToolUseBlock`，逐个发出 `StreamEvent{ToolCall: ...}`。
  - 若 `ToolUseBlock.Input` 不是合法 JSON，由上层 registry 执行时返回参数错误。

**OpenAI 适配：**
- 请求：
  - `ChatCompletionNewParams.Tools` 使用 `[]openai.ChatCompletionToolUnionParam`。
  - 每个 `tools.Definition` 转为 `openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{...})`；`InputSchema` 直接作为 `shared.FunctionParameters`。
  - 文本 user/assistant 仍走 `openai.UserMessage` / `openai.AssistantMessage`。
  - assistant 工具调用历史需要构造带 `ToolCalls` 的 assistant message param。
  - user 工具结果历史使用 `openai.ToolMessage(content, callID)`。
  - 设置 `ParallelToolCalls` 为 false，便于按单调用完成事件稳定解析；模型仍可以在一个 assistant 消息中顺序产生多个工具调用。
- 流式：
  - 继续解析 `Choices[0].Delta.Content` 为 `StreamEvent.Text`。
  - 使用 `openai.ChatCompletionAccumulator` 累积 tool call 参数。
  - 每次 `acc.JustFinishedToolCall()` 返回完成调用时，发出 `StreamEvent{ToolCall: ...}`。
  - 流结束后，如果 accumulator 中仍有未通过 `JustFinishedToolCall` 发出的工具调用，补发一次，防止最后一个调用只在结束时完整。

**依赖：** `tools` 的 `Definition` 类型、`config`、`prompt`、Anthropic SDK、OpenAI SDK。

### `conversation` 模块

**职责：** 维护单进程对话历史，并支持工具调用/工具结果进入历史。

**对外接口：**
```go
func (c *Conversation) AddUser(text string)
func (c *Conversation) AddAssistant(text string)
func (c *Conversation) AddAssistantToolCall(call llm.ToolCall)
func (c *Conversation) AddToolResult(result llm.ToolResult)
func (c *Conversation) Messages() []llm.Message
```

**设计要点：**
- 保留 `AddUser` 和 `AddAssistant` 以兼容 Part 1。
- `AddAssistantToolCall` 在历史中追加 assistant 工具调用记录。
- `AddToolResult` 在历史中追加 user 工具结果记录。
- `Messages()` 继续返回深拷贝，避免外部修改内部历史。

**依赖：** `llm`。

### `tui` 模块

**职责：** 编排“一次用户提交 → 模型流式响应 → 工具调用执行 → 结果回灌 → 回到输入状态”。

**Model 新增字段：**
```go
registry *tools.Registry
toolEnv  tools.Env
curTool  *llm.ToolCall
```

**状态处理：**
- `submit`
  - `conv.AddUser(text)`
  - `events = provider.Stream(ctx, conv.Messages(), registry.Definitions())`
  - 进入 `stateStreaming`
- `updateStreaming`
  - `Text`：沿用 Part 1，追加到 `curReply`。
  - `ToolCall`：
    - 若已有文本回复，先把文本以 assistant 消息追加历史并打印。
    - 追加 assistant 工具调用历史。
    - 打印工具开始状态，如 `Tool: read_file(...)`。
    - 将 `llm.ToolCall` 映射为 `tools.Call` 后调用 `registry.Execute(ctx, toolsCall, toolEnv)`。
    - 将 `tools.Result` 序列化为 `llm.ToolResult` 并追加历史。
    - 打印成功或失败状态。
    - 停止当前流式处理，回到 `stateIdle`；不再次调用模型。
  - `Done`：无工具调用时沿用 Part 1 完成行为。
  - `Err`：沿用 Part 1 错误展示，回到 `stateIdle`。

**构造函数：**
```go
func New(providers []config.ProviderConfig, cwd string, registry *tools.Registry) Model
```

**依赖：** `tools`、`llm`、`conversation`、`config`、现有 TUI 依赖。

### `cmd/PseudoClaude` 入口

**职责：** 装配默认工具系统。

**流程：**
- `config.Load`
- `os.Getwd`
- `tools.DefaultRegistry`
- `tui.New(cfg.Providers, cwd, registry).Run`

**错误处理：**
- 工具注册失败作为启动期错误打印并退出。

## 模块交互

### 启动链路

```text
main
  → config.Load(".PseudoClaude/config.yaml")
  → os.Getwd()
  → tools.DefaultRegistry()
  → tui.New(providers, cwd, registry)
  → tui.Run()
```

### 普通文本对话链路

```text
用户输入
  → conversation.AddUser
  → Provider.Stream(ctx, history, tool definitions)
  → StreamEvent.Text 增量
  → TUI curReply 逐字显示
  → StreamEvent.Done
  → conversation.AddAssistant
  → markdown 定型展示
  → 回到输入状态
```

### 工具调用链路

```text
用户输入
  → conversation.AddUser
  → Provider.Stream(ctx, history, tool definitions)
  → SDK 流式事件
  → provider 适配层拼接完整工具参数
  → StreamEvent.ToolCall
  → conversation.AddAssistantToolCall
  → tools.Registry.Execute
  → tools.Result
  → conversation.AddToolResult
  → TUI 展示工具结果摘要
  → 停止本轮处理并回到输入状态
```

### 下一次请求携带工具结果

```text
用户再次输入
  → conversation.AddUser
  → Provider.Stream(ctx, history, tool definitions)
  → 协议适配层把历史中的 assistant tool_use/tool_calls 和 user tool_result/tool message 转成 SDK messages
```

## 文件组织

```text
PseudoClaude/
├── cmd/
│   └── PseudoClaude/
│       └── main.go                       — 创建默认工具注册中心并注入 TUI
├── internal/
│   ├── tools/
│   │   ├── tool.go                       — Tool、Definition、Env、Result
│   │   ├── registry.go                   — Registry、DefaultRegistry、统一执行错误包装
│   │   ├── file.go                       — read_file、write_file、edit_file
│   │   ├── command.go                    — run_command
│   │   ├── search.go                     — find_files、search_code
│   │   ├── limits.go                     — 截断、文本/二进制判断、路径解析等公共 helper
│   │   ├── registry_test.go              — 注册、重复名称、未知工具、参数错误
│   │   ├── file_test.go                  — 文件读写、唯一匹配替换、边界错误
│   │   ├── command_test.go               — 命令成功、非零退出、超时
│   │   └── search_test.go                — glob 查找、文本/正则搜索、结果截断
│   ├── llm/
│   │   ├── provider.go                   — 扩展 Message、ToolCall、ToolResult、StreamEvent、Provider
│   │   ├── anthropic.go                  — 注入 Anthropic tools，解析 tool_use，回灌 tool_result
│   │   ├── openai.go                     — 注入 OpenAI tools，解析 tool_calls，回灌 tool messages
│   │   ├── stream.go                     — 发送 StreamEvent helper 保持
│   │   ├── anthropic_test.go             — Anthropic message/tool 转换单元测试
│   │   └── openai_test.go                — OpenAI message/tool 转换与 accumulator 单元测试
│   ├── conversation/
│   │   ├── conversation.go               — 增加工具调用/结果历史方法
│   │   └── conversation_test.go          — 历史顺序与深拷贝测试
│   └── tui/
│       ├── tui.go                        — Model 新增 registry/toolEnv，New 签名更新
│       ├── stream.go                     — ToolCall 事件执行与回灌
│       ├── view.go                       — 工具状态/结果展示样式
│       └── select.go                     — provider 选择后沿用同一 registry
└── docs/
    └── Part_2/
        ├── spec.md
        └── plan.md
```

## Spec 覆盖

| Spec | 设计归属 |
| ---- | -------- |
| F1 | `tools.Tool`、`tools.Definition` |
| F2 | `internal/tools/file.go`、`command.go`、`search.go` |
| F3 | `tools.Registry`、`llm.Provider.Stream(...defs)` |
| F4 | `tools.Registry.Execute` 与各工具参数解析 |
| F5 | `tools.Result`、`llm.ToolResult` |
| F6 | `tools.Env.Timeout`、`Registry.Execute`、各工具错误包装 |
| F7 | 文件工具与 `limits.go` 路径/文本 helper |
| F8 | `edit_file` 唯一匹配替换 |
| F9 | `run_command` |
| F10 | Anthropic `Message.Accumulate` 与 OpenAI `ChatCompletionAccumulator` |
| F11 | `llm.StreamEvent.ToolCall` 统一事件 |
| F12 | `conversation.AddAssistantToolCall`、`AddToolResult` 与协议 message 转换 |
| F13 | `tui.updateStreaming` 收到工具调用后执行并回 idle，不再次请求模型 |
| F14 | `tui.view.go` 工具状态展示 |

## 技术决策

| 决策点 | 选择 | 理由 |
| ------ | ---- | ---- |
| 工具模块依赖方向 | `tools` 不依赖 LLM SDK | 工具可单测、可复用；协议转换留在 `llm` |
| 工具参数格式 | `json.RawMessage` | 保留模型原始 JSON，统一处理碎片拼接后的解析错误 |
| 工具 schema 表达 | `map[string]any` JSON Schema | 两个 SDK 都接受 JSON schema 风格对象，避免引入额外 schema 库 |
| 结果格式 | `tools.Result` JSON 字符串作为 tool result content | 模型易解析；成功/失败结构一致 |
| 工具超时 | registry 统一加 `context.WithTimeout` | 避免每个工具重复超时样板，也能集中测试 |
| 文件修改策略 | 原文唯一匹配替换 | 严格满足 spec，避免模糊 patch 带来的误改 |
| 命令参数 | `command` + `args`，不用 shell 字符串 | 避免 shell 引号/注入语义混乱；需要 shell 行为时模型可显式调用 `sh -c` |
| Anthropic 流式解析 | 流中发文本，结束后通过 `message.Accumulate` 提取 `ToolUseBlock` | SDK 已提供完整累积能力，降低手写 JSON 碎片错误 |
| OpenAI 流式解析 | 使用 `ChatCompletionAccumulator`，并关闭 parallel tool calls | SDK 明确说明 `JustFinishedToolCall` 不适用于 parallel tool calls；关闭后行为稳定 |
| 工具调用后行为 | 执行并回灌历史后立即回 idle | 严格遵守本章不做 Agent Loop 的边界 |
| 普通文本兼容 | 没有工具调用时保持 Part 1 路径 | 降低回归风险，满足 N7 |
| 输出限制 | 读文件、搜索、命令输出统一截断并标记 | 防止刷屏和内存膨胀，满足 N5 |
