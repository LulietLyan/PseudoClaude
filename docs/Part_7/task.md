# PseudoClaude 上下文管理 Tasks

## 文件清单

| 操作 | 文件 | 职责 |
| --- | --- | --- |
| 新建 | `internal/compact/constants.go` | 上下文管理阈值常量 |
| 新建 | `internal/compact/runtime.go` | Runtime、Session、ledger、usage 锚点、自动熔断 |
| 新建 | `internal/compact/estimate.go` | 近似 token 估算与 usage 提取 |
| 新建 | `internal/compact/preview.go` | 工具结果预览文本构造 |
| 新建 | `internal/compact/layer1.go` | 工具结果落盘与稳定替换 |
| 新建 | `internal/compact/recent.go` | 近期原文选择与工具边界扩展 |
| 新建 | `internal/compact/summary.go` | 摘要 prompt、provider 调用、summary 解析 |
| 新建 | `internal/compact/manage.go` | 自动/手动压缩编排 |
| 新建 | `internal/compact/*_test.go` | compact 包单元测试 |
| 修改 | `internal/conversation/conversation.go` | 增加整体替换和长度查询 |
| 修改 | `internal/conversation/conversation_test.go` | 增加 ReplaceMessages 深拷贝测试 |
| 修改 | `internal/config/config.go` | 增加 `context_window` 与默认推导 |
| 修改 | `internal/config/config_test.go` | 增加配置字段测试 |
| 修改 | `.PseudoClaude/config.yaml.example` | 展示 `context_window` 示例 |
| 修改 | `internal/agent/runner.go` | 自动压缩接入与 usage 锚点更新 |
| 修改 | `internal/agent/runner_test.go` | 增加自动压缩集成测试 |
| 修改 | `internal/tui/tui.go` | Model 持有 Runtime |
| 修改 | `internal/tui/select.go` | provider 选择后更新 context window |
| 修改 | `internal/tui/stream.go` | `/compact` 命令入口与状态展示 |
| 修改 | `internal/tui/stream_test.go` | 增加命令路由和手动压缩测试 |

## T1: 新建 compact 常量

**文件：** `internal/compact/constants.go`  
**依赖：** 无

**步骤：**

1. 新建 `internal/compact` 目录。
2. 定义单条工具结果阈值、单轮聚合阈值、摘要预留、自动/手动安全余量、近期原文下界、熔断阈值、预览截断阈值和字符/token 估算比。
3. 常量名与 `plan.md` 保持一致，使用导出名，便于测试引用。

**验证：** `go test ./internal/compact` 能编译通过；此时没有测试也应返回无测试文件或通过。

## T2: 实现会话目录与 Runtime 构造

**文件：** `internal/compact/runtime.go`  
**依赖：** T1

**步骤：**

1. 定义 `Session`、`Runtime`、`RuntimeSnapshot`、`UsageAnchor`、`ReplacementDecision`、`ReplacementLedger`。
2. 实现 `NewRuntime(workspace string, contextWindow int64)`，创建 `.PseudoClaude/sessions/<session_id>/tool-results`。
3. 会话 ID 使用 `<unix_ts>-<short_random>` 格式。
4. 初始化 replacement ledger、usage anchor、context window 和自动失败计数。

**验证：** 新增 `TestNewRuntimeCreatesSessionDirectories`，运行 `go test ./internal/compact -run TestNewRuntimeCreatesSessionDirectories`，期望会话目录和 `tool-results` 目录存在。

## T3: 实现 Runtime 并发安全访问

**文件：** `internal/compact/runtime.go`、`internal/compact/runtime_test.go`  
**依赖：** T2

**步骤：**

1. 实现 `Snapshot`、`SetContextWindow`、`RecordAutoSuccess`、`RecordAutoFailure`、`AutoTripped`。
2. 所有 Runtime 状态读写通过同一把 mutex 保护。
3. `RecordAutoSuccess` 清零连续失败计数。
4. `RecordAutoFailure` 达到 3 次后让 `AutoTripped` 返回 true。

**验证：** 新增并运行 `go test ./internal/compact -run 'TestRuntimeSnapshot|TestAutoBreaker'`，期望 context window 可更新，失败 3 次触发熔断，成功后清零。

## T4: 实现 replacement ledger 操作

**文件：** `internal/compact/runtime.go`、`internal/compact/runtime_test.go`  
**依赖：** T3

