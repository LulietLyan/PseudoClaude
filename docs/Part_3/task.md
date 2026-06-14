# Agent Loop Tasks

## 文件清单

| 操作 | 文件 | 职责 |
| ---- | ---- | ---- |
| 新建 | `internal/agent/event.go` | Agent 事件、停止原因、模式等公共类型 |
| 新建 | `internal/agent/collector.go` | 单轮 provider stream 的双路收集 |
| 新建 | `internal/agent/tools.go` | 工具分批、并发/串行执行、结果排序 |
| 新建 | `internal/agent/runner.go` | ReAct 主循环、停止条件、Plan Mode 请求构造 |
| 新建 | `internal/agent/runner_test.go` | Agent Loop、停止条件、Plan Mode、分批行为测试 |
| 修改 | `internal/tools/tool.go` | 增加工具安全性元数据 |
| 修改 | `internal/tools/file.go` | 为文件工具标注安全性 |
| 修改 | `internal/tools/search.go` | 为搜索工具标注安全性 |
| 修改 | `internal/tools/command.go` | 为命令工具标注安全性 |
| 修改 | `internal/tools/registry.go` | 增加只读过滤、已知工具和安全性查询 |
| 修改 | `internal/tools/registry_test.go` | 覆盖安全性过滤与查询 |
| 修改 | `internal/llm/provider.go` | 增加 token 用量结构，扩展 stream event |
| 修改 | `internal/llm/anthropic.go` | 转发 provider 可得用量，保持多工具事件 |
| 修改 | `internal/llm/openai.go` | 支持一轮多个工具调用与可选用量事件 |
| 修改 | `internal/llm/*_test.go` | 更新 stream event 结构相关断言 |
| 修改 | `internal/conversation/conversation.go` | 增加同一轮多工具调用追加 helper |
| 修改 | `internal/conversation/conversation_test.go` | 覆盖多工具调用深拷贝和顺序 |
| 修改 | `internal/tui/tui.go` | Model 字段切换为 Agent Runner 与 Agent 事件 |
| 修改 | `internal/tui/stream.go` | 提交逻辑、Agent 事件消费、`/plan`、`/do` |
| 修改 | `internal/tui/view.go` | 展示 Agent 进度、停止原因、用量、工具状态 |
| 修改 | `internal/tui/stream_test.go` | 更新 TUI 测试，验证事件消费和 Plan Mode |
| 修改 | `cmd/PseudoClaude/main.go` | 如构造函数签名需要变化，同步入口装配 |

## T1: 扩展工具安全性元数据

**文件：** `internal/tools/tool.go`、`internal/tools/file.go`、`internal/tools/search.go`、`internal/tools/command.go`

**依赖：** 无

**步骤：**
1. 在工具包中定义安全性枚举，包含只读和有副作用两类。
2. 在工具定义结构中增加安全性字段。
3. 为 `read_file`、`find_files`、`search_code` 标注只读。
4. 为 `write_file`、`edit_file`、`run_command` 标注有副作用。
5. 确保原有工具名称、描述和参数 schema 不变。

**验证：** 运行 `go test ./internal/tools`，期望工具现有测试仍通过。

## T2: 增加工具注册中心过滤与查询

**文件：** `internal/tools/registry.go`、`internal/tools/registry_test.go`

**依赖：** T1

**步骤：**
1. 增加按安全性过滤工具定义的接口，返回顺序与现有定义列表一样稳定。
2. 增加按工具名查询安全性的接口。
3. 增加判断工具是否已注册的接口。
4. 为只读过滤写测试，期望只返回 `read_file`、`find_files`、`search_code`。
5. 为未知工具查询写测试，期望返回不存在或 false。

**验证：** 运行 `go test ./internal/tools`，期望新增和既有测试全部通过。

## T3: 扩展 LLM 流式事件用量字段

**文件：** `internal/llm/provider.go`

**依赖：** 无

**步骤：**
1. 定义协议无关的 token 用量结构，包含输入、输出和总 token 数。
2. 在流式事件结构中增加可选用量字段。
3. 保持现有文本、工具调用、完成、错误字段的语义不变。
4. 确认 provider 接口签名不需要变化。

