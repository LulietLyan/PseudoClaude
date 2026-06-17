# PseudoClaude 项目记忆与会话恢复 Checklist

> 每一项都通过运行代码、检查文件或观察 TUI 行为来验证，聚焦系统行为。

## 编译与测试

- [ ] 项目完整测试通过（验证：运行 `go test ./...`，退出码为 0，输出无 `FAIL`）
- [ ] 并发敏感包无数据竞态（验证：运行 `go test -race ./internal/conversation/... ./internal/session/... ./internal/memory/...`，无 race 报告）
- [ ] 新增 Go 文件格式化完成（验证：运行 `gofmt -w` 后 `git diff --check` 无格式或空白错误）
- [ ] 旧参考命名已清理（验证：运行 `find docs/Part_8 -name tasks.md -print` 无输出；运行 `rg -n "mewcode|MEWCODE" internal cmd docs/Part_8 --glob '!task.md'` 无旧项目名残留）

## 项目指令

- [ ] 三层指令按优先级加载（验证：在项目根、项目 `.PseudoClaude/`、用户 `~/.PseudoClaude/` 各放一份 `PSEUDOCLAUDE.md`，调用 loader 或观察首轮 system prompt，内容顺序为项目根 → 项目 `.PseudoClaude` → 用户目录）
- [ ] 缺失指令文件静默跳过（验证：只保留项目根 `PSEUDOCLAUDE.md`，启动和 loader 测试均无错误，输出只包含该文件内容）
- [ ] 空项目不注入空模块（验证：三个路径都没有 `PSEUDOCLAUDE.md` 时，`BuildSystemPrompt` 输出不包含空的 Custom Instructions 内容）
- [ ] 独占行 include 正常展开（验证：`PSEUDOCLAUDE.md` 中写 `@include rules/style.md`，输出中该行被 `rules/style.md` 内容替换）
- [ ] 非独占行 include 保持原文（验证：段落中出现 `请参考 @include rules/style.md` 时，输出仍保留原句，不展开文件）
- [ ] include 深度限制生效（验证：构造 6 层 include 链，输出不展开超限层，并包含“超过最大嵌套深度”警告）
- [ ] include 环路检测生效（验证：A include B、B include A，输出跳过重复引用，并包含“检测到环路”警告）
- [ ] include 路径逃逸被拦截（验证：项目级文件写 `@include ../../outside.md`，输出不包含外部文件内容，并包含路径越界警告）
- [ ] 二进制 include 被跳过（验证：include 指向前 512 字节含 `\x00` 的文件，输出包含不可读或二进制警告）

## 会话存档

- [ ] 新会话 ID 使用新格式（验证：启动后 `.PseudoClaude/sessions/` 下出现形如 `YYYYMMDD-HHMMSS-xxxx` 的目录，正则匹配 `[0-9]{8}-[0-9]{6}-[0-9a-f]{4}`）
- [ ] 新会话目录包含 JSONL 和工具结果目录（验证：会话目录中存在 `conversation.jsonl`，并存在 `tool-results/`）
- [ ] JSONL 消息实时追加（验证：发送一条普通消息并获得回复后，`conversation.jsonl` 至少有 user 和 assistant 两条 `type=message` 记录，每行可用 `jq .` 解析）
- [ ] JSONL 首条有效消息记录模型（验证：读取 `conversation.jsonl` 第一条 message，能看到 `"model":"<当前模型>"`）
- [ ] 工具调用和工具结果可持久化（验证：触发一次读文件工具，JSONL 中出现 assistant `tool_calls` 记录和对应 `tool_result` 记录）
- [ ] 整体替换使用 append-only 快照（验证：触发工具结果落盘后，JSONL 中出现 `type=replace` 且 `reason=snapshot`，随后追加替换后的消息快照）
- [ ] 摘要压缩写入压缩边界（验证：触发上下文压缩后，JSONL 中出现 `type=replace` 且 `reason=compact`，随后追加压缩后的消息快照）
- [ ] 不存在单独 meta 文件（验证：会话目录中无独立 meta 文件；会话标题、消息数、更新时间能通过扫描 JSONL 和 stat 得到）
- [ ] 写入失败不破坏已有内容（验证：模拟 writer 错误，已有 JSONL 行仍可逐行解析，主会话报告可诊断错误但不崩溃）

## 会话恢复