**步骤：**

1. 实现 Runtime 内部的 ledger 查询、标记 keep、标记 replace 辅助方法。
2. 确保已 replace 的 id 返回保存的预览字符串。
3. 确保已 keep 的 id 不返回预览。
4. 不暴露裸 map 给包外调用者。

**验证：** 新增 `TestReplacementLedgerFreezesDecisions`，运行 `go test ./internal/compact -run TestReplacementLedgerFreezesDecisions`，期望 keep/replace 决策稳定复用。

## T5: 实现 usage token 提取

**文件：** `internal/compact/estimate.go`、`internal/compact/estimate_test.go`  
**依赖：** T1

**步骤：**

1. 实现 `UsageTokens(*llm.Usage) (int64, bool)`。
2. 优先返回 `TotalTokens`。
3. `TotalTokens` 为 0 时返回 `InputTokens + OutputTokens + CacheRead + CacheWrite`。
4. usage 为 nil 或所有字段为 0 时返回 `false`。

**验证：** 新增并运行 `go test ./internal/compact -run TestUsageTokens`。

## T6: 实现消息近似估算

**文件：** `internal/compact/estimate.go`、`internal/compact/estimate_test.go`  
**依赖：** T5

**步骤：**

1. 实现 `EstimateMessages([]llm.Message) int64`。
2. 估算内容包含普通消息 Content、assistant tool calls 的 id/name/arguments、tool result 的 name/call id/content/error 标记。
3. 使用字符数除以 `EstimateCharsPerToken`，向上取整，空消息返回 0。
4. 避免 JSON 序列化失败导致 panic。

**验证：** 新增 `TestEstimateMessagesCountsContentAndToolPayloads`，运行 `go test ./internal/compact -run TestEstimateMessagesCountsContentAndToolPayloads`。

## T7: 实现锚点增量估算

**文件：** `internal/compact/estimate.go`、`internal/compact/estimate_test.go`  
**依赖：** T6

**步骤：**

1. 实现 `EstimateWithAnchor(messages []llm.Message, anchor UsageAnchor) int64`。
2. 当 `anchor.MessageCount` 在合法范围内时，只估算该索引之后的新增消息并加上 `anchor.Tokens`。
3. 当 anchor 越界或为空时，退回全量估算。
4. 保证返回值不为负。

**验证：** 新增 `TestEstimateWithAnchorUsesOnlyNewMessages`，运行 `go test ./internal/compact -run TestEstimateWithAnchorUsesOnlyNewMessages`。

## T8: 实现 Runtime usage 锚点更新

**文件：** `internal/compact/runtime.go`、`internal/compact/runtime_test.go`  
**依赖：** T5

**步骤：**

1. 实现 `UpdateUsageAnchor(usage *llm.Usage, messageCount int)`。
2. usage 有效时用新 token 和 message count 替换旧锚点。
3. usage 无效时保持旧锚点不变。
4. 实现 `ResetUsageAnchor(tokens int64, messageCount int)` 供压缩后重置。

**验证：** 新增 `TestUsageAnchorUpdateAndReset`，运行 `go test ./internal/compact -run TestUsageAnchorUpdateAndReset`。

## T9: 实现预览头部截断

**文件：** `internal/compact/preview.go`、`internal/compact/layer1_test.go`  
**依赖：** T1

**步骤：**

1. 实现内部函数按“先取前 20 行，再限制到 2048 字节”生成头部预览。
2. 字节截断不得产生无效 UTF-8；必要时向前回退到合法边界。
3. 短内容保持原样。

**验证：** 新增 `TestPreviewHeadLimitsLinesAndBytes`，运行 `go test ./internal/compact -run TestPreviewHeadLimitsLinesAndBytes`。

## T10: 实现稳定预览文本

**文件：** `internal/compact/preview.go`、`internal/compact/layer1_test.go`  
**依赖：** T9

**步骤：**

1. 实现预览文本构造函数。
2. 文本包含原始字节数、头部预览、完整落盘路径、重读提示。
3. 输出格式固定，不包含时间戳等会变化的内容。
4. 重读提示明确要求使用文件读取工具读取路径。

**验证：** 新增 `TestBuildPreviewIncludesMetadataPathAndInstruction`，运行 `go test ./internal/compact -run TestBuildPreviewIncludesMetadataPathAndInstruction`。

