# PseudoClaude 上下文管理 Checklist

> 每一项都通过运行代码、测试替身或观察 TUI/事件行为来验证，聚焦系统行为。

## 实现完整性

- [ ] `internal/compact` 包已创建并可被编译调用。（验证：运行 `go test ./internal/compact`，期望编译和测试通过）
- [ ] 压缩阈值集中定义，包含单条工具结果、同轮聚合、摘要预留、自动 13K 安全余量、手动 3K 安全余量、近期原文 token/消息下界、熔断阈值、预览行数/字节上限和字符/token 估算比。（验证：检查 `internal/compact/constants.go`；运行 `go test ./internal/compact -run Test`）
- [ ] Runtime 创建时生成当前会话专属目录，路径位于 `.PseudoClaude/sessions/<session_id>/tool-results`，进程退出不自动清理。（验证：运行 Runtime 构造测试，检查目录存在且连续两次 session id 不同）
- [ ] Runtime 的替换决策、usage 锚点、context window 和自动失败计数读写是线程安全的。（验证：运行 `go test -race ./internal/compact`，期望无 data race）
- [ ] Conversation 支持整体替换消息并保持深拷贝语义。（验证：运行 `go test ./internal/conversation -run TestConversationReplaceMessagesDeepCopies`）
- [ ] 配置支持 `context_window` 字段，未配置时按 provider 协议推导默认窗口。（验证：运行 `go test ./internal/config`，覆盖 anthropic 默认 200000、openai 默认 128000、显式配置覆盖、负数拒绝）
- [ ] `.PseudoClaude/config.yaml.example` 展示 `context_window` 字段和默认值说明。（验证：打开示例文件，看到字段、单位 token、可选说明和默认值说明）

## 第 1 层轻量预防

- [ ] 每次普通模型请求发出前都会先执行工具结果轻量预防。（验证：Runner 集成测试中预置大工具结果，断言 provider 收到的普通请求已经是预览替换后的消息）
- [ ] 单个工具结果超过单条阈值时完整内容被落盘，对话中只保留预览。（验证：构造 60K 字节 tool result，运行 `OffloadToolResults`，断言落盘文件内容等于原文，消息中的 `ToolResult.Content` 不再是原文）
- [ ] 预览文本包含原始大小、头部预览、完整文件路径和重读提示。（验证：检查预览字符串包含这四类信息，且头部预览不超过 20 行和 2048 字节）
- [ ] 同一个已替换工具结果在后续请求中复用完全相同的预览文本。（验证：对同一 call id 连续运行两次轻量预防，断言预览字符串逐字节相等且落盘文件 mtime 不变）
- [ ] 同一个已决定保留原文的工具结果后续不被反复重新决策。（验证：第一次小结果标记 keep 后，第二次同 call id 内容变大仍按已冻结决策处理）
- [ ] 落盘失败时保留原始工具结果，不写入冻结决策，普通流程不中断。（验证：把 spill dir 指向不可写位置或注入失败写入器，断言 content 保持原文且下一轮仍会重新尝试）
- [ ] 同一轮工具结果合计超过聚合阈值时，系统按体积从大到小落盘足够数量的结果。（验证：构造 3 条 80K 工具结果同属一轮，运行轻量预防，断言剩余未替换体积不超过 200K 且被替换的是最大项集合）
- [ ] 第 1 层不调用 LLM，不改写用户普通消息。（验证：使用 fake provider 计数，运行仅触发第 1 层的 `ManageContext` 后 provider 请求数为 0，用户消息 Content 保持原文）

## Token 估算

- [ ] usage token 提取优先使用 provider 的 `TotalTokens`，否则使用 input/output/cache read/cache write 求和。（验证：运行 `go test ./internal/compact -run TestUsageTokens`）
- [ ] 全量消息估算会统计普通消息、assistant tool calls 和 tool result 内容。（验证：运行 `go test ./internal/compact -run TestEstimateMessagesCountsContentAndToolPayloads`）
- [ ] 锚点估算只对最近一次 usage 之后新增的消息做字符数增量估算。（验证：运行 `go test ./internal/compact -run TestEstimateWithAnchorUsesOnlyNewMessages`）
- [ ] 普通模型请求完成后，用最近一次有效 usage 替换锚点，而不是累加历史 usage。（验证：Runner fake provider 连续返回 1000、1500、2200，断言 Runtime anchor 依次等于最新值）
- [ ] provider 未返回 usage 时，不用虚构数值覆盖已有锚点。（验证：先设置 anchor，再跑一次无 usage 的 fake stream，断言 anchor 不变）
- [ ] 第 1 层或第 2 层改写历史后，Runtime 用压缩后全量近似值重置锚点。（验证：触发轻量替换或摘要后，断言 anchor 的 message count 和 tokens 对应压缩后 conversation）

