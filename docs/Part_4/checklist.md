# 系统提示工程化 Checklist

> 每一项通过运行代码或观察行为来验证，聚焦系统行为。括号内标注验证方式与对应需求。

## 实现完整性

- [ ] 模块化装配：稳定系统提示由身份、系统约束、任务模式、动作执行、工具使用、语气风格、文本输出七个固定模块按优先级拼成，模块间空行分隔。（验证：`go test ./internal/prompt/...`，断言身份段在工具使用段之前且模块以空行分隔。对应 AC1/F1）
- [ ] 挂载即扩展：新增一个非空模块只需加入模块列表，装配结果会按优先级插入，不需要修改装配主逻辑。（验证：`go test ./internal/prompt/...`，传入额外模块并断言位置正确。对应 AC1/F1）
- [ ] 可选空槽：自定义指令、已激活 Skill、长期记忆为空时被跳过，不留下多余空行；填入占位内容时出现在固定模块之后。（验证：`go test ./internal/prompt/...`，断言空模块不出现且无连续多余空行。对应 AC2/F2）
- [ ] 缓存确定性：连续两次构造稳定系统提示逐字节相等；改变环境信息、工作目录或任务文本不改变稳定块。（验证：`go test ./internal/prompt/...` 和 `go test ./internal/agent/...`，断言 `BuildSystemPrompt()` 相等且 `System.Stable` 跨轮一致。对应 AC4/F4/N1）
- [ ] 环境信息呈现：环境段包含工作目录、平台、当前日期、应用版本、provider、模型和可用 git 摘要；环境段与稳定系统提示分属不同内容块。（验证：`go test ./internal/prompt/...` 断言 `Environment.Render()` 字段；`go test ./internal/llm/...` 断言 Anthropic system 为 stable/env 两块。对应 AC3/F3）
- [ ] 环境信息不进稳定块：稳定系统提示不包含当前日期、git 状态、cwd、provider 或 model 等动态字段。（验证：`go test ./internal/prompt/...` 检查稳定提示文本。对应 AC4/F4/N2）
- [ ] 关键约定双重强化：“优先使用专用工具”和“编辑前先读取”同时出现在稳定系统提示和相关工具描述中。（验证：`go test ./internal/prompt/...` 与 `go test ./internal/tools/...`，断言提示与 `read_file/find_files/search_code/edit_file/run_command` 描述包含关键语义。对应 AC7/F7）
- [ ] 工具描述稳定：改变任务、模式或环境不会改变工具名称、参数 schema、说明文本和排序。（验证：`go test ./internal/tools/...` 和 `go test ./internal/agent/...`，断言 `Definitions()` 顺序稳定且 plan/chat 工具集只因 safety 过滤变化。对应 AC8/F8/N1）
- [ ] 补充消息标签：`PlanReminder` 与 `SystemReminder` 输出以 `<system-reminder>` 标签包裹，空正文不注入空标签。（验证：`go test ./internal/prompt/...`。对应 AC9/F9）
- [ ] 缓存字段解析：provider 用量能对外暴露缓存写入与缓存读取；Anthropic 映射 cache creation/read，OpenAI 映射 cached tokens，缺字段时为零值且不报错。（验证：`go test ./internal/llm/...` 和 `go test ./internal/agent/...`，fake usage 断言 `Event.Usage` 透传。对应 AC6/F6/N7）
- [ ] Anthropic 缓存断点：稳定 system block 序列化后带 cache control，环境 block 不带 cache control。（验证：`go test ./internal/llm/...`，测试 `toAnthropicSystem` 或等价 helper，防止 cache control 被 omitzero 丢弃。对应 AC5/F5）

## 集成

