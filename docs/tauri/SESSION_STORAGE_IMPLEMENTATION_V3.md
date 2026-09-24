# Tauri Preview 会话存储实施稿 v3

> 状态：持续实施中；Preview 身份库已升至 schema v4 并采用相对 profile-root 路径，真实稳定版数据迁移仍未授权。本文保留设计边界与阶段验收记录。
> 对照：本地草稿 `SESSION_STORAGE_PLAN_V2.md`、`MIMO_STYLE_SESSION_STORAGE_APPROVAL.md`，以及已入库的[既有评审](./MIMO_STYLE_SESSION_STORAGE_REVIEW.md)。
> 本稿替代前两份文档的实施建议；它们保留为设计讨论记录。本稿不是“已完成迁移”的声明。
>
> **标注约定（DeepSeek 标注）**：本稿中带 `> [DeepSeek]` 前缀的引用块由 DeepSeek
> 侧添加，用于把 `SESSION_STORAGE_PLAN_V2.md` 的**补充与验证**挂到本稿对应位置。
> 原则：**本稿（V3）是实施稿，V2 不覆盖它的结论**；标注只做三种事——
> ① 记录 V2 已认出、并**采纳本稿**的地方；② 指向 V2 中本稿未展开的规格；
> ③ 指向 V2 附带的行号级证据，供评审独立复核。V3 原有正文未作改动。

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

> **[DeepSeek] 第 1–5 条已由 DeepSeek 侧独立复核，全部成立。**
> 行号级证据与三条"本稿纠正了 V2 何处"的对照表见
> `SESSION_STORAGE_PLAN_V2.md` §13.5：
> - 第 1 条（`projects/<slug>/` 缺陷 + 测试把错误路径写成期望）→ V2 §2.1、§4.2、§13.5 错误 2；
>   补充事实：`config.ProjectSessionDir()` 真实存在且在 CLI/desktop/bot 使用
>   （`internal/config/paths.go:451-462`、`internal/cli/session_machine.go:156`、
>   `internal/bot/gateway.go:2358`），所以缺陷是"按 workspaceRoot 自行推导"而非"目录不存在"。
> - 第 3 条（`.jsonl.meta` 不能作为独立证据）→ V2 §6.4 已整节重写。
>   补充证据：`BranchMeta.ID` 通常由文件名派生或在缺失时由文件名补足
>   （`internal/agent/branch.go:186-195`），但已有侧车值不能视为独立身份凭证；
>   `sessionEventIndex` 只有 `writer_id`
>   （`internal/agent/session_events.go:122-130`），`SessionDisplayIndex` 连它也没有
>   （`internal/agent/session_display_index.go:33-53`）。
> - 第 4 条（`REASONIX_STATE_HOME` 优先于 `REASONIX_HOME`）→ V2 §3.3 与 §13.5 已登记为阻断项。
> - 第 5 条（`layers.go` 的 leaf 表不含 `internal/config`）→ V2 §4.1 已更正；
>   V2 上一版正是写错这一条，由本稿纠正。

## 3. 身份与路径契约

### 3.1 唯一路径规则

抽出一个无 `reasonix/` 内部依赖的 `internal/sessionpath.TranscriptPath(sessionDir, id)`，精确保留现有 bridge 的 ID 校验、错误语义和 `tauri-<id>.jsonl` 命名。bridge、导入器与测试都调用它；host 不拼 transcript 路径。

导入器只传**实际** `config.SessionDir()`/Controller session dir，工作区仅是元数据。用跨包契约测试固定：ID=`tauri-abc`、任意工作区时，bridge 和导入器都得到 `<state-home>/sessions/tauri-tauri-abc.jsonl`。禁止根据 `workspaceRoot` 推导 transcript 所在目录。

