# Agent Loop Checklist

> 每一项通过运行代码或观察行为来验证，聚焦系统行为。

## 实现完整性

- [x] ReAct 循环能在一次用户提交内自动完成多轮“模型请求工具 → 执行工具 → 回灌结果 → 再次请求模型 → 最终回答”（验证：运行 Agent fake provider 测试，构造两轮工具调用后一轮最终文本，期望停止原因为 completed）
- [x] 每次用户提交都有独立执行上下文，停止后界面恢复输入，下一轮能携带上一轮已回灌历史（验证：运行 Agent 会话历史测试，期望第二次 provider 请求包含前一轮 tool result）
- [ ] 最终回答停止、迭代上限、用户取消、连续未知工具、流式错误五类停止原因都可区分（验证：运行停止条件测试，分别断言 stop reason）
- [x] 最大迭代次数达到上限后不再发起新的模型请求（验证：将最大迭代设为 1，运行 fake provider 计数测试，期望 provider 调用次数不超过上限且 stop reason 为 max_iterations）
- [ ] 连续未知工具先作为结构化工具错误回灌，达到限制后停止；中间成功执行已知工具会清零计数（验证：运行未知工具限制测试，检查工具结果历史和 stop reason）
- [ ] 用户取消会停止 Agent Run，并且未完成工具不会追加成功结果（验证：运行取消测试，取消 context 后期望 stop reason 为 canceled，历史中无伪造成功 tool result）
- [ ] Agent 事件流能发布文本、工具调用、工具结果、进度、停止原因，以及 provider 可用时的 token 用量（验证：运行事件消费者测试，收集事件类型集合并断言包含预期类型）
- [x] 流式文本实时发布，同时完整 assistant 文本被正确收集并追加历史（验证：运行 collector 测试，输入多个文本分片，期望事件增量拼接结果等于历史文本）
- [x] 同一轮多个工具调用全部被处理，每个调用都有结果或错误结果（验证：运行多工具调用测试，期望结果数量等于调用数量）
- [x] 只读工具调用可并发执行，有副作用工具按稳定顺序串行执行（验证：运行工具分批测试，用慢/快工具和记录顺序的副作用工具断言行为）
- [x] 并发只读工具完成顺序不影响回灌顺序，工具结果顺序始终与模型请求顺序一致（验证：运行结果排序测试，期望历史 tool result call id 顺序等于请求顺序）
- [ ] provider 返回 token 用量时，Agent 发布用量事件；provider 不返回用量时任务仍正常结束（验证：分别运行带用量和无用量 fake provider 测试）
- [ ] 长任务期间能观察到轮次、工具批次或阶段进度（验证：运行进度事件测试，期望包含每轮开始和工具批次开始/结束说明）
- [x] `/plan <任务>` 使用只读工具集合，计划阶段不会执行写文件、改文件或运行命令（验证：运行 Plan Mode 测试，断言 provider 收到的工具定义只包含只读工具）
- [x] `/do` 能基于最近一次成功计划和原任务进入全工具执行；没有可用计划时给出清晰提示（验证：运行 TUI `/do` 测试，分别覆盖有计划和无计划）
- [x] 普通聊天且模型不请求工具时保持既有流式体验，不出现多余工具状态（验证：运行 TUI 普通聊天测试，期望文本流式显示并在 completed 后回到 idle）
- [ ] 已知工具参数错误、执行失败或超时会作为工具结果回灌，未触发停止条件时允许模型继续下一轮（验证：运行工具错误继续推理测试，fake provider 在收到错误结果后给出修正调用或最终回答）
- [x] TUI 只消费 Agent 事件更新展示；替换为测试事件源时仍能完成状态转换（验证：运行 TUI 事件消费测试，不直接调用 provider stream 或工具执行函数）

## 集成