- [ ] Agent 请求装配：每次 Run 都传入非空 `System.Stable` 和 `System.Environment`，并且 reminder 不写入持久 `Conversation`。（验证：`go test ./internal/agent/...`，fake provider 记录 `llm.Request`，再检查 `conv.Messages()`。对应 AC10/F9/F10）
- [ ] 计划模式按轮次注入：计划模式第 1 轮注入完整提醒，第 2 轮注入精简提醒，第 5 轮再次注入完整提醒。（验证：`go test ./internal/agent/...`，多轮 fake provider 断言 `req.Reminder` 详略。对应 AC11/F11）
- [ ] 计划模式工具集：计划模式只暴露只读工具，普通模式和执行模式暴露全量工具。（验证：`go test ./internal/agent/...`，断言 `req.Tools`。对应 AC11/F11）
- [ ] 稳定系统提示跨模式一致：普通模式、计划模式和执行模式的 `System.Stable` 相同，计划模式差异只通过 reminder 与工具集体现。（验证：`go test ./internal/agent/...`。对应 F4/F11/N1）
- [ ] Anthropic 历史合法：注入 reminder 后消息角色和工具结果配对仍合法，不产生破坏 provider 请求的序列。（验证：`go test ./internal/llm/...`，断言 reminder 追加到 user 内容或安全追加 user 消息；人工跑计划模式多轮不报 provider 400。对应 AC10/F10/N4）
- [ ] OpenAI 历史合法：OpenAI 请求中 system 前缀、历史消息、尾部 reminder 和工具结果都能按 SDK 类型正常构造。（验证：`go test ./internal/llm/...`，断言 `toOpenAIMessages` 输出。对应 AC10/F10）
- [ ] 跨协议一致：Anthropic 与 OpenAI 都接收同一稳定系统提示、环境段和 reminder，差异仅限协议装配和缓存字段解析。（验证：`go test ./internal/llm/...` 和一次 Anthropic/OpenAI 配置手工请求观察。对应 AC12/F12）
- [ ] 既有 Agent Loop 不退化：多轮工具调用、工具结果保序、只读工具并发、副作用工具串行、未知工具限制、stream error 和用户取消仍成立。（验证：`go test ./internal/agent/...` 与 `go test ./...`。对应 AC15/F15/N3）
- [ ] TUI 行为不变：TUI 仍能启动、选择 provider、提交普通消息、进入 `/plan`、执行 `/do`；缓存字段不进入状态栏。（验证：`go test ./internal/tui/...` 和一次手工 TUI 流程。对应 AC15/F15）
- [ ] 环境采集降级：非 git 目录、git 不可用或版本字段缺失时，环境段省略或标注不可用，请求仍发起。（验证：`go test ./internal/prompt/...` 使用临时非 git 目录；手工在非 git cwd 启动观察。对应 AC13/F13/N5）
- [ ] 环境采集不阻塞：git 状态采集有短超时，正常仓库中请求发起没有明显卡顿。（验证：手工运行 TUI 或测试消费者，观察请求开始进度能及时出现。对应 F13/N5）
- [ ] 人工对比场景完整：`docs/Part_4/evaluation.md` 包含只读规划、编辑前读取、优先工具选择、工具失败恢复、安全边界、输出风格、历史合法性、缓存观测八类场景。（验证：`rg -n "只读规划|编辑前读取|优先工具选择|工具失败|安全边界|输出风格|历史合法性|缓存观测" docs/Part_4/evaluation.md`。对应 AC14/F14）

## 编译与测试

- [ ] 项目编译无错误。（验证：`go build ./...`。对应 AC16/N11）
- [ ] 全量单元测试通过。（验证：`go test ./...`。对应 AC15/AC16/N3/N11）
- [ ] 静态检查无告警。（验证：`go vet ./...`。对应 AC16/N11）
- [ ] 关键并发路径无竞争告警。（验证：`go test -race ./internal/agent/... ./internal/tools/...`。对应 N3/N11）
- [ ] Go 文件格式化合规。（验证：`gofmt -l` 检查改动过的 Go 文件无输出。对应 AC16/N11）
- [ ] 敏感信息不回显：环境段、调试输出和普通对话区不包含 API key、认证 token 或敏感环境变量。（验证：代码检查 `GatherEnvironment` 不读取环境变量；手工输出检查；必要时 `rg -n "API_KEY|api_key|Authorization|Bearer" internal docs/Part_4` 排除文档说明本身。对应 AC16/N6）

## 端到端场景

- [ ] 场景 1：缓存命中观测。使用支持缓存字段的 Anthropic 配置在同一会话连续发送两条相近请求，观察调试事件或测试消费者输出首轮 `cache_write > 0`、后续轮 `cache_read > 0`。（对应 AC5/AC6/F5/F6）
- [ ] 场景 2：OpenAI 兼容降级。使用 OpenAI 或兼容 base URL 连续发送请求，端点返回 cached tokens 时可观测 `CacheRead`，端点不返回时为 0 且请求正常完成。（对应 AC6/F6/N7）
- [ ] 场景 3：计划模式按轮次。输入 `/plan <需要多步只读调研的任务>`，模型只使用只读工具产出计划；多轮中第 1 轮完整提醒、普通轮精简提醒；输入 `/do` 后切回全工具执行。（对应 AC11/F11）
- [ ] 场景 4：reminder 不被当作用户输入。计划模式下模型不复述 `<system-reminder>` 标签或直接回答其内容，而是按其约束使用只读工具和输出计划。（对应 AC9/F9/N10）
- [ ] 场景 5：环境感知。询问“我现在在哪个目录、什么平台、今天几号、当前模型是什么”，模型能依据环境段正确回答，不需要额外工具。（对应 AC3/F3）
- [ ] 场景 6：编辑前读取。要求修改某个文件时，模型先调用 `read_file` 查看目标文件，再调用 `edit_file` 或 `write_file`；不会直接用 `run_command` 拼凑读写。（对应 AC7/F7）
- [ ] 场景 7：取消后继续。计划模式多轮过程中按 Esc 取消，TUI 回到空闲态；再次发送普通消息或 `/plan` 请求不报 provider 角色/工具配对错误。（对应 AC10/AC15/F10/F15）
- [ ] 场景 8：非 git 目录降级。在非 git 目录启动或将工具环境指向非 git 临时目录，环境段省略或标注 git 状态不可用，普通对话仍正常完成。（对应 AC13/F13）