## 第 2 层重量兜底

- [ ] 自动路径在第 1 层之后重新估算 token，未达到阈值时不触发摘要。（验证：`context_window - 20000 - 13000 - 1` 场景下 fake provider 摘要请求数为 0）
- [ ] 自动路径达到 `context_window - 20000 - 13000` 时，在普通模型请求前触发摘要。（验证：达到阈值场景中 fake provider 第一条请求是摘要请求，普通请求发生在压缩之后）
- [ ] 手动压缩不受自动阈值限制，估算 token 很低时也会触发摘要。（验证：TUI 或 compact 手动测试中设置低 token 历史，输入 `/compact` 后 fake provider 摘要请求数为 1）
- [ ] 手动路径使用 3K 安全余量判断摘要输入裁剪，不使用自动 13K 余量。（验证：构造只会越过手动预检边界的输入，断言按手动 margin 执行裁剪逻辑）
- [ ] 摘要请求不携带任何 tools。（验证：fake provider 记录摘要请求，断言 `len(req.Tools) == 0`）
- [ ] 摘要 prompt 明确要求先写 `<analysis>` 草稿再写 `<summary>` 正式摘要，并禁止工具调用。（验证：检查摘要请求的 system/user prompt，包含禁止工具和两个标签说明）
- [ ] 系统只保留 `<summary>` 内容，丢弃 `<analysis>` 草稿。（验证：fake provider 返回 analysis+summary，压缩后 conversation 中不存在 analysis 内容）
- [ ] 正式摘要包含 9 个固定部分，并在用户消息原文记录中保留压缩前用户原话或明确裁剪标注。（验证：解析压缩后 summary 文本，检查 9 个标题和用户消息子串）
- [ ] 摘要输入过大时按较早历史优先裁剪并重试，重试仍失败则返回一次摘要失败。（验证：fake provider/估算构造过大输入，断言最早历史被移除且失败只计一次自动失败）

## 近期原文与边界消息

- [ ] 压缩后保留近期原文，从尾部向前覆盖约 1 万 token 且不少于 5 条消息。（验证：运行 `go test ./internal/compact -run TestSelectRecentRequiresTokensAndMessageCount`）
- [ ] 近期原文不会切断 assistant tool call 和对应 tool result。（验证：运行 `go test ./internal/compact -run TestSelectRecentDoesNotSplitToolCallAndResult`）
- [ ] 压缩后消息序列包含 summary 用户消息、summary assistant 消息、边界 user 消息和近期原文。（验证：运行 `go test ./internal/compact -run TestBuildCompactedMessagesIncludesBoundaryAndRecent`）
- [ ] 边界消息明确提醒需要文件、错误、工具结果或用户原话细节时重新读取，不要凭摘要猜测。（验证：检查压缩后边界消息文本包含这些含义）

## 自动熔断

- [ ] 自动摘要失败会累计连续失败次数。（验证：fake provider 返回摘要错误，运行自动 `ManageContext`，断言失败计数增加）
- [ ] 自动摘要连续失败 3 次后，第 4 次达到阈值也不再自动触发摘要。（验证：运行 `go test ./internal/compact -run TestManageContextAutoBreakerTripsAndManualBypasses`）
- [ ] 任意一次自动摘要成功后，连续失败计数清零。（验证：失败两次后成功一次，再失败一次，断言未跳闸）
- [ ] 熔断只影响自动路径，手动 `/compact` 仍可执行。（验证：自动熔断后调用 `ForceCompact`，断言 fake provider 仍收到摘要请求）

## TUI 与命令

- [ ] TUI idle 状态下以 `/` 开头的输入走命令分发，不直接作为普通用户消息发送给 LLM。（验证：输入 `/unknown`，断言 conversation 未新增用户消息，Runner 未启动，界面显示可用命令提示）
- [ ] 迁移后的 `/exit`、`/plan`、`/chat`、`/exit-plan`、`/do` 行为保持原有语义。（验证：运行现有 TUI 命令相关测试，确认 plan mode、do mode 和退出行为不回退）
- [ ] `/compact` 不写入 conversation，不调用普通 submit，只触发一次手动摘要。（验证：输入 `/compact`，断言 conversation 用户消息数不变，fake provider 摘要请求数为 1）
- [ ] `/compact` 执行时先显示“正在压缩上下文...”状态。（验证：TUI 单测检查 transcript 出现该状态）
- [ ] `/compact` 成功后显示压缩前后估算 token 数。（验证：fake compact 返回 before=120000、after=42000，transcript 包含两个数字）
- [ ] `/compact` 失败后显示可理解错误信息且不 panic。（验证：fake compact 返回 error，TUI transcript 包含“压缩失败”或错误文本，状态回到 idle）
- [ ] 自动压缩触发时 TUI 可观察到正在压缩和完成提示。（验证：Runner emit progress，TUI handle event 后 transcript/progress 中出现“正在压缩上下文”和 token 变化）

