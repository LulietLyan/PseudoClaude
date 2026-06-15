# 系统提示工程化 Plan

## 技术栈

- 语言：Go 1.26.4
- LLM SDK：`github.com/anthropics/anthropic-sdk-go v1.33.0`、`github.com/openai/openai-go/v3 v3.32.0`
- 并发与取消：`context.Context`、goroutine、channel
- 工具系统：沿用 `internal/tools` 的注册中心、工具定义、安全等级和结构化结果
- Agent Loop：沿用 `internal/agent` 的 Run 循环、事件流、工具分批执行和 Plan Mode
- TUI：沿用 bubbletea 事件循环；缓存命中不进入状态栏
- 测试：Go 单元测试为主，补充可重复的 smoke/debug 场景说明

当前 SDK 源码已核对到以下可用能力：

- Anthropic `Usage` 暴露 `CacheCreationInputTokens` 和 `CacheReadInputTokens`；`TextBlockParam`、`ToolParam` 等参数支持 `CacheControlEphemeralParam`。
- OpenAI Chat Completion 使用 `CompletionUsage`，其中 `PromptTokensDetails.CachedTokens` 可表示缓存命中 token；未返回时为零值。

## 架构概览

本章在提示构造、provider 请求装配和 Agent Run 三层叠加能力，不改变上一章 Agent Loop 的控制流。

1. `prompt` 包从单个 `SystemPrompt` 常量升级为“模块化装配 + 环境采集 + 补充消息构造”。它对外产出三类文本：稳定系统提示、环境信息段、带 `<system-reminder>` 标签的运行时补充指令。`prompt` 包只依赖标准库，不依赖 `llm`，避免环依赖。
2. `llm` 包把 `Provider.Stream` 入参从“消息 + 工具定义”改为 `Request` 结构体，承载消息、工具、稳定系统提示、环境段和 reminder。Anthropic provider 对稳定系统块使用缓存控制，环境段不打缓存；OpenAI provider 保持稳定系统文本在 system 消息前缀。`Usage` 增加缓存写入与读取字段。
3. `agent` 包在每次 Run 开始时采集环境、装配稳定系统提示；每轮迭代按模式和轮次计算本轮 reminder；再组装 `llm.Request` 调 provider。provider 返回的缓存用量通过现有 `EventUsage` 事件透传。
4. `tools` 包只调整工具描述文本，在系统提示和工具描述两处双重强化关键规则，不改变工具行为、schema 和安全等级。
5. TUI 只需要给 `Runner` 提供应用版本，并继续消费现有事件。缓存字段随 `llm.Usage` 可见，但状态栏不展示；调试或 smoke 验证通过测试消费者或后续轻量命令完成。

数据流：

```text
TUI / 测试消费者
  -> agent.Runner.Run
  -> prompt.BuildSystemPrompt()
  -> prompt.GatherEnvironment(version, providerName, model)
  -> prompt.PlanReminder(full) 或空 reminder
  -> llm.Request
  -> provider.Stream
  -> llm.StreamEvent{Text, ToolCall, Usage{CacheWrite, CacheRead}, Done, Err}
  -> agent.Event{Usage}
  -> TUI 忽略缓存字段 / smoke 或测试断言缓存字段
```

依赖方向保持无环：

```text
agent -> prompt
agent -> llm
agent -> conversation
agent -> tools
llm   -> config
llm   -> tools
prompt -> 标准库
```

## 核心数据结构

### `prompt.Module`

```go
type Module struct {
    Name     string
    Priority int
    Content  string
}
```

职责：

- 表示一个系统提示模块。
- `Name` 用于可读性和测试断言。
- `Priority` 数值越小优先级越高。
- `Content` 为空时装配跳过，用于可选空槽。

优先级约定：

| 模块 | Priority |
| ---- | -------- |
| 身份 | 10 |
| 系统约束 | 20 |
| 任务模式 | 30 |
| 动作执行 | 40 |
| 工具使用 | 50 |
| 语气风格 | 60 |
| 文本输出 | 70 |
| 自定义指令 | 80 |
| 已激活 Skill | 90 |
| 长期记忆 | 100 |

### `prompt.Environment`

```go
type Environment struct {
    WorkingDir string
    Platform   string
    Date       string
    GitStatus  string
    Version    string
    Provider   string
    Model      string
}
```