- [ ] `/resume` 命令不发送给模型（验证：在 idle 输入 `/resume`，conversation 长度不增加，进入会话列表）
- [ ] 会话列表展示有效会话（验证：准备 3 个有效新格式会话，列表展示 3 项，并显示标题、相对时间、消息数、文件大小和可推导的模型）
- [ ] 会话列表可搜索和取消（验证：输入关键词后列表过滤；按 Esc 返回 idle，当前 conversation 不变）
- [ ] 旧格式目录不展示（验证：创建无法被新格式解析的 session 目录，`/resume` 列表不展示该项）
- [ ] 坏行恢复时跳过（验证：在 JSONL 中插入无效 JSON 行，恢复后坏行计数增加，其余有效消息正常加载）
- [ ] replace 边界后恢复（验证：JSONL 中存在多个 `type=replace` 标记时，恢复结果只包含最后一个 replace 标记之后的快照消息）
- [ ] 孤立工具调用被截断（验证：JSONL 末尾是 assistant tool calls 且无对应 tool result，恢复后该 assistant 消息不在 conversation 中）
- [ ] 恢复超限先压缩（验证：构造恢复后估算 token 超阈值的会话，选择恢复后先执行一次压缩，再进入 idle）
- [ ] 长时间暂停插入提醒（验证：恢复最后消息时间超过 6 小时的会话，conversation 末尾出现“暂停”或“上下文可能已过时”的提醒）
- [ ] 恢复成功后继续追加到原 JSONL（验证：恢复会话 A 后发送新消息，会话 A 的 `conversation.jsonl` 行数增加，新会话 B 不被写入）
- [ ] 恢复失败不破坏当前会话（验证：选择损坏到无法打开的会话时，TUI 显示错误，当前 conversation、writer、compact runtime 保持原值）
- [ ] Agent 运行中不能恢复（验证：streaming 或 approving 状态尝试 `/resume`，TUI 提示等待当前任务完成，不进入列表）

## 会话清理

- [ ] 30 天以上新格式会话被清理（验证：创建 31 天前 ID 的目录，启动或调用清理后该目录被删除）
- [ ] 新会话不会被清理（验证：创建 1 天前 ID 的目录，清理后目录仍存在）
- [ ] 旧格式目录不会被清理（验证：创建旧 ID 格式目录，清理后仍存在）
- [ ] 单个清理失败不影响其他目录（验证：模拟一个目录删除失败，其他过期目录仍被删除，错误被记录）
- [ ] 清理不阻塞启动（验证：存在大量过期目录时，TUI 仍能先显示 banner 或进入 provider 选择，清理在后台完成）

## 自动笔记

- [ ] 四类笔记类型受支持（验证：memory 单元测试分别 create `user_preference`、`correction_feedback`、`project_knowledge`、`reference_material`，文件和索引均正确）
- [ ] 项目级与用户级分开存储（验证：project operation 写入 `<workspace>/.PseudoClaude/memory/`，user operation 写入 `~/.PseudoClaude/memory/`）
- [ ] 笔记 frontmatter 完整（验证：新建笔记 Markdown 包含 `type`、`title`、`created`、`updated`）
- [ ] 索引行格式正确（验证：`MEMORY.md` 中每条摘要形如 `- [<type>] <title> — <一句话描述>`）
- [ ] 索引按项目级优先注入（验证：项目级和用户级 `MEMORY.md` 都有内容时，system prompt 的 Long-Term Memory 中项目级内容在前）
- [ ] 索引大小限制生效（验证：构造超过 200 行或 25KB 的索引，注入文本被裁剪并包含 `(index truncated)`）
- [ ] 自然完成后异步更新（验证：mock provider 跑一次无工具最终回复，StopCompleted 后触发 `UpdateAsync`，主回复不等待记忆更新完成）
- [ ] 非自然结束不更新记忆（验证：取消、stream error、max iterations、unknown tool limit 场景下 mock memory updater 未被调用）
- [ ] 记忆更新请求不带工具（验证：mock provider 捕获请求，`Tools` 为空）
- [ ] LLM 返回空数组时不写文件（验证：mock provider 返回 `[]`，memory 目录和索引不变）
- [ ] LLM create/update/delete 操作生效（验证：mock provider 依次返回 create、update、delete，笔记文件和 `MEMORY.md` 对应变化）
- [ ] 路径越界操作被拒绝（验证：LLM 返回 `../bad.md` 或含路径分隔符的 filename，Store 拒绝写入 memory 目录外）
- [ ] 记忆更新失败不影响主会话（验证：mock provider 返回错误或非法 JSON，用户下一条消息仍能正常处理，错误仅记录）

