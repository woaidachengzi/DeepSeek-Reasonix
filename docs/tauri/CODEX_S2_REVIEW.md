# Codex 未提交改造审查报告（S2：持久会话目录与项目树）

> 审查对象：`experiment/tauri` 工作区中**未提交**的改动（约 3,400 行 / 39 个已跟踪文件 + 2 个未跟踪文件），
> 提交基线 `c8a2a8deb`。
>
> 审查方式：**只读代码审查，未修改任何文件**。所有结论均给出文件与行号，可逐条复核。
> 两轮读取覆盖：`path.go`、`store.go`、`delete.go`、`inventory.go`、`reviewed_import.go`、
> `offline_snapshot.go`、`catalog.go`、`readonly.go`、`session_list.go`、`session_delete_recovery.go`、
> `session_catalog_import.go`、`session_catalog_sync.go`、`project_folders.go`、`core_runtime.go`、
> `session_previews.go`、`main.rs`、`bridge.rs`、`workbench_catalog.rs`、`workbench_projects.rs`、
> `workbenchSessions.ts`、`TauriChatWorkspace.tsx`、`crash_recovery_test.go`、`inventory_safety_test.go`。

## 0. 总体判断

骨架方向正确，且有几处**做得比我预期更严谨**（见 §5）。但存在 **4 个应当先修的功能性缺陷**，
其中 2 个会让本项的核心目标（解除 50 条上限、缺文件不重开成空会话）在实际使用中失效或打折扣。

| 严重度 | 数量 | 摘要 |
| --- | --- | --- |
| **高（功能）** | 4 | 中断删除无续做入口；标题变化使分页失效；影子校验失败退化为 50 条且不再分页；每页全量重扫 |
| **中（性能/一致性）** | 8 | 每次变更 O(n) 次 syscall；打开库可能整库拷贝；计数语义与实际写入不符；路径归一化三层不一致等 |
| **低（细节）** | 7 | 空 400 响应、死代码、重复计算、平台策略无提示等 |

---

## 1. 高严重度问题

### 1.1 被中断的删除没有重启续做路径，会话会"消失"

**证据**

- 设计稿明确要求（`SESSION_STORAGE_PLAN_V2.md` §7 阶段 2 退出条件；本仓库
  `SESSION_STORAGE_S2_PERSISTENT_CATALOG_DESIGN.md` §4 步骤 5.4）："删除中途 kill → 重启后清理完成或可重试"。
- 但恢复函数只从 DELETE 端点可达：
  - `cmd/reasonix-desktop-bridge/session_delete_recovery.go:27` `retryInterruptedSessionDelete`
  - 唯一调用点：同文件 `:19` `deleteOwnedOrInterruptedSession`，它由删除端点调用。
- 全局搜索 `Reconcile` / `resumePendingDelete` / 启动期扫描：**无命中**。
- 而 `deleting` 被所有列表排除：
  - `internal/sessionidentity/store.go:813`、`:825`（`state NOT IN ('deleting','deleted')`）
  - `internal/sessionidentity/catalog.go:108`、`:118`

**后果**：删除过程中进程被杀 → 行停在 `deleting` → 该会话**不在任何列表里**，用户既看不到也无法选中它去重试删除；
只有恰好用同一 ID 再发一次 DELETE 才会续做。这与"重启恢复"要求不符。

**建议**：bridge 启动时（或首次列表请求惰性触发）扫描 `state='deleting'` 的行并逐个续做
`RemoveSessionArtifacts` + `FinishDelete`；失败则记录诊断并保留 `deleting` 等下次重试。

### 1.2 标题变化会让分页失效并提示"重启列表"

**证据**

- `internal/sessionidentity/store.go:884-902` `visibleSnapshotID` 把 `title` 与 `updated_at_ms` 一起哈希进快照 ID。
- 续页时 `ListVisible` 比较快照：`store.go:820-822` 不一致返回 `ErrDirectoryChanged`。
- 桥接把它翻成 409 `resync_required`：`cmd/reasonix-desktop-bridge/session_list.go:99-102`。
- host 转成错误文案"restart the session list"：`desktop/tauri/src/main.rs:492-494`、`:556`。
- 这条行为被测试固化为期望：`internal/sessionidentity/store_test.go:397-402`（改标题 → 期望 `ErrDirectoryChanged`）。

**后果**：任何一次重命名、或**首轮标题回填**，都会让正在翻页的用户收到"列表已变化，请重启"。
由于标题回填在正常使用中持续发生，可能出现反复触发、无法翻到第二页。