职责：

- 保存每次 Run 开始时采集到的动态环境。
- `WorkingDir` 来自当前工具环境或进程工作目录。
- `Platform` 来自运行时平台。
- `Date` 使用本地日期，格式稳定为 `YYYY-MM-DD`。
- `GitStatus` 是短摘要；非 git 目录、git 不可用或超时时为空或不可用。
- `Version` 从 TUI 或调用方传入。
- `Provider` 与 `Model` 来自当前 provider。

### `llm.System`

```go
type System struct {
    Stable      string
    Environment string
}
```

职责：

- `Stable` 是可缓存稳定系统提示。
- `Environment` 是非缓存环境信息段。
- provider 根据协议能力把两段放入合适的系统级通道。

### `llm.Request`

```go
type Request struct {
    Messages []Message
    Tools    []tools.Definition
    System   System
    Reminder string
}
```

职责：

- 作为 provider 单次流式请求的完整输入。
- `Messages` 是持久对话历史，不包含本轮 reminder。
- `Tools` 是本轮可见工具集，普通和执行模式为全量，计划模式为只读。
- `System` 承载稳定提示和环境段。
- `Reminder` 是已经带特殊标签的本轮补充消息；为空表示不注入。

### `llm.Usage`

```go
type Usage struct {
    InputTokens  int64
    OutputTokens int64
    TotalTokens  int64
    CacheWrite   int64
    CacheRead    int64
}
```

职责：

- 继续表达输入、输出和总 token。
- `CacheWrite` 表示本轮创建缓存的输入 token；Anthropic 从 `cache_creation_input_tokens` 读取，OpenAI 默认为 0。
- `CacheRead` 表示本轮命中缓存复用的输入 token；Anthropic 从 `cache_read_input_tokens` 读取，OpenAI 从 `prompt_tokens_details.cached_tokens` 读取。
- provider 不返回缓存字段时保持零值。

### `agent.Runner`

现有结构扩展：

```go
type Runner struct {
    Provider llm.Provider
    Registry *tools.Registry
    Env      tools.Env
    Config   Config
    Version  string
}
```

职责：

- `Version` 用于环境信息渲染。
- 其他字段语义不变。

## 核心接口

### `prompt` 包

```go
func FixedModules() []Module
func OptionalModules() []Module
func AssembleSystem(mods []Module) string
func BuildSystemPrompt() string

func GatherEnvironment(version, provider, model, cwd string) Environment
func (Environment) Render() string

func SystemReminder(body string) string
func PlanReminder(full bool) string
```

说明：

- `FixedModules` 返回七个固定模块。
- `OptionalModules` 返回三个空内容预留模块。
- `AssembleSystem` 按 `Priority` 升序排序、跳过空内容、用 `\n\n` 连接，确保没有多余空行。
- `BuildSystemPrompt` 等于固定模块和可选模块组合后的装配结果。
- `GatherEnvironment` 采集动态环境，失败字段降级。
- `Render` 输出稳定格式的环境信息段。
- `SystemReminder` 负责 `<system-reminder>` 包裹。
- `PlanReminder` 返回完整版或精简版计划模式提醒，已带标签。

### `llm.Provider`

```go
type Provider interface {
    Name() string
    Model() string
    Stream(ctx context.Context, req Request) <-chan StreamEvent
}
```

说明：

- 用 `Request` 替代原先 `Stream(ctx, msgs, defs)` 的位置参数。
- 后续新增系统块、缓存策略或 reminder 类型时，不再继续扩展参数列表。

## 模块设计

### `prompt` 包

**职责：** 负责稳定系统提示装配、动态环境采集、补充消息构造。

**对外接口：** `FixedModules`、`OptionalModules`、`AssembleSystem`、`BuildSystemPrompt`、`GatherEnvironment`、`Environment.Render`、`SystemReminder`、`PlanReminder`。

**实现要点：**

- 七个固定模块按优先级 10 到 70 排列；三个可选空槽按 80 到 100 排列。
- “工具使用”模块写入关键规则：优先使用 `read_file`、`find_files`、`search_code` 等专用工具；编辑前必须先读取相关文件；执行命令只用于确需 shell 的验证或操作。
- `AssembleSystem` 不读取环境、不读取时间、不读取工具注册表，保证稳定块跨轮逐字节一致。
- `GatherEnvironment` 不读取环境变量，避免泄漏凭据。
- git 状态采集使用短超时执行 `git status --porcelain`，失败时降级为空或不可用。
- `PlanReminder(true)` 包含完整计划模式规则；`PlanReminder(false)` 只包含精简提醒。