## Prompt 与集成

- [ ] `BuildSystemPrompt` 空输入向后兼容（验证：传空 `PromptInputs` 时，固定模块仍按原顺序输出，可选模块不出现）
- [ ] 自定义指令注入正确（验证：传入 instructions 后，system prompt 包含 Custom Instructions 内容）
- [ ] 长期记忆注入正确（验证：传入 memory 后，system prompt 包含 Long-Term Memory 内容）
- [ ] priority 顺序稳定（验证：多次运行同一输入，Custom Instructions 和 Long-Term Memory 的相对顺序一致）
- [ ] Conversation hook 不影响零值使用（验证：零值 `Conversation` 的 Add/Messages/Replace 行为通过现有测试）
- [ ] Layer 1 与 Layer 2 替换原因不同（验证：compact 测试中工具结果落盘触发 `snapshot`，摘要压缩触发 `compact`）
- [ ] 恢复后工具结果落在同一会话目录（验证：恢复会话后触发超大工具结果落盘，预览路径位于被恢复会话的 `tool-results/`）
- [ ] 全新项目静默降级（验证：没有 `PSEUDOCLAUDE.md`、没有 memory、没有 sessions 时，启动成功并能完成普通对话）
- [ ] `/memory` 展示长期记忆（验证：用户级 `MEMORY.md` 存在用户画像时，输入 `/memory` 后 transcript 中能看到 User Memory 和用户画像摘要）
- [ ] `/memory` 不进入模型（验证：输入 `/memory` 后 conversation 长度不增加，JSONL 不新增 user 消息）
- [ ] `/memory` 空状态清晰（验证：无项目级和用户级 memory 时输入 `/memory`，TUI 显示暂无长期记忆）
- [ ] `/resume` 使用轻量选择面板（验证：输入 `/resume` 后底部出现类似 allow 的选择框，而不是完整列表页面）
- [ ] `/resume` 键盘选择可用（验证：上下键移动光标，数字键选择对应会话，Enter 恢复，Esc 取消且当前会话不变）

## 端到端场景

- [ ] 场景 1：首次冷启动（验证：清空 `.PseudoClaude/sessions/` 和 `.PseudoClaude/memory/`，启动 PseudoClaude，发送“你好”，收到回复；会话目录创建，JSONL 至少两行且每行可解析）
- [ ] 场景 2：项目指令优先级（验证：项目根 `PSEUDOCLAUDE.md` 写“回复必须以 A 开头”，用户 `~/.PseudoClaude/PSEUDOCLAUDE.md` 写“回复必须以 B 开头”，启动后首轮回复遵循项目根指令）
- [ ] 场景 3：include 展开（验证：项目根 `PSEUDOCLAUDE.md` 写 `@include .PseudoClaude/rules/style.md`，启动后首轮 system prompt 或模型行为体现 include 文件内容）
- [ ] 场景 4：会话存档和工具记录（验证：请求读取 `go.mod`，JSONL 记录 user、assistant tool calls、tool result、final assistant）
- [ ] 场景 5：恢复并继续写入（验证：会话 A 对话后退出；启动会话 B 输入 `/resume` 选择 A；继续提问后 A 的 JSONL 行数增加）
- [ ] 场景 6：坏行容错（验证：手动向 A 的 JSONL 中插入无效行，`/resume` 仍恢复成功并跳过坏行）
- [ ] 场景 7：崩溃后恢复（验证：对话数轮后 `kill -9` 进程，重启 `/resume`，最后坏行如有则跳过，之前完整行恢复并可继续对话）
- [ ] 场景 8：过期清理（验证：创建 31 天前新格式会话和旧格式目录，启动后前者被删、后者保留）
- [ ] 场景 9：记忆积累与注入（验证：表达明确偏好后等待异步更新，重启新会话，Long-Term Memory 注入索引，后续回复体现该偏好）
- [ ] 场景 10：运行中恢复互斥（验证：长任务运行中尝试 `/resume`，TUI 不进入恢复列表并提示等待当前任务完成）
- [ ] 场景 11：压缩后恢复（验证：小 context window 触发自动压缩，JSONL 出现 `reason=compact` replace 记录；重启恢复后只加载压缩快照后的历史）
