# 系统提示工程化 Tasks

## 文件清单

| 操作 | 文件 | 职责 |
| ---- | ---- | ---- |
| 修改 | `internal/prompt/prompt.go` | 保留 banner；移除旧短系统提示常量；提供 `BuildSystemPrompt` 入口 |
| 新建 | `internal/prompt/modules.go` | `Module` 类型、固定模块、可选空槽、系统提示装配 |
| 新建 | `internal/prompt/environment.go` | 环境采集、git 状态降级、环境段渲染 |
| 新建 | `internal/prompt/reminder.go` | `<system-reminder>` 包裹、计划模式完整/精简提醒 |
| 修改 | `internal/prompt/prompt_test.go` | 模块顺序、跳空槽、稳定性、环境、reminder 测试 |
| 修改 | `internal/tools/file.go` | 强化 `read_file`、`write_file`、`edit_file` 描述 |
| 修改 | `internal/tools/search.go` | 强化 `find_files`、`search_code` 描述 |
| 修改 | `internal/tools/command.go` | 强化 `run_command` 描述 |
| 修改 | `internal/tools/registry_test.go` | 断言工具定义顺序稳定、关键描述存在 |
| 修改 | `internal/llm/provider.go` | 新增 `System`、`Request`；扩展 `Usage`；修改 `Provider.Stream` 签名 |
| 修改 | `internal/llm/anthropic.go` | 按 `Request` 装配系统块、缓存控制、reminder、缓存用量 |
| 修改 | `internal/llm/anthropic_test.go` | Anthropic system/reminder/cache usage 装配测试 |
| 修改 | `internal/llm/openai.go` | 按 `Request` 装配 system/reminder/tools，解析 cached tokens |
| 修改 | `internal/llm/openai_test.go` | OpenAI system/reminder/cached tokens 装配测试 |
| 修改 | `internal/agent/runner.go` | 采集环境、构造稳定提示、按轮次 reminder、组装 `llm.Request` |
| 修改 | `internal/agent/collector.go` | 适配扩展后的 `llm.Usage` 透传 |
| 修改 | `internal/agent/runner_test.go` | fake provider 新签名、请求装配、计划提醒频率、缓存用量透传 |
| 修改 | `internal/tui/tui.go` | 初始化 Runner 时传入应用版本 |
| 修改 | `internal/tui/select.go` | provider 切换后保持 Runner 版本与 provider |
| 新建 | `docs/Part_4/evaluation.md` | 人工对比场景：输入、观察点、预期差异 |

## T1: 新增提示模块模型与装配

**文件：** `internal/prompt/modules.go`、`internal/prompt/prompt.go`
**依赖：** 无

**步骤：**

1. 在 `modules.go` 定义 `Module`，包含 `Name`、`Priority`、`Content` 字段。
2. 定义固定模块优先级常量：身份 10、系统约束 20、任务模式 30、动作执行 40、工具使用 50、语气风格 60、文本输出 70。
3. 实现 `FixedModules() []Module`，返回七个固定模块，内容以内置英文提示为主，并使用 PseudoClaude 命名。
4. 在“工具使用”模块中写入关键规则：优先使用 `read_file`、`find_files`、`search_code`；编辑前先读取；需要构建、测试或 shell 能力时才用 `run_command`。
5. 实现 `OptionalModules() []Module`，返回自定义指令、已激活 Skill、长期记忆三个空内容模块，优先级为 80、90、100。
6. 实现 `AssembleSystem(mods []Module) string`：复制入参后按 `Priority` 升序稳定排序，跳过空白内容，用 `"\n\n"` 连接。
7. 在 `prompt.go` 中用 `BuildSystemPrompt()` 替代旧 `SystemPrompt` 常量；保留 banner、logo 和响应式 banner 逻辑。

**验证：** `go test ./internal/prompt/...` 通过，且新增测试能看到七个固定模块按顺序出现。

## T2: 新增环境采集与渲染

**文件：** `internal/prompt/environment.go`
**依赖：** 无

**步骤：**