## T11: 实现单条工具结果落盘

**文件：** `internal/compact/layer1.go`、`internal/compact/layer1_test.go`  
**依赖：** T2、T10

**步骤：**

1. 实现内部 `spillToolResult`，写入 `Session.SpillDir/<call_id>`。
2. 文件已存在时跳过写入，不更新 mtime。
3. call id 为空时使用稳定派生 id，避免非法文件名。
4. 返回完整落盘路径。

**验证：** 新增 `TestSpillToolResultIsIdempotent`，运行 `go test ./internal/compact -run TestSpillToolResultIsIdempotent`。

## T12: 实现单条阈值替换

**文件：** `internal/compact/layer1.go`、`internal/compact/layer1_test.go`  
**依赖：** T11

**步骤：**

1. 实现 `OffloadToolResults` 的基础遍历。
2. 对超过 `SingleToolResultLimitBytes` 的 `ToolResult.Content` 执行落盘。
3. 返回新消息切片，不直接修改传入切片。
4. 将替换决策写入 Runtime ledger。

**验证：** 新增 `TestOffloadSingleLargeToolResult`，运行 `go test ./internal/compact -run TestOffloadSingleLargeToolResult`，期望原文落盘、消息中为预览。

## T13: 实现替换决策复用

**文件：** `internal/compact/layer1.go`、`internal/compact/layer1_test.go`  
**依赖：** T12

**步骤：**

1. `OffloadToolResults` 遇到已 replace 的 call id 时直接复用 ledger 中的预览字符串。
2. 已 keep 的 call id 保持原文。
3. 不重复计算预览，不重复写文件。
4. 测试中用第二轮不同内容验证 replace 仍复用旧预览。

**验证：** 新增 `TestOffloadReusesFrozenReplacement`，运行 `go test ./internal/compact -run TestOffloadReusesFrozenReplacement`。

## T14: 实现落盘失败降级

**文件：** `internal/compact/layer1.go`、`internal/compact/layer1_test.go`  
**依赖：** T12

**步骤：**

1. 让落盘错误只影响当前工具结果。
2. 出错时保留原始 content。
3. 出错 id 不写入 ledger，下一轮可重新评估。
4. `OffloadToolResults` 返回可观测的 error 或 warning 由调用方记录，但不阻断消息返回。

**验证：** 新增 `TestOffloadFailureKeepsOriginalAndDoesNotFreeze`，运行 `go test ./internal/compact -run TestOffloadFailureKeepsOriginalAndDoesNotFreeze`。

## T15: 实现同轮工具结果分组

**文件：** `internal/compact/layer1.go`、`internal/compact/layer1_test.go`  
**依赖：** T12

**步骤：**

1. 识别一条 assistant tool call 消息及其后连续 tool result 消息为一组。
2. 只把 call id 与该 assistant tool calls 匹配的结果纳入同组。
3. 普通 user/assistant 文本消息会结束当前工具结果组。
4. 暴露内部辅助函数供测试。

**验证：** 新增 `TestGroupToolResultsByAssistantToolCalls`，运行 `go test ./internal/compact -run TestGroupToolResultsByAssistantToolCalls`。

## T16: 实现聚合阈值替换

**文件：** `internal/compact/layer1.go`、`internal/compact/layer1_test.go`  
**依赖：** T15

**步骤：**

1. 对每组未替换工具结果统计剩余 content 字节数。
2. 超过 `ToolRoundAggregateLimitBytes` 时，按 content 字节数从大到小落盘。
3. 落盘到剩余聚合体积小于等于阈值为止。
4. 单条阈值已替换项不重复参与聚合预算。

**验证：** 新增 `TestOffloadAggregateLimitChoosesLargestResults`，运行 `go test ./internal/compact -run TestOffloadAggregateLimitChoosesLargestResults`。

## T17: 实现 Conversation 整体替换

**文件：** `internal/conversation/conversation.go`、`internal/conversation/conversation_test.go`  
**依赖：** 无

**步骤：**

1. 新增 `ReplaceMessages([]llm.Message)`。
2. 复用或抽取 deep copy 逻辑，确保 ToolCalls 和 ToolResult 都被深拷贝。
3. 新增 `Len() int`。
4. 保持现有 `Messages()` 行为不变。

**验证：** 新增 `TestConversationReplaceMessagesDeepCopies`，运行 `go test ./internal/conversation -run TestConversationReplaceMessagesDeepCopies`。