**建议**：快照只覆盖结构性字段（`id`、`position`、`state`，必要时 `updated_at_ms`），
把 `title` 从快照剔除——标题变化通过分页响应本身更新即可；若确实要检测标题漂移，
应作为诊断字段返回而不是让分页失败。

### 1.3 影子校验失败后退回上限 50 的旧目录，且不再分页

**证据**

- 每页都重新校验：`desktop/tauri/src/main.rs:462-463`（`compare_session_catalog_with_directory`）。
- 校验通过才用身份库分页；否则走 `legacy` 分支并把游标置空：
  `main.rs:498-518`（`page_entries_from_legacy(legacy_sessions, ...)` + `next_cursor: None`）。
- `legacy_sessions` 来自 host 的 JSON 目录，硬上限 50：`desktop/tauri/src/workbench_catalog.rs:12`（`MAX_SESSIONS = 50`）。
- 校验依据：`main.rs:691-692` `identity_session_catalog_is_verified` 要求
  `report.legacy_matches_directory`，其定义为 `missing_from_directory == 0`
  （`desktop/tauri/src/session_shadow.rs:114`）。

**后果**：只要有一处不一致（例如某个 legacy 条目尚未导入身份库），侧栏就**永久停在 50 条且无分页**，
与本项"解除 50 条上限"的目标直接冲突；用户得不到任何"数据其实更多"的提示。

**建议**：区分"结构性不一致"与"可解释差异"；至少让 legacy 分支也保留分页游标，
并在 UI 上显式标注"当前显示兼容列表（最多 50 条）"。

### 1.4 宿主每翻一页都做全量扫描与全量比对

**证据**

- 每页调用：`main.rs:462-463` → `compare_session_catalog_with_directory`（`main.rs:680-689`）内部：
  - `session_directory_snapshot_with_id`（`bridge.rs:803-847`）：**按 200/页循环翻完整个目录**；
  - `session_physical_inventory`（`bridge.rs:851-856`）：完整拉取一次 `/v1/sessions/inventory`。
- 列表页本身还有 1/3 处额外全量：`session_list.go` 每个请求都做 `COUNT(*)`（`:812`）＋
  `visibleSnapshotID` 全表扫描（`:863-911`）。

**后果**：翻 k 页 ≈ k 次全量扫描。读满一个 2,000 条的列表需要约 2,000×10 次行读取；
代码自身设了 `MAX_SHADOW_SESSIONS = 10_000`（`bridge.rs:30`），说明作者预期会到万级，该量级下不可行。

**建议**：影子校验只在"首次列表/显式刷新"时做一次并缓存结果；
分页路径不应触发全量比对；`visibleSnapshotID` 改为只读结构列（`id/position/state`）而不扫全列。

---

## 2. 中严重度问题

### 2.1 每次会话变更都做 O(n) 次文件系统调用

`ensureTranscriptPathAvailable`（`store.go:412-463`）遍历**所有**会话，对每行调用
`resolveTranscriptPath`（内部再 `EvalSymlinks` 一次，`:391-405`）＋ `os.Stat` ×2 ＋ 可能的 `SameFile`。
它在 `Reserve`／`MarkReady`／`BeginDelete`／`FinishDelete`／`Import` 中都被调用，即
**每次开会话、首次落盘、每次删除**都付这个代价（n=1000 时每次操作数千次 syscall）。
建议先用 `relative_path` 的唯一约束挡住绝大多数冲突，只对"物理同一文件"的少数情形
（存在 symlink/hard link 或大小写差异）按需做昂贵的物理比较。

### 2.2 打开身份库时可能整库拷贝，且位于热路径

`validateExistingIdentityDatabase` → `inspectWALSchemaOnCopy`（`path.go:128-186`）：
只要 `-wal` 存在且非空，就把 `.sqlite`＋`-wal`＋`-journal` 复制到临时目录再打开校验。
`Open` 出现在：`core_runtime.go:135`（每次开会话）、
`core_runtime.go:95-117`（**每个事件**都调用 `Open`，直到 `MarkReady` 成功为止）、
`main.go:129`、`session_catalog_*.go`、`session_delete_recovery.go:48`。
建议改为只读元数据探测（如读 header 的 `user_version` 字段，已在 `:124` 做过），
把昂贵的 WAL 校验限制在"检测到 schema 异常"之后。

### 2.3 `Accepted` / `Synced` 计数与"实际写入"不符

