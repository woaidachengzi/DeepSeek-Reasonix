# MiMo 式会话存储：评审与分阶段落地

> 2026-09-23；针对 [原始提案](./MIMO_STYLE_SESSION_STORAGE_PROPOSAL.md) 与当前 `experiment/tauri` 分支。本文是实施边界，不表示已迁移用户数据。

## 结论

参考 MiMo Code 的**职责划分**：SQLite 保存会话身份和需要事务更新的状态，Markdown 保存可编辑的长期记忆，FTS5 是可重建的检索索引。Reasonix 先让 Tauri Preview 的会话身份与工作台状态进入独立、持久的 SQLite；现有 JSONL 和侧车继续负责执行记录与恢复。消息/工具事件改由 SQLite 写入是后续独立切换，必须先有完整的回放、崩溃与回滚测试。

这一区分很重要：MiMo Code 的 SQLite 不只是一张会话索引表。已对照用户提供的 `/Users/jerry/temp/app/MiMo-Code-main` 源码：`session.sql.ts` 定义 `session`、`message`、`part` 等表，`json-migration.ts` 将旧 JSON 数据导入 SQLite；Markdown 记忆经独立 FTS5 索引，恢复时使用 checkpoint、项目记忆和近期消息。[会话表源码](https://github.com/XiaomiMiMo/MiMo-Code/blob/main/packages/opencode/src/session/session.sql.ts)、[迁移源码](https://github.com/XiaomiMiMo/MiMo-Code/blob/main/packages/opencode/src/storage/json-migration.ts)、[记忆检索源码](https://github.com/XiaomiMiMo/MiMo-Code/blob/main/packages/opencode/src/memory/service.ts)。本路线借鉴职责划分，不直接复制它的存储格式。

## 当前系统已有的能力

| 数据 | 当前权威位置 | 已有索引/恢复能力 |
| --- | --- | --- |
| Agent 对话与工具事件 | `*.jsonl` 和相邻侧车 | `internal/sessioncatalog` 从文件重建目录；`internal/historycatalog` 用 SQLite FTS5 搜索历史。 |
| Desktop 主题元数据 | `internal/topicstate` 的 SQLite | 数据库位于 state home，而非可删除的 cache。 |
| Tauri 最近对话、标题和工作区 | `desktop/tauri/src/workbench_catalog.rs` 的 `workbench-sessions.json` | Bridge 以 `tauri-…` ID 定位固定名称的 JSONL；移动文件仍会让恢复失败。 |
| 长期记忆 | `internal/memory` 的 Markdown 事实文件及 `MEMORY.md` | 已有检索和下一次真实用户回合的 `session-context` 注入；尚无 MiMo 式 `notes.md` / `tasks/<id>/progress.md` 完整生命周期。 |

当前 Tauri 前端生成的 `session.id` 已是独立的 `tauri-…` ID，**不是**原提案所写的 `path:/…`；bridge 为这个 ID 生成 `tauri-<session.id>.jsonl` 路径，因此常见文件名是 `tauri-tauri-<UUID>.jsonl`。Wails/其他入口仍可能以路径作引用。两者的迁移不能混成一个字段重命名。

## 对原提案的必要修正

1. **身份库不能是可删除索引。** UUID 仅存于可重建 SQLite 时，删除索引就会重新分配 ID；“索引可全量重建”和“ID 永不变”不能同时成立。新身份库必须放在 Preview 的 state home，纳入备份；`sessioncatalog` 与 `historycatalog` 继续作为可重建投影。
2. **不要用大小与 mtime 判定同一个会话。** 它们可用于跳过未变文件的扫描，但不能决定重命名、复制或替换后的身份。自动重连只接受唯一、经内容/侧车证据确认的匹配；歧义时保留旧记录为 missing，等待显式选择。
3. **旧数据不在原稳定目录上就地迁移。** Tauri Preview 的 `REASONIX_HOME` 默认隔离。若以后导入 Wails 1.38.3 会话，应先备份并复制到 Preview，再为副本建立 ID；不能让两个宿主同时写同一套文件。
4. **会话“状态”需要限定含义。** SQLite 第一阶段负责 ID、当前路径、工作区、标题、归档/缺失标记及恢复定位；`running` 这类进程状态启动后必须从 Controller 和日志核实。待审批、执行进度和消息在事件存储切换前仍以原持久化为准。
5. **恢复快照已有入口。** `session-context` 和 Markdown 记忆已经参与恢复/下一回合。新增 checkpoint、notes、progress 时要复用这一入口，限制注入预算，并确保不会把旧 checkpoint 当作更新的权威事实。
6. **备份目标是实际 Preview 数据根。** 不固定为 `~/.reasonix`，也不在 SQLite WAL 活跃时复制单个 `.sqlite` 文件。备份流程需要在宿主停止写入后执行或使用 SQLite backup API，并校验可恢复。

## 存储边界

```text
Preview state home/session-state-v1.sqlite  持久：ID、路径、标题、工作区、生命周期元数据
Preview sessions/*.jsonl + sidecars          当前执行与对话真相源；保留原格式
cache/session-catalog + history-search       可重建的目录投影和历史 FTS5
Preview memory/**/*.md                       可编辑记忆与任务文件的真相源
cache/memory-search                           可重建的 Markdown FTS5（后续新增）
```

同一事实只指定一个写入权威。Go sidecar 写会话身份库，Tauri host 通过 bridge 查询；host 不直接修改该库。Tauri 的 `workbench-sessions.json` 在迁移期只作为回退输入，完成核对后才停止读取。

### 身份约束

- 新 Tauri 会话沿用现有 `tauri-…` ID，不为已有会话重新分配 UUID。ID 一经登记不得改变。
- SQLite 的 `id` 为主键；规范化路径在当前可见会话中唯一；旧路径放入单独 alias 表以支持显式移动，不把路径当 ID。
- 记录已有 ID 的文件若消失，只标记 missing 并提示恢复；不能以相同 ID 静默创建空 JSONL。
- 数据库损坏或丢失时从备份恢复。若只有旧 JSONL 且没有可验证的 ID 证据，可恢复内容，但不能承诺原 ID 不变。

## 实施顺序与退出条件

| 阶段 | 改动 | 通过条件 |
| --- | --- | --- |
| 0：契约 | 冻结上述权威边界、数据库路径及 schema；准备样本清单 | 明确区分 Preview 与稳定版数据；旧应用完全不受影响。 |
| 1：旁路 | 在 Preview state home 建持久身份库，只读检查现有 Preview JSONL；从 `workbench-sessions.json` 保留原 ID、标题与顺序。先完成独立存储与导入测试，再接宿主目录 | 重扫幂等、关机中断可重跑、文件内容不变、旧 catalog 仍可用。 |
| 2：双轨 | 新会话登记 ID；打开、重命名、删除和工作区切换都经同一个 resolver；host 从 bridge 读最近列表 | 现有会话重启后 ID 不变；缺失文件不会变空会话；删除/移动失败可恢复。 |
| 3：检索与记忆 | 复用历史 FTS5；为 Markdown 记忆加独立可重建索引；按预算注入 notes/progress/checkpoint | 手工编辑可被重新索引；恢复时只注入最新有效快照，消息和秘钥不泄露到诊断。 |
| 4：可选的事件存储切换 | 在单独 RFC 中定义 SQLite 消息/工具事件表、顺序号、提交确认和旧 JSONL 导入；先影子校验再切换新会话写入 | 崩溃、重放、审批、撤销、分支、压缩、导出/回滚均通过；旧 JSONL 保持可读。 |

阶段 1–3 可以兑现稳定身份与 MiMo 式记忆体验，同时保持 1.38.3 的执行格式。阶段 4 才意味着“整段对话由 SQLite 存储”；在它之前不能对外宣称已完成全量 SQLite 迁移。

### 阶段 1 当前进展

已新增 `internal/sessionidentity` 的独立 SQLite 存储和只读 `workbench-sessions.json` 导入器；数据库路径由 `config.DesktopSessionIdentityPath()` 指向 Preview 的 state home。导入时保留现有 ID、标题、工作区和顺序，不读取或改写 JSONL 内容；未落盘的新标签不会注册，已登记但文件缺失的会话会标记 missing，路径变化拒绝自动绑定。重复导入、重启、冲突回滚和路径边界已有单元测试。

**此时尚未接入 Tauri 启动/bridge 流程，也未对用户现有数据执行导入。** 下一步要让宿主以显式 Preview 目录调用导入器，再核对导入清单并加入失败回退；在此之前旧 `workbench-sessions.json` 仍是实际 UI 数据源。

## 验证重点

- 旧 Preview 会话：空历史、长历史、损坏尾行、缺失侧车、同名复制、路径移动、运行中断各有样本；迁移前后历史和恢复状态逐字段对比。
- SQLite：重复扫描、事务中断、WAL 恢复、数据库损坏、备份恢复、同路径冲突；任何错误都不得覆盖 JSONL。
- Bridge：同一 ID 打开/切换/重命名/删除；原有 Tauri 客户端与新可选字段兼容。
- 记忆：项目与全局隔离、手工编辑重新索引、检索结果去重、`session-context` 注入预算与时间顺序。

原提案中的 `41` 个提交和 `89` 个文件、`+22,543` 行可从本地 `v1.38.3..v1.38.10` 的 `internal/session` 差异复核；它们说明变更规模，不能单独证明某种架构是崩溃原因。