## T18: 实现近期原文选择

**文件：** `internal/compact/recent.go`、`internal/compact/recent_test.go`  
**依赖：** T6

**步骤：**

1. 实现 `SelectRecent([]llm.Message) []llm.Message`。
2. 从尾部向前累加，直到 token 下界和消息条数下界同时满足。
3. 历史不足时返回全部消息。
4. 返回结果保持原始顺序。

**验证：** 新增 `TestSelectRecentRequiresTokensAndMessageCount`，运行 `go test ./internal/compact -run TestSelectRecentRequiresTokensAndMessageCount`。

## T19: 实现工具边界扩展

**文件：** `internal/compact/recent.go`、`internal/compact/recent_test.go`  
**依赖：** T18

**步骤：**

1. 实现 `ExpandToToolBoundary`。
2. 如果近期原文起点落在 tool result 上，向前找到对应 assistant tool call。
3. 如果多个 tool result 属于同一 assistant tool call 消息，保留整组。
4. `SelectRecent` 调用该扩展逻辑后再返回。

**验证：** 新增 `TestSelectRecentDoesNotSplitToolCallAndResult`，运行 `go test ./internal/compact -run TestSelectRecentDoesNotSplitToolCallAndResult`。

## T20: 实现摘要 prompt 和 summary 解析

**文件：** `internal/compact/summary.go`、`internal/compact/summary_test.go`  
**依赖：** T1

**步骤：**

1. 实现摘要 system prompt，明确禁止工具调用。
2. 实现摘要 user prompt，要求 `<analysis>` 草稿和 `<summary>` 正式摘要。
3. 列出 9 个固定摘要部分。
4. 实现 `extractSummary`，只返回 `<summary>` 内文本。

**验证：** 新增 `TestSummaryPromptRequiresAnalysisAndNineSections` 和 `TestExtractSummaryDropsAnalysis`，运行 `go test ./internal/compact -run 'TestSummaryPrompt|TestExtractSummary'`。

## T21: 实现 fake provider 摘要调用测试工具

**文件：** `internal/compact/summary_test.go`  
**依赖：** T20

**步骤：**

1. 在测试中定义 fake provider，记录收到的 `llm.Request`。
2. fake provider 返回包含 `<analysis>` 和 `<summary>` 的流式文本。
3. fake provider 可配置返回错误，用于后续失败测试。
4. 保证测试不访问网络。

**验证：** `go test ./internal/compact -run TestFakeSummaryProvider`，期望 fake provider 能记录请求并返回摘要文本。

## T22: 实现摘要调用无工具

**文件：** `internal/compact/summary.go`、`internal/compact/summary_test.go`  
**依赖：** T21

**步骤：**

1. 实现内部 `summarize` 调用 provider。
2. 构造 `llm.Request` 时 `Tools` 必须为 nil。
3. 收集 provider 文本输出，解析正式 summary。
4. provider 返回错误时向上返回错误。

**验证：** 新增 `TestSummarizeSendsNoToolsAndKeepsOnlySummary`，运行 `go test ./internal/compact -run TestSummarizeSendsNoToolsAndKeepsOnlySummary`。

## T23: 实现摘要输入过大裁剪重试

**文件：** `internal/compact/summary.go`、`internal/compact/summary_test.go`  
**依赖：** T22

**步骤：**

1. 实现摘要请求前的输入估算。
2. 超过 `context_window - SummaryReserveTokens - safetyMargin` 时，丢弃最早一组历史后重试构造。
3. 最多重试 `SummaryRetryLimit` 次。
4. 重试后仍过大或 provider 仍失败时返回错误。

**验证：** 新增 `TestSummarizeDropsOldestHistoryWhenInputTooLarge`，运行 `go test ./internal/compact -run TestSummarizeDropsOldestHistoryWhenInputTooLarge`。

## T24: 实现压缩后消息构造

**文件：** `internal/compact/summary.go`、`internal/compact/summary_test.go`  
**依赖：** T18、T20

**步骤：**

1. 实现根据 summary、boundary、recent messages 构造新消息列表的函数。
2. 新列表形态为 summary 用户消息、summary assistant 消息、boundary 用户消息、近期原文。
3. boundary 文案明确要求需要文件/错误/工具结果/用户原话细节时重新读取，不要凭摘要猜测。
4. 保持近期原文的原始消息结构。