1. 定义 `Environment`，包含 `WorkingDir`、`Platform`、`Date`、`GitStatus`、`Version`、`Provider`、`Model`。
2. 实现 `GatherEnvironment(version, provider, model, cwd string) Environment`。
3. 当 `cwd` 为空时回退到 `os.Getwd()`；失败则留空或标注不可用。
4. 使用 `runtime.GOOS` 或等价平台信息填充 `Platform`。
5. 使用 `time.Now().Format("2006-01-02")` 填充 `Date`。
6. 使用短超时执行 `git status --porcelain`，工作目录为 `cwd`；非 git 目录、git 不可用、超时或非零退出时 `GitStatus` 留空或标注不可用。
7. 将 git 输出渲染为短摘要，例如“clean”或“N changed files”；避免输出大量文件列表。
8. 实现 `(Environment) Render() string`，输出带明确标题的环境信息段，空值字段省略或显示不可用。
9. 确保环境采集不读取环境变量，不包含 API key 或 token。

**验证：** `go test ./internal/prompt/...` 通过；环境测试在临时非 git 目录中不 panic，且 `Render()` 包含 cwd/platform/date/provider/model。

## T3: 新增补充消息与计划模式提醒

**文件：** `internal/prompt/reminder.go`
**依赖：** 无

**步骤：**

1. 实现 `SystemReminder(body string) string`，用 `<system-reminder>` 与 `</system-reminder>` 包裹正文。
2. 空白正文传入时返回空字符串，避免注入空标签。
3. 定义完整计划模式提醒文本，覆盖只读工具、先调研、产出计划、不得写文件或运行有副作用命令、等待执行模式。
4. 定义精简计划模式提醒文本，覆盖“仍在计划模式、只读工具、不要执行改动”。
5. 实现 `PlanReminder(full bool) string`，返回已包裹标签的完整或精简提醒。

**验证：** `go test ./internal/prompt/...` 通过；测试断言完整提醒和精简提醒都带标签，且正文不同。

## T4: 扩展 prompt 单元测试

**文件：** `internal/prompt/prompt_test.go`
**依赖：** T1、T2、T3

**步骤：**

1. 增加 `BuildSystemPrompt` 顺序测试，断言七个固定模块按身份到文本输出排列。
2. 增加空槽跳过测试，传入包含空内容模块的切片，断言输出没有该模块，也没有连续三段以上空行。
3. 增加稳定性测试，连续两次调用 `BuildSystemPrompt()` 结果完全相等。
4. 增加双重强化基础测试，断言稳定提示中包含 `read_file`、`find_files`、`search_code`、`edit`/`read before edit` 等关键语义。
5. 增加环境渲染测试，构造固定 `Environment`，断言 `Render()` 包含关键字段且不包含空字段。
6. 增加非 git 目录采集测试，使用临时目录调用 `GatherEnvironment`，断言正常返回。
7. 增加 reminder 测试，断言 `SystemReminder` 标签和空正文行为。

**验证：** `go test ./internal/prompt/...` 通过。

## T5: 强化工具描述并验证稳定顺序

**文件：** `internal/tools/file.go`、`internal/tools/search.go`、`internal/tools/command.go`、`internal/tools/registry_test.go`
**依赖：** 无

**步骤：**

1. 更新 `read_file` 描述，明确它是读取本地文本文件的专用工具。
2. 更新 `write_file` 描述，提醒覆盖写入前应确认目标内容与用户改动风险。
3. 更新 `edit_file` 描述，明确编辑前应先用 `read_file` 读取目标文件，并确认 `old_text` 唯一。
4. 更新 `find_files` 描述，明确找文件优先使用该工具，而不是用命令拼凑。
5. 更新 `search_code` 描述，明确搜索代码或文本优先使用该工具，而不是用命令拼凑。
6. 更新 `run_command` 描述，明确读文件、找文件、搜内容优先使用专用工具；命令用于构建、测试、验证或确需 shell 的操作。
7. 不修改工具名称、schema、执行逻辑或 safety。
8. 在 registry 测试中断言 `Definitions()` 顺序稳定，并断言关键工具描述包含强化文本。

**验证：** `go test ./internal/tools/...` 通过。

## T6: 改造 llm Provider 接口

**文件：** `internal/llm/provider.go`
**依赖：** 无

**步骤：**

1. 新增 `System`，包含 `Stable`、`Environment` 字段。
2. 新增 `Request`，包含 `Messages []Message`、`Tools []tools.Definition`、`System System`、`Reminder string`。
3. 将 `Usage` 的 token 字段类型统一为 `int64`，并新增 `CacheWrite`、`CacheRead`。
4. 将 `Provider.Stream` 签名改为 `Stream(ctx context.Context, req Request) <-chan StreamEvent`。
5. 保留 `Name()`、`Model()` 和 `New()` 语义不变。
6. 调整受影响的编译错误位置，先让 provider 实现文件在后续任务中适配。