**依赖：** 标准库 `context`、`os`、`os/exec`、`runtime`、`sort`、`strings`、`time`。

### `llm` 包

**职责：** 将协议无关的 `llm.Request` 转换为 Anthropic 或 OpenAI 请求，解析流式文本、工具调用、用量和缓存字段。

**对外接口：** `Provider.Stream(ctx, req)`、`llm.Request`、`llm.System`、扩展后的 `llm.Usage`。

**通用改动：**

- 删除 provider 对 `internal/prompt` 的直接 import；系统提示由 agent 传入。
- `StreamEvent.Usage` 继续使用 `*llm.Usage`，新增字段自动透传到 agent 和 TUI。
- provider 单元测试增加对 request 装配和缓存字段解析的断言。

**Anthropic provider：**

- `req.System.Stable` 非空时创建一个 `anthropic.TextBlockParam`，设置 `Text` 为稳定系统提示，并设置 `CacheControl` 为 `anthropic.CacheControlEphemeralParam{TTL: anthropic.CacheControlEphemeralTTLTTL5m}`。
- `req.System.Environment` 非空时创建第二个 `TextBlockParam`，不设置 `CacheControl`。
- 稳定块在环境块之前，保证缓存断点位于稳定前缀末端。
- 工具定义保持稳定排序，随请求的工具列表进入 Anthropic 请求；稳定系统块缓存标记使“工具 + 稳定系统块”前缀可复用。
- 流结束后从累计 message 的 usage 中读取 `InputTokens`、`OutputTokens`、`CacheCreationInputTokens`、`CacheReadInputTokens`，发出 `StreamEvent{Usage: ...}`。
- reminder 注入时，优先把 `req.Reminder` 作为文本块追加到最后一条 user 消息内容里；若最后一条不是 user，则追加一条新的 user 消息，避免破坏工具结果配对。

**OpenAI provider：**

- 构造单条 system message：`Stable` 在前，`Environment` 非空时追加两个换行和环境段。这样兼容 OpenAI 兼容端点对多条 system message 支持不一致的问题。
- `Stable` 始终位于 system message 开头，依赖端点前缀缓存尽力命中稳定部分。
- `req.Reminder` 非空时追加一条尾部 user message，不写入持久历史。
- 流结束后从 accumulator 或最终 choice 可用 usage 中读取输入、输出、总 token 和 `PromptTokensDetails.CachedTokens`；当前 streaming accumulator 若无法直接提供 usage，则保留测试可验证的解析辅助函数，并在 SDK 可提供字段时发送 `EventUsage`。
- OpenAI 不提供缓存写入字段，`CacheWrite` 保持 0。

### `agent` 包

**职责：** 在不改变 Agent Loop 控制流的前提下，负责本次 Run 的提示上下文准备、每轮 reminder 计算和 `llm.Request` 组装。

**改动点：**

- `Runner` 增加 `Version string` 字段。
- `run` 起始阶段计算：
  - `stableSystem := prompt.BuildSystemPrompt()`
  - `env := prompt.GatherEnvironment(r.Version, r.Provider.Name(), r.Provider.Model(), r.Env.CWD)`
  - `environment := env.Render()`
- 每轮迭代根据模式计算 reminder：
  - `ModePlan` 时使用 `prompt.PlanReminder(full)`
  - `full := iteration == 1 || (iteration-1)%planReminderInterval == 0`
  - `planReminderInterval` 使用包内常量，初始值为 4
  - 非计划模式为空 reminder
- 将 provider 调用改为 `r.Provider.Stream(ctx, llm.Request{Messages: req.Conversation.Messages(), Tools: defs, System: llm.System{Stable: stableSystem, Environment: environment}, Reminder: reminder})`。
- `prepareRequest` 仍决定用户消息、工具定义集合和工具执行限制；计划模式仍只暴露只读工具，执行模式和普通模式暴露全量工具。
- `streamCollector` 无需改事件类型，只保留并透传扩展后的 `llm.Usage`。

### `tools` 包

**职责：** 保持工具行为不变，仅强化描述文本。