## 集成

- [ ] TUI 创建 Runtime，并在单 provider 场景使用该 provider 的有效 context window 初始化。（验证：TUI 构造测试断言 `compactRuntime` 非 nil 且 context window 等于 provider 配置）
- [ ] 多 provider 选择成功后，Runtime 的 context window 更新为所选 provider 的有效值。（验证：运行 `go test ./internal/tui -run TestProviderSelectionUpdatesCompactContextWindow`）
- [ ] Runner 在 `CompactRuntime == nil` 时保持旧行为兼容。（验证：运行现有 `internal/agent` 测试，未注入 Runtime 的 Runner 仍能完成多轮工具调用）
- [ ] Runner 自动压缩发生在普通 provider 请求之前。（验证：fake provider 记录请求顺序，摘要请求先于普通请求）
- [ ] Runner 自动压缩后的普通请求使用压缩后的 conversation。（验证：fake provider 记录普通请求 messages，断言旧的大工具结果原文不在其中）
- [ ] Runner 收到自动压缩错误时通过 error/stop 事件报告，不导致 goroutine 泄漏或 panic。（验证：fake provider 摘要错误，收集事件包含 `EventError` 和 `StopStreamError`）
- [ ] 配置示例可以被 YAML 解析为 `config.Config`。（验证：运行 `go test ./internal/config -run TestConfigExampleParses`）

## 编译与测试

- [ ] `go test ./internal/compact` 全部通过。（验证：运行命令，退出码 0）
- [ ] `go test ./internal/conversation` 全部通过。（验证：运行命令，退出码 0）
- [ ] `go test ./internal/config` 全部通过。（验证：运行命令，退出码 0）
- [ ] `go test ./internal/agent` 全部通过。（验证：运行命令，退出码 0）
- [ ] `go test ./internal/tui` 全部通过。（验证：运行命令，退出码 0）
- [ ] `go test ./...` 全部通过。（验证：仓库根目录运行命令，退出码 0）
- [ ] `go test -race ./internal/compact` 无 data race。（验证：运行命令，退出码 0）
- [ ] 新增和修改的 Go 文件均已格式化。（验证：运行 `gofmt -l internal/compact/*.go internal/conversation/conversation.go internal/conversation/conversation_test.go internal/config/config.go internal/config/config_test.go internal/agent/runner.go internal/agent/runner_test.go internal/tui/tui.go internal/tui/select.go internal/tui/stream.go internal/tui/stream_test.go`，期望输出为空）
- [ ] 文档无旧项目残留或占位符。（验证：对 `docs/Part_7/spec.md`、`plan.md`、`task.md`、`checklist.md` 运行旧项目名、旧消息结构名和占位符扫描，排除本条验证文本自身后应无命中）

## 端到端场景

- [ ] 场景 1：长会话自动轻量预防。构造多轮工具调用，每轮产生大工具结果，观察到超大结果写入 `.PseudoClaude/sessions/<id>/tool-results`，普通请求中只保留预览和路径。（验证：fake provider + fake tool 集成测试通过）
- [ ] 场景 2：长会话自动重量兜底。配置较小 `context_window`，预置或累积足够历史，观察到普通模型请求前自动摘要，压缩后对话长度下降且后续回复继续进行。（验证：Runner 集成测试完成且事件中出现压缩状态）
- [ ] 场景 3：用户主动压缩。TUI 输入 `/compact`，观察到不新增用户消息、不发普通对话请求、执行一次摘要，并显示 token 从 X 到 Y。（验证：TUI 单测或手工运行）
- [ ] 场景 4：自动失败熔断。让自动摘要连续失败 3 次，观察第 4 次不再自动摘要；随后手动 `/compact` 仍能发起摘要。（验证：compact 管理层测试通过）
- [ ] 场景 5：需要原文时重新读取。压缩后的边界消息存在，摘要没有把落盘工具结果伪装成原文；当模型需要完整内容时，可通过预览里的路径使用文件读取工具重读。（验证：检查压缩后消息和落盘文件路径，手动或测试读取该路径内容等于原工具结果）