**验证：** 暂不要求包编译通过；本任务完成后 `rg -n "Stream\\(ctx context.Context, msgs|Stream\\(_ context.Context, msgs" internal` 不再出现旧接口定义。

## T7: 适配 Anthropic 请求装配、缓存与 reminder

**文件：** `internal/llm/anthropic.go`、`internal/llm/anthropic_test.go`
**依赖：** T6

**步骤：**

1. 将 `anthropicProvider.Stream` 改为接收 `llm.Request`。
2. 用 `req.Tools` 调用 `toAnthropicTools`。
3. 新增 `toAnthropicSystem(sys System) []anthropic.TextBlockParam` 辅助函数。
4. 当 `sys.Stable` 非空时，创建带 `CacheControl: anthropic.CacheControlEphemeralParam{TTL: anthropic.CacheControlEphemeralTTLTTL5m}` 的 system text block。
5. 当 `sys.Environment` 非空时，创建不带 cache control 的第二个 system text block。
6. 用 `req.Messages` 调用 `toAnthropicMessages`。
7. 新增 `appendAnthropicReminder(msgs []anthropic.MessageParam, reminder string) []anthropic.MessageParam`。
8. reminder 非空时，优先追加到最后一条 user 消息的 content；最后一条不是 user 时追加新的 user message。
9. 流结束后从累计 message usage 中发送 `StreamEvent{Usage: &Usage{InputTokens, OutputTokens, TotalTokens, CacheWrite, CacheRead}}`，其中 `TotalTokens` 至少包含输入、输出、缓存写、缓存读之和或 SDK 总量可得值。
10. 保持 thinking、文本 delta、tool use 解析和 Done 行为不变。
11. 增加测试覆盖 system stable cache control、environment 无 cache control、reminder 注入、cache usage 映射。

**验证：** `go test ./internal/llm/...` 在 T8 完成后通过；本任务局部可运行 `go test ./internal/llm -run Anthropic`。

## T8: 适配 OpenAI 请求装配、缓存与 reminder

**文件：** `internal/llm/openai.go`、`internal/llm/openai_test.go`
**依赖：** T6

**步骤：**

1. 将 `openAIProvider.Stream` 改为接收 `llm.Request`。
2. 用 `req.Tools` 调用 `toOpenAITools`。
3. 新增 `toOpenAIMessages(req Request) []openai.ChatCompletionMessageParamUnion` 或更新现有函数接收 `Request`。
4. 构造单条 system message，内容为 `req.System.Stable`，当 `Environment` 非空时追加 `"\n\n"` 和环境段。
5. 按现有逻辑追加持久历史消息。
6. 当 `req.Reminder` 非空时，追加尾部 `openai.UserMessage(req.Reminder)`。
7. 新增 `openAIUsageFromCompletionUsage` 或等价辅助函数，将 `PromptTokens`、`CompletionTokens`、`TotalTokens`、`PromptTokensDetails.CachedTokens` 映射到 `llm.Usage`。
8. 在 streaming accumulator 可取得 usage 时发送 `StreamEvent{Usage: ...}`；若 SDK 流式 accumulator 暂无 usage，也保留辅助函数单测，运行时缺字段保持零值。
9. 保持文本 delta、tool call 累积和 Done 行为不变。
10. 增加测试覆盖 system + environment 拼接、reminder 尾部注入、cached tokens 映射。

**验证：** `go test ./internal/llm/...` 通过。

## T9: 改造 Agent Runner 请求构造

**文件：** `internal/agent/runner.go`、`internal/agent/collector.go`
**依赖：** T1、T2、T3、T6、T7、T8

**步骤：**

1. 在 `Runner` 增加 `Version string` 字段。
2. 新增 `const planReminderInterval = 4`。
3. 在 `run` 开始且 provider 非空后，构造 `stableSystem := prompt.BuildSystemPrompt()`。
4. 在 `run` 开始构造环境：`env := prompt.GatherEnvironment(r.Version, r.Provider.Name(), r.Provider.Model(), r.Env.CWD)`。
5. 将 `env.Render()` 保存为本次 Run 的环境段，避免同一 Run 内每轮重复采集。
6. 保留 `prepareRequest` 的用户消息和工具选择逻辑，但移除计划模式系统后缀概念；计划模式规则由 reminder 承载。
7. 每轮迭代调用 `reminderForMode(req.Mode, iteration)` 或等价辅助函数。
8. `ModePlan` 下 `iteration == 1 || (iteration-1)%planReminderInterval == 0` 使用完整提醒，其余轮次使用精简提醒。
9. 非计划模式 reminder 为空。
10. 将 provider 调用改为 `r.Provider.Stream(ctx, llm.Request{Messages: req.Conversation.Messages(), Tools: defs, System: llm.System{Stable: stableSystem, Environment: environmentText}, Reminder: reminder})`。
11. `collector.go` 保持事件透传，适配 `llm.Usage` 字段类型从 `int` 到 `int64` 的变化。