**验证：** 新增 `TestBuildCompactedMessagesIncludesBoundaryAndRecent`，运行 `go test ./internal/compact -run TestBuildCompactedMessagesIncludesBoundaryAndRecent`。

## T25: 实现手动 ForceCompact

**文件：** `internal/compact/summary.go`、`internal/compact/manage.go`、`internal/compact/summary_test.go`  
**依赖：** T17、T22、T24

**步骤：**

1. 实现 `ForceCompact(ctx, ManageInput)`。
2. 使用手动安全余量进行摘要输入判断。
3. 成功后调用 `Conversation.ReplaceMessages`。
4. 用压缩后的全量估算重置 Runtime usage 锚点。
5. 返回 `ManageOutput`，包含压缩前后 token。

**验证：** 新增 `TestForceCompactReplacesConversationAndResetsAnchor`，运行 `go test ./internal/compact -run TestForceCompactReplacesConversationAndResetsAnchor`。

## T26: 实现自动 ManageContext 未达阈值路径

**文件：** `internal/compact/manage.go`、`internal/compact/manage_test.go`  
**依赖：** T12、T17、T25

**步骤：**

1. 实现 `ManageContext` 自动路径入口。
2. 每次先调用第 1 层。
3. 第 1 层后重新估算 token。
4. 未达到自动阈值时不调用 provider 摘要。
5. 如果第 1 层有改写，则替换 conversation 并重置 usage 锚点。

**验证：** 新增 `TestManageContextAutoBelowThresholdOnlyRunsLayer1`，运行 `go test ./internal/compact -run TestManageContextAutoBelowThresholdOnlyRunsLayer1`。

## T27: 实现自动 ManageContext 达阈值路径

**文件：** `internal/compact/manage.go`、`internal/compact/manage_test.go`  
**依赖：** T26

**步骤：**

1. 达到 `context_window - SummaryReserveTokens - AutoSafetyMarginTokens` 时调用摘要。
2. 摘要成功后替换 conversation。
3. 清零自动失败计数。
4. 返回 `TriggeredLayer2=true` 和压缩前后 token。

**验证：** 新增 `TestManageContextAutoThresholdTriggersSummary`，运行 `go test ./internal/compact -run TestManageContextAutoThresholdTriggersSummary`。

## T28: 实现自动熔断

**文件：** `internal/compact/manage.go`、`internal/compact/manage_test.go`  
**依赖：** T27

**步骤：**

1. 自动摘要失败时调用 `RecordAutoFailure`。
2. 连续失败达到 3 次后，自动路径即使达阈值也不再调用 provider。
3. 手动 `ForceCompact` 不读取自动熔断状态。
4. 任意一次自动摘要成功后清零失败计数。

**验证：** 新增 `TestManageContextAutoBreakerTripsAndManualBypasses`，运行 `go test ./internal/compact -run TestManageContextAutoBreakerTripsAndManualBypasses`。

## T29: 接入 config context_window

**文件：** `internal/config/config.go`、`internal/config/config_test.go`  
**依赖：** 无

**步骤：**

1. `ProviderConfig` 新增 `ContextWindow int64` 字段。
2. 新增默认窗口常量。
3. 实现 `EffectiveContextWindow()`。
4. 校验 `context_window < 0` 返回可读错误。
5. 保持旧配置不带该字段时可加载。

**验证：** 新增 `TestProviderConfigEffectiveContextWindow` 和 `TestConfigRejectsNegativeContextWindow`，运行 `go test ./internal/config`。

## T30: 更新配置示例

**文件：** `.PseudoClaude/config.yaml.example`  
**依赖：** T29

**步骤：**

1. 在 provider 示例中加入 `context_window` 字段。
2. 增加注释说明单位 token、字段可选、默认值来源。
3. 不写真实 API key。

**验证：** `go test ./internal/config`；并手动检查示例中包含 `context_window` 和默认值注释。

## T31: TUI 创建 Runtime

**文件：** `internal/tui/tui.go`  
**依赖：** T2、T29

**步骤：**

1. `Model` 新增 `compactRuntime *compact.Runtime`。
2. `New` 根据 cwd 创建 Runtime。
3. 单 provider 时用该 provider 的 `EffectiveContextWindow()` 初始化 Runtime。
4. 多 provider 待选择时先用默认窗口初始化，确保 Runtime 非 nil。
5. 把 Runtime 注入 `m.runner.CompactRuntime`。

