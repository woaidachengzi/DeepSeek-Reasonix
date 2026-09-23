# Tauri Preview 会话存储实施稿 v3

> 状态：实施设计，尚未授权真实用户数据迁移；基于 `experiment/tauri` 的 `4ed41fbe6` 及其后未提交的界面改动。
> 对照：本地草稿 `SESSION_STORAGE_PLAN_V2.md`、`MIMO_STYLE_SESSION_STORAGE_APPROVAL.md`，以及已入库的[既有评审](./MIMO_STYLE_SESSION_STORAGE_REVIEW.md)。
> 本稿替代前两份文档的实施建议；它们保留为设计讨论记录。本稿不是“已完成迁移”的声明。

## 1. 决定

采用 MiMo Code 的**职责划分**，不复制其一次性 JSON→SQLite 权威切换：

| 数据 | 阶段 1–3 的权威 | 说明 |
| --- | --- | --- |
| Tauri 会话 ID、当前定位、标题、工作区、排序、生命周期 | Preview state home 的持久 SQLite | 只能由 Go sidecar 写；不是可删除索引。 |
| 消息、工具事件、恢复侧车 | 既有 JSONL、事件日志与侧车 | 不改 1.38.3 执行格式或 Controller。 |
| 历史搜索与目录投影 | `historycatalog`、`sessioncatalog` | 已有 FTS5、迁移版本和扫描游标；继续可重建。 |
| 长期记忆 | 现有 Markdown 事实、`MEMORY.md` | 新 notes/progress/checkpoint 应复用 `session-context` 注入；另立阶段。 |

不在这轮把消息写入 SQLite，也不批量改名或重写 JSONL。阶段 4 若要做全量消息入库，必须单独 RFC 和崩溃/回放/回滚验证。保留现有 `tauri-<uuid>` 会话 ID；磁盘上的 `tauri-tauri-<uuid>.jsonl` 是既有命名，不能顺手消除双前缀。

## 2. 本轮必须修正的事实

1. `cmd/reasonix-desktop-bridge/core_runtime.go` 用 `controller.SessionDir()` 构造 `tauri-<sessionID>.jsonl`；默认 `SessionDir()` 是 **state home 下的扁平 `sessions/`**，与 `workspaceRoot` 无关。当前 `internal/sessionidentity/catalog.go` 把工作区会话推到 `projects/<slug>/sessions/`，是实际缺陷；此前对应测试把错误路径写成期望值。
2. Tauri 前端已有 `tauri-<uuid>` ID，不能把 Wails/CLI 的路径引用问题写成“当前 Tauri 尚无稳定 ID”。缺的是**持久、唯一的 ID→位置与生命周期权威**。
3. `.jsonl.meta` 的 `BranchMeta.ID` 通常由 transcript 文件名得来；现有 event/display index 不含可独立验证的 Tauri 会话 ID。因此三者不能作为“移动文件属于旧 ID”的独立证据。
4. `REASONIX_STATE_HOME` 优先于 `REASONIX_HOME`，同时影响会话目录和身份库。Tauri 只设后者时，继承前者会突破“默认 Preview 隔离”的假设。宿主的单实例插件只约束同一应用实例，不等于任意进程共享 profile 的锁。
5. `tools/repolint/layers.go` 的 leaf 集合含 `internal/store`，**不含** `internal/config`。共享路径函数做成零内部依赖的小包是合理的，但不应以“config 也是 leaf”为论据。

以上 1 和 4 是任何真实 profile 导入之前的阻断项。只读样本检查不等于已修复。

## 3. 身份与路径契约

### 3.1 唯一路径规则

抽出一个无 `reasonix/` 内部依赖的 `internal/sessionpath.TranscriptPath(sessionDir, id)`，精确保留现有 bridge 的 ID 校验、错误语义和 `tauri-<id>.jsonl` 命名。bridge、导入器与测试都调用它；host 不拼 transcript 路径。

导入器只传**实际** `config.SessionDir()`/Controller session dir，工作区仅是元数据。用跨包契约测试固定：ID=`tauri-abc`、任意工作区时，bridge 和导入器都得到 `<state-home>/sessions/tauri-tauri-abc.jsonl`。禁止根据 `workspaceRoot` 推导 transcript 所在目录。

### 3.2 SQLite 模型

`id` 为不可变主键；路径只是可变且必须唯一的定位。`path UNIQUE` **不会**把路径变成身份。v2 的 `session_dir + file_name` 拆分不是必要条件，且若没有合成路径唯一约束，会削弱防冲突能力。

建议在第一次真实写入前，将存储路径定为相对规范化 state root 的 `relative_path TEXT NOT NULL UNIQUE`。现有 Preview 的值应形如 `sessions/tauri-tauri-abc.jsonl`；恢复整份 profile 到不同绝对位置时无需批量改库。根目录由宿主/sidecar 的同一个明确 profile 解析，不能从数据库内的旧绝对路径反推。对相对路径拒绝 `..`、绝对路径、符号链接逃逸及非法目录；工作区路径仍独立保存。保留 `created_at_ms`、`updated_at_ms` 和 `position`；`position` 维持当前“越小越新”，避免无收益的排序迁移。