> **[DeepSeek] 采纳本稿的"传入实际 session dir"原则。**
> V2 §4.2 原先按 `workspaceRoot` 推导目录的写法已作废，改写后的原则与本段一致：
> 目录是真实存在的（`config.ProjectSessionDir()`），错在**按 workspaceRoot 推导
> 而非沿用写入方的 dir**。本稿这条与 V2 §4.1 的 `internal/sessionpath.TranscriptPath`
> 抽取是同一件事，可合并实施：函数抽取按 V2 §4.1，调用点与测试按本段。

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
  title_source TEXT NOT NULL DEFAULT 'fallback'
    CHECK (title_source IN ('fallback','generated','user','legacy_unknown')),
  title_revision INTEGER NOT NULL DEFAULT 0 CHECK (title_revision >= 0),
  position INTEGER NOT NULL,
  state TEXT NOT NULL CHECK (state IN ('reserved','ready','missing','deleting','deleted')),
  created_at_ms INTEGER NOT NULL,
  updated_at_ms INTEGER NOT NULL
);
CREATE INDEX sessions_visible_order ON sessions(state, position, id);
```

存储 API 至少分开 `ImportKnown`（只读校验后登记）、`ReserveNew`、`ResolveForOpen`、`MarkPersisted`、`MarkMissing`、`BeginDelete`、`FinishDelete` 和 `ListVisible`；不能让一个通用 Upsert 顺便改变路径、恢复已删除 ID 或清除 missing。`reserved` 跨重启仍是未落盘新会话，只能按同一 ID 继续；一旦进入 `ready`，文件缺失只能转 `missing`。

现有 `session-state-v1.sqlite` 尚未接入运行流，但“真实 profile 没有数据库”只能通过只读检查确认。**若已经有任何实际身份库或备份，不能原地把 schema v1 改定义**：必须用带版本号的事务迁移，并保留恢复路径。若确认从未真实使用，可在首次接线前调整尚未生效的 schema 与测试。

> **[DeepSeek] V2 已采纳本稿的 `relative_path` 与状态机；两处补充来自 MiMo 源码复核。**
> - V2 §5 的 schema 已按本稿改写为 `relative_path TEXT NOT NULL UNIQUE` +
>   `state TEXT CHECK(state IN ('reserved','ready','missing','deleting','deleted'))`，
>   并给出本段要求的动作分离 API（`ImportKnown/ReserveNew/ResolveForOpen/MarkPersisted/
>   MarkMissing/BeginDelete/FinishDelete/ListVisible`），另加"禁止通用 Upsert 兼办多件事"。
> - 本稿的 schema 未含标题溯源列。V2 §5 依 MiMo 源码补了
>   `title_source ∈ {user,generated,fallback}` + `title_revision`（CAS）：
>   MiMo 的守卫是显式的（`packages/opencode/src/session/projectors.ts:95-99`），
>   "user 标题不可被 generated 降级"。**建议本稿采纳这两列**，否则将来接入自动标题时
>   生成结果会覆盖用户手改的标题，且无 CAS 可依。
>   **完整提案（含精确 diff、4 条写入守卫、3 条可失败测试、影响面与待裁决点）见
>   [SESSION_STORAGE_V3_TITLE_PROVENANCE_PROPOSAL.md](./SESSION_STORAGE_V3_TITLE_PROVENANCE_PROPOSAL.md)。**
>   补充事实：**项目里已经有两条标题写入路径**——用户手工 `agent.RenameSession`，
>   以及 AI 生成的 `desktop/session_ai_title.go:22-70`（底层
>   `internal/control/session_title.go:30`），后者已自行实现"读取-比较-写入"。
>   所以这不是"将来接入"的问题，而是"同一条规则已有两处实现、Preview 会成为第三处"。
> - `relative_path` 的相对根必须由宿主与 sidecar 的同一个 profile 解析，**不得从库内旧值反推**；
>   并需拒绝 `..`、绝对路径、符号链接逃逸（V2 §5 已列入构造函数职责）。

**标题溯源裁决（覆盖上方三态提案）**：接受 `title_source` 与
`title_revision`，但新增 `legacy_unknown`，且新行默认 `fallback`。旧 catalog 的非空标题
可能是手工、首条消息派生或侧车回填，导入时统一标为 `legacy_unknown` 并保护；空标题
标为 `fallback`。首条消息派生标题仍属 `fallback`，后台自动标题是 `generated`，
手工重命名和用户主动要求的 AI 重命名具有 `user` 权限。标题操作由 bridge/存储层的
**操作类型**决定，不允许 host 自称 `title_source`。每次成功更新用 SQLite 的
`title_revision` CAS；后台生成不得覆盖 `user` 或 `legacy_unknown`。重复导入只能登记
身份或回填未初始化标题，不能用旧 catalog 覆盖已登记行的标题。UI 占位文案不写入库。

**权威切换门禁**：当前 `.jsonl.meta`、host catalog 和身份库尚未有跨资源原子提交。
在 SQLite 成为标题权威前，必须明确侧车兼容镜像、失败后的对账规则，以及 bridge 的
读取路径；不可把增加两列视为完成切换。现有 v1 身份库升级必须使用版本化事务迁移，
非空旧标题保守回填为 `legacy_unknown`，不得重建真实库。`position` 规则保持不变。

### 3.3 认领与移动

导入优先级：已有身份库记录 > 有效 workbench catalog ID > 未登记文件的只读候选清单。文件名 `tauri-<id>.jsonl` 可提供候选 ID，但复制、改名、同名冲突时不能自动断定身份。大小/mtime 只用于扫描加速，绝不用于认领。

阶段 1 **不做自动重连或物理移动**。`ErrPathChanged` 是阻止无证据漂移的保护，不是必须马上加 alias 表的理由。要支持显式 Move，先定义它是“认领已搬好的完整 artifact bundle”，还是“应用负责移动整个 bundle”；两者的文件操作、失败恢复不同。届时 alias 仅把旧引用解析到同一 ID，不能把旧路径当作读取 transcript 的后备位置；当前路径和 alias 之间也必须查重。未定义 Move 前保持 `missing` 并要求显式选择。

> **[DeepSeek] 确认：V2 已撤回"阶段 1 就带 alias"的主张，改与本稿一致。**
> V2 §13.5 采纳的第 3 条即本段，`MIMO_STYLE_SESSION_STORAGE_APPROVAL.md` 的 B2 时机分歧
> 就此闭合（V2 §13.4 第 1 条已标闭合）。另外补一条 V2 §6.3 的实现约定：
> MiMo 的 `memory.md` 单文件 rename 可作为并发场景的参考，不能直接移植为会话 Move
> 的成功判据。Move 涉及 JSONL、侧车及 SQLite 定位/alias；单个文件的 `ENOENT`
> 不能证明整个 artifact bundle 已被另一写者成功移动，必须通过完整状态核验和恢复记录。

## 4. 数据库、隔离与备份门禁

- `Open` 在任何可持久修改现有数据库的操作前，先读取并验证 `user_version`、完整性和路径归属；未来 schema/损坏库应拒绝打开，不能“修成空库”。先配置 `busy_timeout`，再做可能取锁的初始化。`journal_mode=WAL` 是持久变更，不能放在“未来 schema 零改动”检查之前；身份库使用 `synchronous=FULL`。测试未来库拒绝时，不只检查版本值，也检查文件和伴随文件未被意外改写。
- managed Preview 启动时必须核对规范化 `REASONIX_HOME`、`REASONIX_STATE_HOME` 与实际 `SessionDir()`/身份库路径。不能让继承的 `REASONIX_STATE_HOME` 指向稳定版目录；要么将 managed state 明确路由到 Preview，要么拒绝启动身份迁移并给出诊断。显式自定义 profile 保留为用户选择，但应标记非隔离且禁止自动导入稳定版。
- 同一 canonical state root 的多进程写入需 profile 级锁或等价的唯一 sidecar 所有权；Tauri 单实例插件不能代替它。锁覆盖数据库操作与 JSONL 写入切换，不仅覆盖 UI。
- 当前 bridge 在发布 ready 前获取 `internal/profilegate` 的独占锁，运行时关闭后才释放；同一 profile 的第二个**已接入锁**的 bridge 会拒绝启动。此门禁只覆盖使用该协议的新进程，不能检测或停止 1.38.3/1.38.10 等旧写者。真实 profile 的离线快照、导入与回退仍须由外部确认所有旧写者已退出；不得把获取新锁视为静默迁移许可。
- 身份库、JSONL、侧车、Markdown 和 host catalog 的备份是**一组跨资源快照**。运行中仅对 SQLite 使用 backup API，不能保证它与 JSONL 同时点一致；完整迁移/回退备份须暂停写入后做，记录 manifest/hash 并演练恢复。禁止只复制活跃 WAL 下的 `.sqlite` 单文件。

## 5. 交付切片与退出条件

### S0：修正路径与隔离（无用户数据写入）

1. 提取共享路径函数；删除导入器的 `projects/<slug>/sessions` 分支，修正原测试。
2. 加 bridge↔导入器同源路径测试、带/不带工作区样本、非法 ID 和越界路径测试。
3. 补 managed Preview 的 state-home 覆盖检查；只读核对实际配置路径。此步不打开/创建真实身份库。

**退出**：测试证明两个路径消费者完全相同；原 workbench 和 JSONL 字节不变；Preview 与稳定版目录无交叉。回退只是撤销代码，无数据迁移。

> **[DeepSeek] S0 建议追加一条退出条件：`resumeBridgeSession` 的静默空会话路径必须已被设计覆盖。**
> `cmd/reasonix-desktop-bridge/core_runtime.go:65-68` 在 `agent.LoadSession` 返回
> `os.ErrNotExist` 时**无条件** `controller.SetFreshSessionPath(path)`：已登记且曾落盘的
> 会话，其 transcript 被删/移后再打开，会**静默变成同 ID 的空会话**。
> 本稿 §3.2 的状态机正是它的解药（`missing` 不得走新建分支）。
> 由于 S0 不写身份库，S0 只需记录该缺陷并加一条失败测试作为基线，
> 真正的修复落在 S2；V2 §13.5 已把这条登记为"V3 发现、V2 遗漏"的缺陷。

### S1：清单与旁路导入（UI 仍读 JSON）

1. 用只读命令输出 `id | 候选路径 | 文件状态 | title | workspace | 来源 | 冲突原因`。扫描仅限 Preview 的会话目录，并用 `store.IsSessionTranscriptName` 排除辅助日志；未在 catalog 中的文件只列候选，不自动随机分配 ID。
2. 人工/测试样本核对后，以单事务导入确认的记录；未落盘的新标签跳过，已登记文件消失才标 `missing`。任何路径冲突或越界都使该批次失败，并输出可诊断清单，不写 JSONL。
3. 重复导入、进程中断、损坏行、相同内容不同 ID、同名副本与恢复后 root 改变均有固定测试。阶段 1 仍允许 `workbench-sessions.json` 作为 UI 权威。

**退出**：清单与磁盘逐条一致、两次导入结果和 ID 一致、JSONL/侧车 hash 不变；真实 profile 导入前有可恢复快照。此时身份库尚非运行权威，测试库可删除重建；真实库一旦被引用就不得再“删库重导”。

**当前实现边界**：只读 inventory 与 `PrepareImportReview` / `ApplyImportReview` 已在存储层落地。
审核计划必须显式列出 catalog ID，并包含 catalog 与每个选中 transcript 的 SHA-256；
应用前重新核对，冲突整批拒绝，未选中的扫描文件不认领。当前没有对真实 profile
暴露导入端点；bridge 已有仅供新版本协作的 profile 锁，但没有旧写者停写确认或已接入的跨资源备份门禁；因此这只是离线/测试 profile
的 S1 能力，不能在运行中的 Preview 上直接执行真实数据迁移。

离线快照工具现可把**整份** Preview profile 与独立的 workbench catalog 复制到 profile
之外的私有目录，manifest 逐文件记录大小和 SHA-256，完成后可验证并在**新目录**演练恢复；
符号链接、嵌套备份位置、缺失或被篡改的成员会被拒绝。它尚未接入用户迁移入口，
也**不能替代停写门禁**：旧客户端不认识新 profile 锁，快照 API 只适用于已由外部确认
全部写者停止的离线 profile。真实资料备份/恢复演练与旧写者停写确认仍待完成。

> **[DeepSeek] S1 的两份未展开规格在 V2，可直接引用而不必重写：**
> - **清单字段**：V2 §7.1 给出列定义（`id | 来源(workbench/scan) | 推导路径 | 磁盘是否存在 |
>   判定 | 标题 | workspace_root`），并要求单列一节 **"未认领文件"**——扫到但无法确定性
>   推导 ID 的文件不进库但必须报出，否则用户会以为身份库已覆盖全部会话。
> - **扫描器规则**：V2 §6.6 给出只读范围（`sessions/` 一层 + `store.IsSessionTranscriptName`
>   排除 `.events/.turns/.conflicts/.guardian`）、确定性 ID 推导与往返校验、
>   只读 SHA-256 前后比对、以及 workbench 优先的合并优先级。
> - **两类容错必须分开**（V2 §6.1 表）：已登记 ID 的路径变了 → **整体失败、一条不写**；
>   单条候选不可读/父实体缺失 → **跳过并计入错误清单**，其余照常提交。
>   本稿 S1.2 写的是"任何路径冲突或越界都使该批次失败"，与第一类一致；
>   请确认第二类也按"跳过 + 报出"而不是整体失败，否则一个坏文件会挡住全部导入。
> - **一致性提醒**：MiMo 的批量导入对孤儿记录是*跳过 + 计数 + warn*
>   （`packages/opencode/src/storage/json-migration.ts:238`），对批内失败是
>   *保存点回滚 + 记 `errors[]` 后继续*（同文件 `102-112`）。V2 §14.1 有逐条行号。

### S2：shadow 读写与 resolver

**当前安全切片**：bridge 已将新建、恢复、缺失、删除中与已删除会话分开处理；已登记但 transcript 缺失时拒绝静默创建同 ID 空会话，缺失与中断删除均可经显式删除安全退休并保留 tombstone。Tauri Preview 侧栏按页读取身份目录，但首屏只有在 count-only 影子比对确认一致时才选 SQLite；比对不一致或失败时回退旧 JSON 目录，且新增/改名/删除后的重新盘点发现漂移时也会回退。身份表现为相对 state-root 的 `relative_path`；旧 v1–v3 绝对路径通过校验后事务迁移到 v4，离线快照与旧 catalog 重放的跨资源恢复演练已覆盖新 profile 路径。此为可回退的 Preview 读取路径，不代表稳定版迁移或全 profile 权威切换。旧 bridge 缺少必需的 `session_catalog_sync` capability 时，新 host 会在启动阶段拒绝该 sidecar；真实旧 writer 停写确认与旧 Tauri host 二进制认证仍待完成，详见下方兼容矩阵。

### 旧客户端 / sidecar 兼容矩阵（当前验证范围）

| 组合 | 结论 | 依据 / 限制 |
| --- | --- | --- |
| 当前 Tauri host + 当前 bridge（protocol v1，含 `session_catalog_sync`） | 支持 | Host 启动时先校验 ready frame，再读取认证 health；协议版本、sidecar instance ID 或会话目录同步 capability 不匹配时，拒绝把该进程登记为可用并终止它。 |
| 当前 Tauri host + 旧 bridge（仍报 protocol v1、但不含 `session_catalog_sync`） | 明确拒绝 | protocol major 相同不代表新增 endpoint 可用；health capability 缺失会在启动阶段报错，不等到初次目录同步才失败。 |
| 旧 Tauri host + 新 bridge（protocol v1） | 预期向后兼容，非发布认证 | bridge 保留既有 v1 路由；旧 host 不调用新增目录同步接口时，不会要求它理解身份库。完整旧 host 二进制尚未纳入自动化矩阵。 |
| Wails 1.38.3 / 1.38.10 与 Tauri Preview 共用 profile 并同时写入 | 不支持 | 这些旧 writer 不遵守 `profilegate`。不得用新 bridge 的锁推断旧进程已停；真实 profile 操作前需外部确认 writer 全退出。 |
| 稳定版 Wails profile 顺序复制到隔离的 Preview，再离线导入 | 有条件支持，需人工核对 | 只对副本执行 inventory、快照、逐项审核导入与恢复演练；不在稳定目录就地迁移、不让两个版本并发写。 |

自动化目前覆盖 capability 缺失/instance 不匹配拒绝、当前协议 health 响应、当前 Rust host 启动并关闭真实 Go bridge、sidecar 意外退出后的重启握手、Rust host 经真实 bridge 同步会话工作区/顺序并读回 SQLite、真实测试 profile 的身份目录与磁盘盘点、旧 catalog 导入和相对路径恢复；没有真实 1.38.3/1.38.10 writer 停写证明，也没有旧 Tauri host 二进制的端到端认证。仓库没有随项目跟踪的旧 Tauri 应用包；`50c1b9bc6` 是当前历史中存储改造开始前最近的 Tauri host 提交，可作为**候选源码基线**，但不是已认证的发布二进制。将“预期向后兼容”升级为“发布认证”前，需从该基线构建并记录 host 二进制哈希，在一次性隔离 profile 中与当前 bridge 做端到端启动及会话操作，再按实际发布版本补充二进制矩阵；若找到对应发布包，应优先认证发布包。仅凭协议版本号或源码兼容推断不能替代这项验证。

**候选基线预检（2026-09-24）**：从 `50c1b9bc6` 的源码构建出了 arm64 host（SHA-256 `d24e8031e8b5e2b4a02dcb7249585ae6912621a46f098fd3daf975943ae91b6a`），并用当前 bridge（构建源码基线 `c4ce11fe5`）通过了旧 `BridgeSupervisor` 的真实进程启停、改名后重启读取、删除隔离会话三项测试。候选 host Mach-O 也已在临时 `REASONIX_HOME`/`REASONIX_STATE_HOME` 下启动，并拉起当前 bridge；退出后确认没有遗留进程。所有运行均使用 `/private/tmp` 下的隔离 profile。改名、重启读取和删除操作是在 Cargo test harness 中执行，尚未通过应用窗口操作会话，也没有认证签名/发布包，因此兼容矩阵仍保持“预期向后兼容，非发布认证”。

可用 `bash tools/tauri/verify-legacy-host.sh` 重建这组候选验证：脚本从 `50c1b9bc6` 和运行时 `HEAD` 的归档源码开始，在新的 `/private/tmp` 工作目录中构建 bridge、运行上述三项旧 host 集成测试并构建候选 host；它不会读取或修改真实 profile，也不会清理生成目录。脚本输出 bridge/host 源码基线、候选 host SHA-256 和产物目录。该脚本只复现源码基线与测试 harness 验证，不验证应用窗口交互、签名、公证或实际发布包；执行环境还需允许测试 host 绑定 loopback 端口。

**脚本复现（2026-09-24）**：在允许 `127.0.0.1` loopback 的运行环境中，三项 host 集成测试均通过；bridge 源码基线为 `251bb55e3ba069918a463403d365df25aac465bf`，本次 debug host SHA-256 为 `accf0273d96338ed6071ff0c5f73364eddf7f9ab0205cf6c720f060c83c7281a`，产物与独立 profile 保留在 `/private/tmp/reasonix-tauri-compat.7tpjTY`。该哈希对应本次 debug 构建，不代表稳定发布二进制；兼容矩阵仍保持“预期向后兼容，非发布认证”。

1. 新会话先 `reserved` 登记 ID；实际写入后转 `ready`。bridge 的打开/切换/重命名/删除均先解析身份行，再定位文件。无记录的新建与已登记但 `missing` 的恢复必须分开。
2. 一段发布窗口内，host 对比 JSON 与 SQLite 的 ID/标题/工作区/顺序/文件状态；只记录差异统计，不把私密消息写诊断。SQLite 只在影子报告 clean 时作为当前 Preview 侧栏来源，否则继续显示 JSON；差异或盘点错误不得进入 SQLite 来源。
3. 对删除使用持久状态机：先标 `deleting` 并阻止新写，再调用既有 `control.RemoveSessionArtifacts`，成功后保留 `deleted` tombstone；重启时重试未完成清理。文件系统与 SQLite 无法组成一个原子事务，不能承诺“同时消失”。
4. 首屏 clean 门禁与分页读取已接入 Preview；新增/改名/删除后的重新盘点若发现差异会触发回退。只有缺失/删除/崩溃恢复、损坏库、备份恢复及双进程测试完成后，才能考虑移除 JSON 回退并宣布 SQLite 为唯一权威。当前保留旧 JSON 回退输入与排序同步，避免在验证窗口丢失可用列表。

**退出**：重启 ID 不变；缺文件不会变空会话；删除失败可续做；同 profile 竞争写者被阻止；新旧列表差异可解释。回退时恢复旧读路径，**保留**已成为权威的身份库和 tombstone，不删除或重分配 ID。

### S3：记忆与检索（独立发布）

已有 `internal/memory` 负责 Markdown 事实、`MEMORY.md` 与 `session-context`。先定义项目/全局/会话作用域和注入预算，再添加 `notes.md`、任务 progress、checkpoint；不要与现有 `REASONIX.md` 或记忆事实双写同一含义。Markdown 是真相源，新增 FTS5 只作可重建检索，手工编辑要能 reconcile。`historycatalog` 已有迁移版本和扫描游标，不因 MiMo 有类似表就再造一套。

## 6. 验证与发布决策

阶段 2 上线前必须覆盖：空历史、长历史、损坏尾行、缺失侧车、同名/同内容复制、文件外移、工作区不同但 session dir 相同、进程被强制终止、SQLite future schema/损坏/WAL 恢复、备份恢复、两进程同 profile、删除中途失败，以及旧 bridge 客户端兼容。必要时用测试 profile 注入故障，不拿真实用户会话做破坏性试验。

审批稿 A1–A6、B3–B5、C1–C5 的边界继续有效；A7 改为“记忆布局待独立设计”，B1 的扫描器改为**只读候选清单**而非自动认领，B2 的 alias 延至 Move 语义确定。审批稿中 Wails 的路径身份与 Tauri 已有 ID 必须分开描述，`C6`/`B5` 的交叉引用也需更正。

S0 路径与隔离修正已提交；S1 的只读清单和离线核验导入已有代码与测试，
离线快照会校验整份 profile 与 host catalog，暂存快照后的身份路径重绑定及 catalog 对恢复 transcript 的重放已有自动化演练。身份库 schema v4 已把绝对 `path` 迁为相对 `relative_path`，v1–v3 迁移会先核对路径归属，越界时拒绝迁移且不改 v3 数据。当前 host 已在启动时校验 `session_catalog_sync` capability，旧 v1 sidecar 缺能力会被拒绝；上方兼容矩阵记录了可证明与仅预期兼容的组合。仍待旧 writer 停写确认及旧 Tauri host 二进制端到端认证；新版本 bridge 同 profile 进程互斥已有跨进程测试，但不约束不使用新锁协议的旧版本 writer。Preview 仅在首屏影子报告 clean 时选择身份目录；启动或新增、
改名、删除后若重盘点发现漂移或失败，则回退 JSON。稳定版及真实 profile 自动迁移仍未授权。

> **[DeepSeek] 本段的 A/B/C 编号与两处更正已并入 V2 的决议索引。**
> - V2 §13.1 声明**直接采纳** A1–A7、B3–B5、C1–C5，并注明 A7 的
>   "已有 `REASONIX.md` 不迁移、双轨写入需 RFC"这一限定必须保留。
> - V2 §13.2/§13.3 记录了 V2 与 APPROVAL 的覆盖/补充关系；
>   本段所指的 `C6`/`B5` 交叉引用更正，V2 §13.3 末行已标注"上游规模数据只说明
>   变更规模与修复密度，不能单独证明某架构是崩溃原因"，沿用同一限定。
> - **编号不要混读**：APPROVAL 用 A/B/C/Q，V2 新增的问题用 V1–V6；
>   评审请分别表态（V2 §10 有对照表）。

## 7. 代码改动位置

| 切片 | 主要位置 | 限制 |
| --- | --- | --- |
| S0 | 新 `internal/sessionpath/`；`cmd/reasonix-desktop-bridge/core_runtime.go`；`internal/sessionidentity/catalog.go` 与其测试；`desktop/tauri/src/data_profile.rs` | 只修路径/隔离，不打开真实身份库。 |
| S1 | `internal/sessionidentity/store.go`、清单命令与测试；必要时 `internal/config/paths.go` | 先查实际 profile 是否已有数据库，再决定 schema v1 修订或版本迁移。 |
| S2 | bridge 的会话打开/切换/删除与列表接口、Tauri host 的 workbench catalog 适配 | 新接口兼容旧客户端；保留原 JSON 只读回退，不能双写两个权威。 |
| S3 | `internal/memory`、`session-context` 组装与独立 Markdown 检索索引 | 不改缓存稳定的系统前缀；注入走回合尾部并做效果测试。 |