**验证：** 新增或更新 TUI 测试，运行 `go test ./internal/tui -run TestNewStartsTextareaFocusedAndAcceptsInput`，确保构造不回退。

## T32: Provider 选择后更新 Runtime 窗口

**文件：** `internal/tui/select.go`、`internal/tui/stream_test.go`  
**依赖：** T31

**步骤：**

1. provider 选择成功后调用 `compactRuntime.SetContextWindow(item.cfg.EffectiveContextWindow())`。
2. 确保 `m.runner.CompactRuntime` 指向同一个 Runtime。
3. provider 初始化失败时不改变当前 Runtime。

**验证：** 新增 `TestProviderSelectionUpdatesCompactContextWindow`，运行 `go test ./internal/tui -run TestProviderSelectionUpdatesCompactContextWindow`。

## T33: Runner 自动压缩接入

**文件：** `internal/agent/runner.go`  
**依赖：** T26、T31

**步骤：**

1. `Runner` 新增 `CompactRuntime *compact.Runtime` 字段。
2. 在每次构造普通 `llm.Request` 前调用 `compact.ManageContext` 自动路径。
3. `CompactRuntime` 为 nil 时跳过，保持现有行为。
4. `ManageContext` 返回错误时发送 error 事件和 stop。

**验证：** `go test ./internal/agent -run TestRunnerCompletesAfterMultipleToolRounds`，期望现有 Runner 测试仍通过。

## T34: Runner 自动压缩状态展示

**文件：** `internal/agent/runner.go`、`internal/agent/runner_test.go`  
**依赖：** T33

**步骤：**

1. 自动摘要触发前发送 `EventProgress`：`正在压缩上下文...`。
2. 自动摘要成功后发送 `EventProgress`：`已压缩上下文，token X -> Y`。
3. 自动摘要失败时发送可理解错误事件。
4. 第 1 层仅落盘但未摘要时不刷屏。

**验证：** 新增 `TestRunnerEmitsProgressForAutoCompact`，运行 `go test ./internal/agent -run TestRunnerEmitsProgressForAutoCompact`。

## T35: Runner 更新 usage 锚点

**文件：** `internal/agent/runner.go`、`internal/agent/runner_test.go`  
**依赖：** T8、T33

**步骤：**

1. 模型流收集完成后，如果有 usage，调用 Runtime `UpdateUsageAnchor`。
2. message count 使用本次 assistant 文本或 tool calls 写入 conversation 之后的长度。
3. usage 为空时不覆盖锚点。

**验证：** 新增 `TestRunnerUpdatesCompactUsageAnchor`，运行 `go test ./internal/agent -run TestRunnerUpdatesCompactUsageAnchor`。

## T36: Runner 自动触发集成测试

**文件：** `internal/agent/runner_test.go`  
**依赖：** T27、T33

**步骤：**

1. 使用 fake provider 构造较小 context window。
2. 预置 conversation，使估算 token 达到自动阈值。
3. fake provider 第一请求用于摘要，第二请求用于普通回复。
4. 断言摘要请求没有 tools，普通请求使用压缩后的 conversation。

**验证：** 新增 `TestRunnerAutoCompactBeforeModelRequest`，运行 `go test ./internal/agent -run TestRunnerAutoCompactBeforeModelRequest`。

## T37: TUI 内置命令分发

**文件：** `internal/tui/stream.go`、`internal/tui/stream_test.go`  
**依赖：** 无

**步骤：**

1. 抽取 idle 状态下以 `/` 开头输入的命令分发函数。
2. 迁移 `/exit`、`/plan`、`/chat`、`/exit-plan`、`/do` 的现有行为，保持输出文案兼容。
3. 未知命令显示可用命令提示，不发送给模型。
4. 普通文本仍走 `submit`。

**验证：** 更新 `TestPlanAndDoInputHandling` 并新增 `TestUnknownSlashCommandDoesNotSubmit`，运行 `go test ./internal/tui -run 'TestPlanAndDoInputHandling|TestUnknownSlashCommandDoesNotSubmit'`。

## T38: 实现 `/compact` TUI 命令

**文件：** `internal/tui/stream.go`、`internal/tui/stream_test.go`  
**依赖：** T25、T37