生命周期必须至少区分：`reserved`（新 ID、尚未有 transcript）、`ready`、`missing`（已落盘记录后来消失）、`deleting`、`deleted`（保留 ID tombstone，禁止重用）。这是防止 bridge 当前 `os.ErrNotExist → SetFreshSessionPath` 把旧会话静默变空的关键。`missing` 或 `deleting/deleted` 一律不得走“新建空会话”分支；只有刚由 resolver 登记的 `reserved` 可创建。

建议的首次启用 schema 骨架（具体迁移 SQL 另附测试）：

```sql
CREATE TABLE sessions (
  id TEXT PRIMARY KEY,
  relative_path TEXT NOT NULL UNIQUE,
  workspace_root TEXT NOT NULL DEFAULT '',
  title TEXT NOT NULL DEFAULT '',
  position INTEGER NOT NULL,
  state TEXT NOT NULL CHECK (state IN ('reserved','ready','missing','deleting','deleted')),
  created_at_ms INTEGER NOT NULL,
  updated_at_ms INTEGER NOT NULL
);
CREATE INDEX sessions_visible_order ON sessions(state, position, id);
```

存储 API 至少分开 `ImportKnown`（只读校验后登记）、`ReserveNew`、`ResolveForOpen`、`MarkPersisted`、`MarkMissing`、`BeginDelete`、`FinishDelete` 和 `ListVisible`；不能让一个通用 Upsert 顺便改变路径、恢复已删除 ID 或清除 missing。`reserved` 跨重启仍是未落盘新会话，只能按同一 ID 继续；一旦进入 `ready`，文件缺失只能转 `missing`。

现有 `session-state-v1.sqlite` 尚未接入运行流，但“真实 profile 没有数据库”只能通过只读检查确认。**若已经有任何实际身份库或备份，不能原地把 schema v1 改定义**：必须用带版本号的事务迁移，并保留恢复路径。若确认从未真实使用，可在首次接线前调整尚未生效的 schema 与测试。

### 3.3 认领与移动

导入优先级：已有身份库记录 > 有效 workbench catalog ID > 未登记文件的只读候选清单。文件名 `tauri-<id>.jsonl` 可提供候选 ID，但复制、改名、同名冲突时不能自动断定身份。大小/mtime 只用于扫描加速，绝不用于认领。

阶段 1 **不做自动重连或物理移动**。`ErrPathChanged` 是阻止无证据漂移的保护，不是必须马上加 alias 表的理由。要支持显式 Move，先定义它是“认领已搬好的完整 artifact bundle”，还是“应用负责移动整个 bundle”；两者的文件操作、失败恢复不同。届时 alias 仅把旧引用解析到同一 ID，不能把旧路径当作读取 transcript 的后备位置；当前路径和 alias 之间也必须查重。未定义 Move 前保持 `missing` 并要求显式选择。

## 4. 数据库、隔离与备份门禁

- `Open` 在任何可持久修改现有数据库的操作前，先读取并验证 `user_version`、完整性和路径归属；未来 schema/损坏库应拒绝打开，不能“修成空库”。先配置 `busy_timeout`，再做可能取锁的初始化。`journal_mode=WAL` 是持久变更，不能放在“未来 schema 零改动”检查之前；身份库使用 `synchronous=FULL`。测试未来库拒绝时，不只检查版本值，也检查文件和伴随文件未被意外改写。
- managed Preview 启动时必须核对规范化 `REASONIX_HOME`、`REASONIX_STATE_HOME` 与实际 `SessionDir()`/身份库路径。不能让继承的 `REASONIX_STATE_HOME` 指向稳定版目录；要么将 managed state 明确路由到 Preview，要么拒绝启动身份迁移并给出诊断。显式自定义 profile 保留为用户选择，但应标记非隔离且禁止自动导入稳定版。
- 同一 canonical state root 的多进程写入需 profile 级锁或等价的唯一 sidecar 所有权；Tauri 单实例插件不能代替它。锁覆盖数据库操作与 JSONL 写入切换，不仅覆盖 UI。
- 身份库、JSONL、侧车、Markdown 和 host catalog 的备份是**一组跨资源快照**。运行中仅对 SQLite 使用 backup API，不能保证它与 JSONL 同时点一致；完整迁移/回退备份须暂停写入后做，记录 manifest/hash 并演练恢复。禁止只复制活跃 WAL 下的 `.sqlite` 单文件。

## 5. 交付切片与退出条件

### S0：修正路径与隔离（无用户数据写入）

1. 提取共享路径函数；删除导入器的 `projects/<slug>/sessions` 分支，修正原测试。
2. 加 bridge↔导入器同源路径测试、带/不带工作区样本、非法 ID 和越界路径测试。
3. 补 managed Preview 的 state-home 覆盖检查；只读核对实际配置路径。此步不打开/创建真实身份库。