**改动点：**

- `read_file` 描述保留其作为读取文本文件的专用工具定位。
- `find_files` 描述补充“找文件优先使用该工具，而不是运行 shell 查找命令”。
- `search_code` 描述补充“搜索文本或代码优先使用该工具，而不是运行 shell 搜索命令”。
- `edit_file` 描述补充“编辑前先用 `read_file` 读取目标文件，并确认替换文本唯一”。
- `write_file` 描述补充“覆盖写入前应已确认目标内容和用户改动风险”。
- `run_command` 描述补充“读取、找文件、搜索内容优先用专用工具；仅在需要构建、测试、验证或 shell 能力时使用命令”。

### `tui` 包

**职责：** 保持现有交互，向 Runner 提供版本。

**改动点：**

- 创建 `agent.Runner` 时设置 `Version: tui.Version`。
- provider 切换后保留 Version 字段。
- `usage` 仍使用 `*llm.Usage`；状态栏是否显示缓存字段不变，本章不新增展示。

### 调试与人工对比

**职责：** 验证缓存字段和提示行为，不建设自动评估平台。

**设计：**

- 在测试或后续轻量 smoke 命令中消费 `EventUsage` 并打印或断言 `input/output/cache_write/cache_read`。
- 在 `docs/Part_4/` 中准备人工对比场景文档，供旧提示与新提示定性比较。
- 场景覆盖只读规划、编辑前读取、优先工具选择、工具失败恢复、安全边界、输出风格、历史合法性、缓存观测。

## 模块交互

```text
Runner.Run
  ├─ BuildSystemPrompt()                       -> stable system
  ├─ GatherEnvironment(version, provider, model, cwd).Render()
  ├─ prepareRequest(mode)                       -> user text, tool defs, tool safety
  └─ for each iteration
       ├─ PlanReminder(full(iter)) or ""
       ├─ Conversation.Messages()
       ├─ Provider.Stream(ctx, llm.Request{...})
       ├─ streamCollector publishes TextDelta / Usage
       ├─ append assistant text and tool calls to history
       ├─ execute tool calls
       └─ append ordered tool results to history
```

Provider 装配：

```text
Anthropic:
  tools + system[stable with cache_control, environment without cache_control] + messages(+reminder)

OpenAI:
  system message: stable + "\n\n" + environment
  messages + trailing user reminder
  tools
```

## 文件组织

```text
PseudoClaude/
├── internal/prompt/
│   ├── prompt.go          — 保留 banner；新增 BuildSystemPrompt 或迁移旧 SystemPrompt
│   ├── modules.go         — 新增 Module、FixedModules、OptionalModules、AssembleSystem
│   ├── environment.go     — 新增 Environment、GatherEnvironment、Render
│   ├── reminder.go        — 新增 SystemReminder、PlanReminder
│   └── prompt_test.go     — 扩展模块顺序、跳空槽、稳定性、环境渲染、reminder 测试
├── internal/llm/
│   ├── provider.go        — 新增 Request/System；Usage 增加缓存字段；Provider.Stream 签名变更
│   ├── anthropic.go       — 改为从 Request 装配系统块、缓存控制、reminder 和 usage
│   ├── anthropic_test.go  — 增加系统块、reminder、缓存字段解析测试
│   ├── openai.go          — 改为从 Request 装配 system/reminder；解析 cached tokens
│   └── openai_test.go     — 增加 system/reminder 装配和 cached tokens 解析测试
├── internal/agent/
│   ├── runner.go          — 采集环境、构造 stable/env/reminder、调用新 llm.Request
│   ├── collector.go       — 保持用量透传，必要时适配 int64
│   └── runner_test.go     — 增加 Request 装配、计划 reminder 频率、缓存用量透传测试
├── internal/tools/
│   ├── file.go            — 强化 read/write/edit 文件工具描述
│   ├── search.go          — 强化 find/search 工具描述
│   ├── command.go         — 强化 run_command 描述
│   └── registry_test.go   — 断言工具顺序稳定和关键描述存在
├── internal/tui/
│   ├── tui.go             — Runner 设置 Version
│   └── select.go/stream.go — 如 provider 切换处需要，保持 Runner.Version
└── docs/Part_4/
    ├── spec.md
    ├── plan.md
    └── evaluation.md      — 新增人工对比场景文档
```