- `session_catalog_import.go:84-87` 把 `Accepted` 直接写成 `len(candidates)`，即使
  `ImportLegacyCatalog` 跳过了（例如 `preserveMissing` 未落盘的条目）也照样计入。
- `catalog.go:127-133、145` 的 `listedPosition` 只统计"在身份库中确实存在"的条目，
  而 host 用 `require_complete_workbench_sync`（`main.rs:651-657`）要求它等于 legacy 条数，
  于是**合法的跳过会被判成"同步不完整"**。

### 2.4 路径归一化三层不一致（潜在：按工作区过滤会查不到会话）

- Go 侧按大小写**敏感**精确匹配：`store.go:813`（`workspace_root=?`）。
- Rust 侧 host 的组键在 Windows 上**小写化**并统一分隔符：`workbench_projects.rs:158-173`（`normalized_project_key`）。
- 前端组键同样小写化：`desktop/frontend/src/tauri/workbenchSessions.ts:15-25`（`workbenchProjectKey`）。
- 身份库里存的是 `filepath.Abs` 的原始大小写：`catalog.go:56`、`session_catalog_import.go:59`。

目前**尚未暴露**，因为前端分页只传 `limit`/`cursor`（`tauriBridge.ts:270`），没有使用工作区过滤；
`bridge_session_directory_page`（`main.rs:575`）虽已注册但**前端无任何调用点**。
一旦按工作区取会话（正是"完整项目树"想要的能力），大小写不匹配会让某个项目组查出 0 条。
建议在写入与查询两侧统一采用同一种规范化（或改用大小写不敏感比较）。

### 2.5 离线快照对同一份数据读 4 遍

`copyVerifiedSnapshotFile`（`offline_snapshot.go:430-479`）每次拷贝都做：写入后哈希目标文件
（`:470`）＋ 再哈希源文件（`:474`）；`CreateOfflineSnapshot` 结束时又整体 `VerifyOfflineSnapshot`
（`:153`，再哈希全部成员一次）。加上读取源文件本身 = **每个字节读 4 遍**。
对带大量 transcript 的 profile，一次备份的耗时与 I/O 都显著偏高。建议至少合并
"拷贝后校验"与"最终验证"（校验一次即可，因为中间没有写入者）。

### 2.6 `ApplyImportReview` 的 TOCTOU 无法只用指纹消除

`reviewed_import.go:219-256`：先 `PrepareImportReview`（对每个 transcript 做 SHA-256），
把 `plan` 与 `fresh` 做 `reflect.DeepEqual` 后调用 `importCandidates`。
注释要求"调用方必须先静默 transcript 写入者并持有 profile 级所有权锁"（`:220-222`），
但**该要求没有代码强制**：一个忘了持锁的调用方仍会导入"审查后又变化"的 transcript。
建议把 profile 锁做成函数内的前置条件（或要求传入已持有的锁句柄），而不是注释约定。

### 2.7 `SyncWorkbenchOrder` 的 50 条硬限制

`catalog.go:32`：`len(entries) > 50` 直接报错。这是 legacy 目录的上限，本身没写错，
但它**写死了"身份库的有序前缀只能是 50 条"**（`:104-124` 的 ordered 逻辑依赖它）。
一旦 host 目录扩容（本项的目标之一），同步会直接失败而不是增量处理。

### 2.8 恢复路径用读写 `Open`，与只读消费方不一致

`session_delete_recovery.go:48` 使用 `sessionidentity.Open`（读写、会迁移 schema），
而同期的列表与清单使用 `OpenReadOnly`（`session_list.go:90`、`readonly.go` 的 `mode=ro` ＋ `query_only`）。
删除恢复确实要改状态，但**一次失败重试也可能触发 schema 迁移**；
若这是有意取舍，建议在注释中写清，避免后续把只读路径也改成读写。

---

## 3. 低严重度问题

1. **解析失败返回空 400**：`session_catalog_import.go:31-33`、`session_catalog_sync.go:25-27`
   在 `decodeJSONBody` 出错时直接 `return`，响应体为空、code 由 `http.MaxBytesReader` 决定，
   客户端拿不到协议错误。其他处理器统一用 `writeProtocolError(...)`（如 `main.go:537-539`）。
2. **死代码**：`bridge_session_directory_page`（`main.rs:575-613`）已注册为 Tauri 命令但无调用方，
   且内部含整目录快照逻辑（`:620-625`）。