**验证：** `go test ./internal/agent/...` 在 T10 完成后通过。

## T10: 适配 Agent 单元测试

**文件：** `internal/agent/runner_test.go`
**依赖：** T9

**步骤：**

1. 修改 `fakeProvider.Stream` 签名为 `Stream(ctx context.Context, req llm.Request)`。
2. 在 fake provider 中记录每次收到的完整 `llm.Request`，包括 `Messages`、`Tools`、`System`、`Reminder`。
3. 适配现有多轮工具、工具顺序、计划模式只读、最大迭代、未知工具、stream error 测试。
4. 新增断言：普通模式和计划模式收到的 `System.Stable` 非空，且稳定提示一致。
5. 新增断言：`System.Environment` 非空，并包含版本、provider、model 或 cwd 的可观察字段。
6. 新增计划 reminder 频率测试：构造至少两轮或五轮 provider 响应，断言第 1 轮完整，第 2 轮精简，第 5 轮完整。
7. 新增断言：计划模式 `Tools` 仅包含只读工具，执行模式包含全量工具。
8. 新增断言：reminder 不写入 `conversation.Conversation` 持久历史。
9. 新增缓存用量透传测试：fake provider 发出 `llm.Usage{CacheWrite: 11, CacheRead: 22}`，断言 `EventUsage` 携带相同值。

**验证：** `go test ./internal/agent/...` 通过。

## T11: 适配 TUI Runner 初始化

**文件：** `internal/tui/tui.go`、`internal/tui/select.go`
**依赖：** T9

**步骤：**

1. 在 `tui.New` 初始化 `agent.Runner` 时设置 `Version: Version`。
2. provider 选择完成后继续设置 `m.runner.Provider = provider`，并确保 `Version` 字段不被覆盖。
3. `submit` 中刷新 runner provider/registry/env 时不清空 `Version`。
4. 确认 TUI 不新增缓存字段展示。

**验证：** `go test ./internal/tui/...` 通过。

## T12: 新增人工对比场景文档

**文件：** `docs/Part_4/evaluation.md`
**依赖：** docs/Part_4/spec.md、docs/Part_4/plan.md

**步骤：**

1. 创建文档标题和说明，明确这是人工定性对比，不是自动评测框架。
2. 增加“只读规划”场景，包含输入、观察点、旧提示可能表现、新提示预期表现。
3. 增加“编辑前读取”场景。
4. 增加“优先工具选择”场景。
5. 增加“工具失败后继续推理”场景。
6. 增加“安全边界”场景。
7. 增加“输出风格”场景。
8. 增加“历史合法性”场景。
9. 增加“缓存观测”场景。
10. 每个场景都写明可观察证据，例如工具调用顺序、是否出现内部标签、usage cache 字段、是否请求副作用工具。

**验证：** `rg -n "只读规划|编辑前读取|优先工具选择|工具失败|安全边界|输出风格|历史合法性|缓存观测" docs/Part_4/evaluation.md` 能找到全部场景。

## T13: 全量格式化、静态检查与测试

**文件：** 全项目
**依赖：** T1-T12

**步骤：**

1. 运行 `gofmt` 格式化改动过的 Go 文件。
2. 运行 `go test ./...`。
3. 运行 `go vet ./...`。
4. 运行 `go test -race ./internal/agent/... ./internal/tools/...`。
5. 搜索 `api_key`、`API_KEY`、`token` 等敏感词，确认新增环境渲染和调试输出没有引入凭据展示。
6. 若 `go vet` 或 race 在当前环境不可用，记录实际失败原因和已完成的替代验证。

**验证：** 上述命令通过，或在受限环境下给出具体失败原因与已通过命令输出。

## 执行顺序

```text
T1 ─┐
T2 ─┼─→ T4
T3 ─┘

T5

T6 ─┬─→ T7 ─┐
    └─→ T8 ─┼─→ T9 ─→ T10 ─→ T11

T12

T1-T12 ─→ T13
```