**步骤：**

1. `/compact` 不调用 `submit`，不写 conversation 用户消息。
2. 清空输入框。
3. 追加 `正在压缩上下文...` 状态消息。
4. 返回一个 `tea.Cmd` 调用 `compact.ForceCompact`。
5. 命令完成后追加成功或失败 transcript。

**验证：** 新增 `TestCompactCommandDoesNotSubmitUserMessage`，运行 `go test ./internal/tui -run TestCompactCommandDoesNotSubmitUserMessage`。

## T39: 手动压缩显示 token 变化

**文件：** `internal/tui/stream.go`、`internal/tui/stream_test.go`  
**依赖：** T38

**步骤：**

1. 定义手动压缩完成消息类型。
2. 成功消息包含 before/after token。
3. 失败消息包含错误文本。
4. TUI 收到完成消息后回到 idle 并保持输入框聚焦。

**验证：** 新增 `TestCompactCommandShowsTokenDelta`，运行 `go test ./internal/tui -run TestCompactCommandShowsTokenDelta`。

## T40: 配置示例解析测试

**文件：** `internal/config/config_test.go`、`.PseudoClaude/config.yaml.example`  
**依赖：** T30

**步骤：**

1. 测试读取 `.PseudoClaude/config.yaml.example`。
2. 用 yaml 解析到 `config.Config`。
3. 断言至少一个 provider 的 `ContextWindow` 非零。
4. 断言 `validate` 通过或示例中占位 key 符合现有校验。

**验证：** `go test ./internal/config -run TestConfigExampleParses`。

## T41: 全包 compact 测试整理

**文件：** `internal/compact/*_test.go`  
**依赖：** T1-T28

**步骤：**

1. 检查测试命名和 helper 是否重复。
2. 确保所有 fake provider 不访问网络。
3. 确保临时目录来自 `t.TempDir()`。
4. 跑 compact 全包测试。

**验证：** `go test ./internal/compact`。

## T42: 全项目格式化

**文件：** 所有新增/修改 Go 文件  
**依赖：** T1-T41

**步骤：**

1. 对新增和修改的 Go 文件运行 `gofmt -w`。
2. 不格式化 Markdown 文档。
3. 检查 gofmt 后没有意外改动无关文件。

**验证：** `gofmt -w internal/compact/*.go internal/conversation/conversation.go internal/conversation/conversation_test.go internal/config/config.go internal/config/config_test.go internal/agent/runner.go internal/agent/runner_test.go internal/tui/tui.go internal/tui/select.go internal/tui/stream.go internal/tui/stream_test.go` 后，`go test ./internal/compact` 仍通过。

## T43: 全项目测试

**文件：** 全项目 Go 包  
**依赖：** T42

**步骤：**

1. 运行 `go test ./...`。
2. 如果失败，先定位是否为新增上下文管理导致。
3. 修复后重跑失败包。
4. 最后再跑一次 `go test ./...`。

**验证：** `go test ./...` 全部通过。

## T44: 文档一致性检查

**文件：** `docs/Part_7/spec.md`、`docs/Part_7/plan.md`、`docs/Part_7/task.md`  
**依赖：** T1-T43

**步骤：**

1. 检查任务中的文件名、函数名、常量名与 plan 保持一致。
2. 检查每条 plan 模块至少有对应任务。
3. 检查 spec 的 F1-F29 都能在任务列表中找到落点。
4. 如实现中因审批修改了命名，回写 task 文档。

**验证：** 人工 review `docs/Part_7/task.md`；运行 `rg -n "TBD|mewcode|RoleTool|\\.mewcode|占位符" docs/Part_7/task.md` 不应出现旧项目残留或未完成标记。

## 执行顺序

```text
T1
 ├─> T2 -> T3 -> T4
 ├─> T5 -> T6 -> T7 -> T8
 └─> T9 -> T10 -> T11 -> T12 -> T13 -> T14 -> T15 -> T16

T17
T18 -> T19
T20 -> T21 -> T22 -> T23
T18 + T20 -> T24 -> T25
T12 + T17 + T25 -> T26 -> T27 -> T28

T29 -> T30 -> T40
T2 + T29 -> T31 -> T32
T26 + T31 -> T33 -> T34 -> T35 -> T36
T37 -> T38 -> T39

T1-T39 -> T41 -> T42 -> T43 -> T44
```