**退出**：测试证明两个路径消费者完全相同；原 workbench 和 JSONL 字节不变；Preview 与稳定版目录无交叉。回退只是撤销代码，无数据迁移。

### S1：清单与旁路导入（UI 仍读 JSON）

1. 用只读命令输出 `id | 候选路径 | 文件状态 | title | workspace | 来源 | 冲突原因`。扫描仅限 Preview 的会话目录，并用 `store.IsSessionTranscriptName` 排除辅助日志；未在 catalog 中的文件只列候选，不自动随机分配 ID。
2. 人工/测试样本核对后，以单事务导入确认的记录；未落盘的新标签跳过，已登记文件消失才标 `missing`。任何路径冲突或越界都使该批次失败，并输出可诊断清单，不写 JSONL。
3. 重复导入、进程中断、损坏行、相同内容不同 ID、同名副本与恢复后 root 改变均有固定测试。阶段 1 仍允许 `workbench-sessions.json` 作为 UI 权威。

**退出**：清单与磁盘逐条一致、两次导入结果和 ID 一致、JSONL/侧车 hash 不变；真实 profile 导入前有可恢复快照。此时身份库尚非运行权威，测试库可删除重建；真实库一旦被引用就不得再“删库重导”。

### S2：shadow 读写与 resolver

1. 新会话先 `reserved` 登记 ID；实际写入后转 `ready`。bridge 的打开/切换/重命名/删除均先解析身份行，再定位文件。无记录的新建与已登记但 `missing` 的恢复必须分开。
2. 一段发布窗口内，host 仍显示 JSON 列表，同时在后台读 SQLite 并比较 ID/标题/工作区/顺序/文件状态；只记录差异统计，不把私密消息写诊断。
3. 对删除使用持久状态机：先标 `deleting` 并阻止新写，再调用既有 `control.RemoveSessionArtifacts`，成功后保留 `deleted` tombstone；重启时重试未完成清理。文件系统与 SQLite 无法组成一个原子事务，不能承诺“同时消失”。
4. 差异为零且缺失/删除/崩溃恢复测试通过后，host 才改为读 bridge 的列表。原 JSON 保留作只读回退输入，停止双向写入，避免两套权威。

**退出**：重启 ID 不变；缺文件不会变空会话；删除失败可续做；同 profile 竞争写者被阻止；新旧列表差异可解释。回退时恢复旧读路径，**保留**已成为权威的身份库和 tombstone，不删除或重分配 ID。

### S3：记忆与检索（独立发布）

已有 `internal/memory` 负责 Markdown 事实、`MEMORY.md` 与 `session-context`。先定义项目/全局/会话作用域和注入预算，再添加 `notes.md`、任务 progress、checkpoint；不要与现有 `REASONIX.md` 或记忆事实双写同一含义。Markdown 是真相源，新增 FTS5 只作可重建检索，手工编辑要能 reconcile。`historycatalog` 已有迁移版本和扫描游标，不因 MiMo 有类似表就再造一套。

## 6. 验证与发布决策

阶段 2 上线前必须覆盖：空历史、长历史、损坏尾行、缺失侧车、同名/同内容复制、文件外移、工作区不同但 session dir 相同、进程被强制终止、SQLite future schema/损坏/WAL 恢复、备份恢复、两进程同 profile、删除中途失败，以及旧 bridge 客户端兼容。必要时用测试 profile 注入故障，不拿真实用户会话做破坏性试验。

审批稿 A1–A6、B3–B5、C1–C5 的边界继续有效；A7 改为“记忆布局待独立设计”，B1 的扫描器改为**只读候选清单**而非自动认领，B2 的 alias 延至 Move 语义确定。审批稿中 Wails 的路径身份与 Tauri 已有 ID 必须分开描述，`C6`/`B5` 的交叉引用也需更正。

**下一步唯一可直接开工的改动是 S0。** 未通过 S0 的同源路径与隔离测试前，不接入真实 profile，也不切换 UI 数据源。

## 7. 代码改动位置

| 切片 | 主要位置 | 限制 |
| --- | --- | --- |
| S0 | 新 `internal/sessionpath/`；`cmd/reasonix-desktop-bridge/core_runtime.go`；`internal/sessionidentity/catalog.go` 与其测试；`desktop/tauri/src/data_profile.rs` | 只修路径/隔离，不打开真实身份库。 |
| S1 | `internal/sessionidentity/store.go`、清单命令与测试；必要时 `internal/config/paths.go` | 先查实际 profile 是否已有数据库，再决定 schema v1 修订或版本迁移。 |
| S2 | bridge 的会话打开/切换/删除与列表接口、Tauri host 的 workbench catalog 适配 | 新接口兼容旧客户端；保留原 JSON 只读回退，不能双写两个权威。 |
| S3 | `internal/memory`、`session-context` 组装与独立 Markdown 检索索引 | 不改缓存稳定的系统前缀；注入走回合尾部并做效果测试。 |