**验证：** 运行 `go test ./internal/llm`，期望编译通过；若 provider 测试因结构字段变化失败，记录到后续任务修复。

## T4: 更新 Anthropic Provider 事件适配

**文件：** `internal/llm/anthropic.go`、`internal/llm/anthropic_test.go`

**依赖：** T3

**步骤：**
1. 在不改变文本流式行为的前提下，保留一轮结束后扫描所有工具调用并逐个发出事件的行为。
2. 如果 SDK stream 或最终 message 中能取得 token 用量，将其转换为通用用量事件。
3. 如果无法稳定取得用量，不发送用量事件，保持任务可正常完成。
4. 补充或更新测试，确认多工具调用事件不会被丢弃。

**验证：** 运行 `go test ./internal/llm -run Anthropic`，期望 Anthropic 相关测试通过。

## T5: 更新 OpenAI Provider 多工具事件适配

**文件：** `internal/llm/openai.go`、`internal/llm/openai_test.go`

**依赖：** T3

**步骤：**
1. 移除固定禁止模型返回并行工具调用的请求设置。
2. 确保 stream accumulator 可以在一轮中发出多个完整工具调用事件。
3. 保留文本增量实时事件。
4. 如果 SDK stream 或最终 completion 中能取得 token 用量，将其转换为通用用量事件。
5. 补充或更新测试，确认一轮多个工具调用都能被上层观察到。

**验证：** 运行 `go test ./internal/llm -run OpenAI`，期望 OpenAI 相关测试通过。

## T6: 增加多工具调用历史追加能力

**文件：** `internal/conversation/conversation.go`、`internal/conversation/conversation_test.go`

**依赖：** 无

**步骤：**
1. 增加一次追加多个 assistant 工具调用的 helper。
2. 让原有单工具调用 helper 复用多工具 helper 或保持兼容行为。
3. 确保多个工具调用保存在同一个 assistant 消息中，顺序与传入顺序一致。
4. 更新深拷贝测试，修改返回切片不影响内部历史。

**验证：** 运行 `go test ./internal/conversation`，期望全部测试通过。

## T7: 创建 Agent 事件与停止类型

**文件：** `internal/agent/event.go`

**依赖：** T3

**步骤：**
1. 定义运行模式：普通、计划、执行。
2. 定义事件类型：进度、文本增量、完整助手文本、工具调用开始、工具调用完成、工具结果、用量、停止、错误。
3. 定义停止原因：完成、迭代上限、取消、未知工具连续上限、流式错误。
4. 定义事件结构，包含轮次、文本、说明、工具调用、工具结果、用量、停止信息和错误。
5. 定义工具结果包装结构，绑定工具调用、结构化结果和耗时。

**验证：** 运行 `go test ./internal/agent`，期望包可编译；此时若无测试，至少编译通过。

## T8: 实现流式双路收集器

**文件：** `internal/agent/collector.go`、`internal/agent/runner_test.go`

**依赖：** T7

**步骤：**
1. 实现 collector 消费一轮 `llm.StreamEvent`。
2. 收到文本增量时立即向 Agent 事件流发布文本事件。
3. 同时将文本增量累积为完整 assistant 文本。
4. 收集同一轮所有工具调用事件。
5. 收集最后一次或合并后的 token 用量事件。
6. 遇到 provider 错误时返回错误，交给 Runner 映射停止原因。
7. 写测试模拟文本分片、多个工具调用、用量和 done，验证实时事件和完整输出一致。

**验证：** 运行 `go test ./internal/agent -run Collector`，期望 collector 测试通过。

## T9: 实现工具分批与执行器

**文件：** `internal/agent/tools.go`、`internal/agent/runner_test.go`

**依赖：** T2、T7