## 技术决策

| 决策点 | 选择 | 理由 |
| ------ | ---- | ---- |
| 系统提示组织 | `Module{Name, Priority, Content}` + `AssembleSystem` | 满足模块化与可扩展需求；优先级排序保证稳定顺序。 |
| 可选模块 | 以空 `Content` 预留，自定义指令、Skill、长期记忆暂不接真实来源 | 满足本章边界，同时给后续章节留下挂载点。 |
| 环境信息位置 | 独立系统级内容块，由 provider 放在稳定系统块之后 | 模型能感知环境，同时不污染稳定块。 |
| Anthropic 缓存 | 稳定 system 文本块设置 `CacheControlEphemeralParam{TTL: TTL5m}`，环境块不设置 | 当前 SDK 支持 TextBlock cache control；断点放在稳定块末尾，使环境变化不影响稳定前缀。 |
| Anthropic 工具缓存 | 不单独给每个工具设置 cache control | 工具定义顺序稳定，稳定 system 块的缓存断点已覆盖此前前缀；避免过多断点和实现复杂度。 |
| OpenAI 缓存 | 稳定文本位于单条 system message 前缀，环境拼在后面 | 兼容 OpenAI-compatible base URL；前缀缓存尽力而为，不强制端点支持。 |
| Provider 入参 | `Stream(ctx, llm.Request)` | 系统块、工具、历史和 reminder 都在一个结构体里，后续扩展不会继续拉长参数列表。 |
| prompt 与 llm 依赖 | agent 负责把 prompt 产物传给 llm，llm 不 import prompt | 职责清晰，避免环依赖。 |
| reminder 标签 | `<system-reminder>...</system-reminder>` | 可被模型识别为系统补充上下文，并便于测试断言。 |
| reminder 持久化 | 不写入 `conversation.Conversation` | 每轮动态构造，不污染历史和缓存。 |
| Anthropic reminder 注入 | 优先并入最后一条 user 消息内容块，必要时追加 user 消息 | 尽量保持 Anthropic 消息角色合法，并避免工具结果配对被打断。 |
| OpenAI reminder 注入 | 追加尾部 user 消息 | Chat Completions 可接受尾部用户补充；不影响持久历史。 |
| 计划提醒频率 | `iteration == 1 || (iteration-1)%4 == 0` 为完整，其余精简 | 实现首轮完整、间隔重复、其余精简；频率集中在常量里。 |
| 缓存字段 | `Usage.CacheWrite` / `Usage.CacheRead` | Anthropic 和 OpenAI 字段映射到统一事件模型，provider 缺字段时零值。 |
| 环境采集 git 状态 | 短超时 `git status --porcelain`，失败降级 | 满足上下文需求，又不阻塞界面。 |
| 敏感信息 | 不读取环境变量，不渲染 provider API key | 避免把凭据带入提示或调试输出。 |
| 缓存展示 | 不进入 TUI 状态栏，只通过事件、测试或 smoke/debug 输出观察 | 符合 spec 边界，减少界面改动。 |

## Spec 覆盖关系

| Spec | Plan 归属 |
| ---- | --------- |
| F1 | `prompt.Module`、`FixedModules`、`AssembleSystem` |
| F2 | `OptionalModules` 与空内容跳过 |
| F3 | `prompt.Environment`、`GatherEnvironment`、`Render` |
| F4 | `llm.System`、`llm.Request`、provider 稳定块和环境块分离 |
| F5 | Anthropic/OpenAI provider 缓存装配策略 |
| F6 | `llm.Usage.CacheWrite/CacheRead` 与 provider 字段解析 |
| F7 | `prompt` 工具使用模块和 `tools` 描述强化 |
| F8 | `tools.Registry.Definitions` 稳定排序和工具描述不含动态内容 |
| F9 | `SystemReminder`、`PlanReminder`、`llm.Request.Reminder` |
| F10 | provider reminder 注入策略与 conversation 不持久化 |
| F11 | agent 按 iteration 计算计划模式 reminder |
| F12 | 两个 provider 共享 `llm.Request` 语义，各自适配协议 |
| F13 | `GatherEnvironment` 降级和敏感信息边界 |
| F14 | `docs/Part_4/evaluation.md` |
| F15 | agent/provider/TUI 兼容现有事件和历史流程 |