- [x] `internal/tools` 的安全性元数据不会改变 provider 工具 schema（验证：运行 llm 工具转换测试，期望发送给 provider 的 schema 不包含 safety 字段）
- [x] `Registry.DefinitionsBySafety` 返回稳定排序的只读工具列表（验证：运行 registry 测试，期望多次调用结果顺序一致）
- [x] conversation 能把同一轮多个 assistant 工具调用保存在同一消息中并深拷贝返回（验证：运行 conversation 测试，修改返回值不影响内部历史）
- [ ] OpenAI provider 能在一轮中报告多个工具调用事件（验证：运行 OpenAI provider 适配测试，期望收集到多个 tool call id）
- [ ] Anthropic provider 能在一轮结束后报告所有 tool use block（验证：运行 Anthropic provider 适配测试，期望所有 tool use block 都转为统一事件）
- [x] Agent Run 使用 Plan Mode 只读 definitions，使用 Chat/Do Mode 全量 definitions（验证：运行 Agent 模式测试，检查 fake provider 收到的工具定义名称集合）
- [x] TUI `/plan` 成功完成后保存最近计划，`/do` 执行完成后清空计划（验证：运行 TUI Plan Mode 状态测试，检查 saved plan 状态变化）
- [ ] Ctrl+C 或现有取消路径能取消正在运行的 Agent Run（验证：运行 TUI 取消测试或手动按 Ctrl+C，期望界面退出或恢复时终端状态正常）
- [x] main 入口仍能装配配置、provider、registry 和 TUI（验证：运行 `go test ./cmd/PseudoClaude ./internal/tui`，期望编译通过）

## 编译与测试

- [x] 工具模块测试通过（验证：运行 `go test ./internal/tools`，期望通过）
- [x] conversation 模块测试通过（验证：运行 `go test ./internal/conversation`，期望通过）
- [x] llm 模块测试通过（验证：运行 `go test ./internal/llm`，期望通过）
- [x] agent 模块测试通过（验证：运行 `go test ./internal/agent`，期望通过）
- [x] tui 模块测试通过（验证：运行 `go test ./internal/tui`，期望通过）
- [x] 全项目测试通过（验证：运行 `go test ./...`，期望通过）
- [x] 项目能构建可执行程序（验证：运行 `go build ./cmd/PseudoClaude`，期望无编译错误）

## 端到端场景

- [ ] 场景 1（多轮连环）：使用 OpenAI 兼容端点启动程序，输入“读 `docs/ch03/spec.md`，再据内容新建 `docs/ch03/summary.txt` 写一句话摘要”，观察 `read_file` 与 `write_file` 跨多轮自动出现、状态栏 token 用量增长、动态区轮次递增、最终答复出现，随后输入 `/exit` 退出无终端残留（验证：summary 文件被创建且包含一句话摘要；无需手动输入“继续”；退出后终端状态正常）
- [ ] 场景 2（用户取消）：发送一个多步任务，中途按 Esc 取消，确认回到空闲态但程序不退出；再用 Ctrl+C 路径验证退出或取消行为符合设计；随后重新启动或继续发一条普通消息，确认历史未损坏且 provider 不返回 400（验证：取消后 stop reason 为 canceled 或界面显示取消提示；后续对话正常）
- [ ] 场景 3（流出错恢复）：临时改坏 `base_url` 后发送一条消息，观察错误块且程序不退出；改回正确 `base_url` 后继续发送普通消息，观察对话恢复正常（验证：错误显示为 stream/provider 错误；恢复后 stop reason 为 completed）
- [ ] 场景 4（Plan Mode）：输入 `/plan <一个改动类需求>`，观察计划阶段只出现 read/glob/grep 类只读工具和计划文本，没有写文件、改文件或执行命令；随后输入 `/do`，观察切回全工具并按计划执行，出现 write/edit/bash 类副作用工具（验证：计划阶段无副作用工具结果；执行阶段可见副作用工具并完成任务）
- [ ] 场景 5（跨协议，若有 Anthropic 配置）：切到 Anthropic 配置重跑场景 1，观察多轮工具调用、用量展示、轮次进度、最终回答和退出行为与 OpenAI 兼容端点一致（验证：同一任务在 Anthropic 下无需手动继续即可完成）
- [ ] 场景 6（迭代上限）：主要由 agent 测试确定性验证；可选手动复现为临时把 `maxIterations` 改小到 3，运行一个会多步调用工具的任务，观察第 3 轮后停止并显示迭代上限提示，之后仍可继续对话（验证：stop reason 为 max_iterations；provider 不再继续调用；后续普通对话正常）
- [ ] 场景 7（连续未知工具）：主要由 agent 测试确定性验证；可选手动复现为临时引导模型连续调用不存在的工具名，观察连续达到未知工具限制后停止并显示未知工具上限提示，之后仍可继续对话（验证：stop reason 为 unknown_tool_limit；未知工具错误已回灌；后续普通对话正常）
- [ ] 场景 8（普通聊天兼容）：输入一个模型直接文本回答的问题，观察流式文本显示、markdown 定型和结束回到输入状态，与引入 Agent Loop 前体验一致（验证：无工具状态出现，stop reason 为 completed）