**步骤：**
1. 按模型请求顺序把工具调用切分为批次。
2. 连续只读且已知的工具放入同一个并发批次。
3. 有副作用工具各自形成串行批次。
4. 未知工具形成串行批次，仍通过注册中心执行以获得结构化错误。
5. 并发执行时使用原始索引回填结果，保证最终结果顺序与模型请求顺序一致。
6. 每个工具执行前发布工具调用开始事件，完成后发布工具结果事件。
7. 写测试构造慢只读工具和快只读工具，验证并发完成顺序不同但回灌顺序稳定。
8. 写测试构造两个副作用工具，验证按顺序串行执行。

**验证：** 运行 `go test ./internal/agent -run Tool`，期望分批和执行器测试通过。

## T10: 实现 Agent Runner 基础循环

**文件：** `internal/agent/runner.go`、`internal/agent/runner_test.go`

**依赖：** T6、T8、T9

**步骤：**
1. 实现 Runner 默认配置归一化。
2. 实现 `Run` 创建事件 channel 并启动 goroutine。
3. 普通模式下追加用户消息，选择全部工具定义。
4. 每轮请求 provider stream，并用 collector 获取完整文本、工具调用和用量。
5. 有完整文本时追加 assistant 文本历史，并发布完整文本事件。
6. 没有工具调用时发布完成停止事件并关闭 channel。
7. 有工具调用时追加 assistant 多工具调用历史，执行工具批次，按顺序追加工具结果历史，然后进入下一轮。
8. 写 fake provider，验证两轮工具调用后第三轮最终回答能自动完成。

**验证：** 运行 `go test ./internal/agent -run RunnerCompletes`，期望基础循环测试通过。

## T11: 实现停止条件

**文件：** `internal/agent/runner.go`、`internal/agent/runner_test.go`

**依赖：** T10

**步骤：**
1. 在每轮 provider 请求前检查最大迭代次数。
2. 在 collector、工具执行前后检查 context 取消。
3. provider stream 错误时发布错误事件和流式错误停止事件。
4. 统计连续未知工具调用次数，成功执行已知工具或最终回答时清零。
5. 连续未知工具达到配置限制时，先回灌当前未知工具错误结果，再发布停止事件。
6. 为迭代上限、取消、流式错误、未知工具连续限制分别写测试。

**验证：** 运行 `go test ./internal/agent -run Stop`，期望停止条件测试通过。

## T12: 实现 Plan Mode 请求构造

**文件：** `internal/agent/runner.go`、`internal/agent/runner_test.go`

**依赖：** T10

**步骤：**
1. 计划模式下将用户任务包装为“只制定计划，可使用只读工具收集信息”的请求文本。
2. 计划模式只传入只读工具定义。
3. 执行模式下将原始任务和计划文本包装为执行请求文本。
4. 执行模式传入全部工具定义。
5. 写测试确认 `/plan` 模式 provider 收到的工具定义只包含只读工具。
6. 写测试确认 `/do` 模式 provider 收到全部工具，并且消息中包含原任务和计划文本。

**验证：** 运行 `go test ./internal/agent -run Plan`，期望 Plan Mode 测试通过。

## T13: 接入 TUI Agent Runner 字段

**文件：** `internal/tui/tui.go`

**依赖：** T7、T10

**步骤：**
1. 将 Model 中的 provider stream channel 替换为 Agent 事件 channel。
2. 增加 Runner 或 Runner 构造所需字段。
3. 保留 provider、registry、conversation、cwd 和 tool env。
4. 增加当前进度、当前用量、最近停止原因和最近计划状态字段。
5. 删除或停用 TUI 自己执行工具所需的状态字段，只保留展示所需状态。

**验证：** 运行 `go test ./internal/tui -run TestNew`，期望构造和基础输入测试通过。

## T14: 改造 TUI 提交与命令解析

**文件：** `internal/tui/stream.go`、`internal/tui/stream_test.go`

**依赖：** T12、T13

**步骤：**
1. 普通输入构造 `ModeChat` 请求并启动 Agent Run。
2. `/plan <任务>` 构造 `ModePlan` 请求；任务为空时显示提示，不启动 Run。
3. `/do` 在存在最近成功计划时构造 `ModeDo` 请求；没有计划时显示提示。
4. 执行模式完成后清空最近计划。
5. 保留 `/exit` 退出行为。
6. 更新测试，使用 fake runner 或 fake provider 事件驱动 TUI 状态变化。