3. **重复计算**：`store.go:401` 调用 `relativeTranscriptPath` 后丢弃返回值，
   而外层紧接着再做 `resolveIdentityPath`——两次 `EvalSymlinks` 只为一处校验。
4. **平台策略无提示**：`path.go:33-38` `sameCaseInsensitivePath` 在 darwin/windows 上把
   仅大小写不同的路径视为同一路径，注释承认"在大小写敏感卷上会拒绝合法的一对"；
   触发时用户只会看到 `ErrTranscriptPathConflict`，不知道是平台策略所致。
5. **快照包含缓存目录**：`CreateOfflineSnapshot`（`offline_snapshot.go:82-117`）整树拷贝 profile，
   含 `cache/`（可重建）。备份体积与耗时被可丢弃数据放大，建议显式排除或单列一个"精简"模式。
6. **`Import` 的 `previewRoot` 语义已过时**：`store.go:521`、`:583` 仍以 `previewRoot` 命名，
   而现在调用方传的是 session dir（`catalog.go:181`、`session_catalog_import.go:80`）。
   命名与文档容易误导下一个改这段代码的人。
7. **`importLegacyCatalog` 的空数组提前返回不带 `Accepted` 字段语义**：
   `session_catalog_import.go:38-41` 返回 `Accepted` 默认 0，与"确实写入了 0 条"无法区分，
   虽不影响校验（host 用 `SyncWorkbenchOrder` 的返回值），但语义含糊。

---

## 4. 已确认**没有**问题的地方（避免重复排查）

| 项 | 结论 |
| --- | --- |
| 路径规则是否又分叉 | **没有**。生产代码中构造 `tauri-<id>.jsonl` 只有 `internal/desktopbridge/sessionpath`；其他命中都在测试里 |
| 缺文件重开成空会话（5.0 核心修复） | **已修好**。`core_runtime.go:141-201` 分支完整：已登记 `ready`＋文件缺失 → `MarkMissing` 后报错；未登记但有残余 → 拒绝；仅"无记录且无残余"才 `SetFreshSessionPath` |
| 删除的 fencing | **正确**。`delete.go:31-40`、`:79-96` 用 `UPDATE ... updated_at_ms=updated_at_ms` 先取写锁再读状态，规避 deferred 快照无法升级；`FinishDelete` 在 transcript 仍存在时拒绝落 tombstone |
| 状态机非法迁移 | `MarkReady` 只接受 `reserved`、`MarkMissing` 只接受 `ready`、`BeginDelete` 接受 `reserved/ready/missing`，非法迁移均返回 `ErrSessionStateConflict` |
| 离线快照的安全校验 | 完整：成员哈希＋尺寸、清单完整性双向比对（磁盘↔清单）、符号链接拒绝、私有权限检查、恢复暂存区在源之外 |
| 目录完整性判定 | 正确：`session_shadow.rs:114` 只要求 `missing_from_directory == 0`，因此身份库是**超集**是允许的（这正是解除 50 条上限所需） |
| 崩溃恢复 | 有真实子进程 `os.Exit` 测试（`crash_recovery_test.go`），验证已提交事务保留、未提交回滚 |
| 清单安全性 | `inventory_safety_test.go` 覆盖符号链接、往返校验、重复 catalog ID、跨目录同名文件四种情形 |
| 平台分隔符映射 | Go 的统一路径比较与 Windows 反斜杠比较语义一致，未发现混用 |

---

## 5. 建议的修复顺序

1. **1.1 中断删除的续做入口**（功能缺口，与设计稿明确不符）。
2. **1.2 标题不得使分页失效**（会直接影响日常使用；同时需要更新那条把该行为固化的测试）。
3. **1.3 legacy 分支保留分页 + 显式标注**（否则本项目标无法达成）。
4. **1.4 / 2.1 / 2.2 的性能收敛**（三者都在"每次操作/每页"的路径上，建议一起做）。
5. 2.3、2.4、2.7 的语义与归一化一致性；其余低严重度项可随后清理。

## 6. 本次审查未覆盖的部分

- 未运行或新增任何测试；结论均来自静态阅读。
- 未逐行读：`session_title_backfill.go`、`session_previews.go` 的全部改动、
  `workbench_catalog.rs` / `workbench_projects.rs` 的写入细节、
  `TauriChatWorkspace.tsx` 的分页状态机（只读了分页消费与审计重载两段）、
  `docs/tauri/*` 的文档改动。
- 未在真实 profile 上做端到端验证（迁移、备份、恢复演练均未执行）。