**验证：** 运行 `go test ./internal/tui -run Plan` 和 `go test ./internal/tui -run Submit`，期望提交与命令解析测试通过。

## T15: 改造 TUI Agent 事件消费

**文件：** `internal/tui/stream.go`、`internal/tui/view.go`、`internal/tui/stream_test.go`

**依赖：** T14

**步骤：**
1. 将等待事件命令改为读取 `agent.Event`。
2. 文本增量事件继续写入当前流式回复。
3. 完整 assistant 文本事件用于打印 markdown 定型块。
4. 工具调用开始和工具结果事件更新当前工具展示，并打印工具状态块。
5. 进度事件更新当前状态说明。
6. 用量事件更新当前用量展示。
7. 停止事件使界面回到 idle，并根据停止原因显示完成、取消、上限或错误状态。
8. 错误事件显示错误块，但最终状态仍由停止事件收束。

**验证：** 运行 `go test ./internal/tui -run Event`，期望 TUI 能按事件转换状态并恢复 idle。

## T16: 更新 TUI 展示组件

**文件：** `internal/tui/view.go`

**依赖：** T15

**步骤：**
1. 在流式视图中展示当前轮次或进度说明。
2. 在工具状态中支持多个工具事件的简洁展示。
3. 在完成块或状态栏中展示 token 用量；用量不可用时不显示。
4. 为迭代上限、取消、未知工具连续上限、流式错误提供可读停止文案。
5. 保持普通文本聊天的既有显示风格。

**验证：** 运行 `go test ./internal/tui`，期望所有 TUI 测试通过。

## T17: 同步入口装配

**文件：** `cmd/PseudoClaude/main.go`

**依赖：** T13

**步骤：**
1. 检查 TUI 构造函数签名是否变化。
2. 如有变化，同步 main 中 provider、cwd、registry 的传入方式。
3. 确认入口不直接感知 Agent Loop 细节。

**验证：** 运行 `go test ./cmd/PseudoClaude ./internal/tui`，期望编译通过。

## T18: 跑全量测试并修正集成问题

**文件：** 所有受影响 Go 文件

**依赖：** T1-T17

**步骤：**
1. 运行全量测试。
2. 修复因为接口变更导致的编译错误。
3. 修复因为 TUI 行为变化导致的测试断言。
4. 确认普通聊天、单工具、多工具、Plan Mode 相关测试都在同一轮全量测试中通过。

**验证：** 运行 `go test ./...`，期望全部测试通过。

## T19: 手动验证普通 Agent Loop 场景

**文件：** 无固定文件，使用本地构建产物或 `go run`

**依赖：** T18

**步骤：**
1. 启动程序。
2. 输入一个需要读文件后继续回答的任务。
3. 观察文本增量、工具调用、工具结果、下一轮模型请求和最终回答是否都在一次用户提交内完成。
4. 观察完成后输入框是否恢复。

**验证：** 手动场景中无需再次输入“继续”，Agent 自动完成多轮循环并回到可输入状态。

## T20: 手动验证 Plan Mode 场景

**文件：** 无固定文件，使用本地构建产物或 `go run`

**依赖：** T18

**步骤：**
1. 启动程序。
2. 输入 `/plan <一个需要查看项目文件的任务>`。
3. 观察计划阶段只使用读文件、找文件、搜代码等只读工具，并输出计划。
4. 输入 `/do`。
5. 观察执行阶段可以使用全部工具并接续上一轮计划。
6. 执行完成后再次输入 `/do`，确认不会误复用旧计划。

**验证：** `/plan` 不执行副作用工具；`/do` 使用最近计划执行；执行完成后旧计划被清空或不再可用。

## 执行顺序

```text
T1 → T2 ┐
T3 → T4 ├─→ T7 → T8 → T9 → T10 → T11 → T12
T3 → T5 ┘                         │
T6 ────────────────────────────────┘
                                    ↓
T13 → T14 → T15 → T16 → T17 → T18 → T19 → T20
```
