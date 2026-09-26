# Codex 未提交改造审查报告（S2：持久会话目录与项目树）

> 审查对象：`experiment/tauri` 工作区中**未提交**的改动（约 3,400 行 / 39 个已跟踪文件 + 2 个未跟踪文件），
> 提交基线 `c8a2a8deb`。
>
> 审查方式：**只读代码审查，未修改任何文件**。所有结论均给出文件与行号，可逐条复核。
>
> **时点说明**：上面的文件规模与下方 §0、§1–§6 是初始审查快照，不代表当前工作树状态。
> 后续复核与修复记录见 §7 以后；截至 2026-09-26，标题漂移不再单独触发回退，安全可核验的
> `missing` 行及未认领 transcript 差异可保留身份目录分页；缓存续页和启动/会话变更刷新会复用
> 首屏 shadow 报告。可读 shadow 中的结构/物理差异现在显示全量身份分页与独立旧目录项，但标记为未核验只读；shadow 不可读时也会尝试通过身份分页只读展示，首屏身份页不可用时才回退到最多 50 条兼容目录，续页失败则不拼接来源。
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

## 7. 后续状态复核（2026-09-25，工作区基线 HEAD `c95688000`）

本节更新 §1 的历史审查结论。原报告仍记录基线 `c8a2a8deb` 的代码状态；以下结论来自当前源码复核，**不是新增测试或真实 profile 认证**。

| 原项 | 当前状态 | 当前证据与边界 |
| --- | --- | --- |
| **1.1 中断删除不可发现** | **可发现并可显式重试；当前 host 已支持有界分页** | bridge 在打开 listener 前持 profile gate 对已有 identity DB 执行可迁移 `Open`；`/v1/sessions/deletion-recovery/page` 使用 sidecar 生命周期内 integrity-checked 的只读 Store，按不可变 session ID 游标每页最多 200 条 `deleting` 身份。schema 6 为该续页查询维护仅含 `deleting` 行的 ID 部分索引；旧 host 兼容路由与 schema 5 只读检查仍可工作，但旧发布二进制兼容性仍未认证。重试完成只会移除对应 ID；按 ID 排序使 workbench 顺序变化和旧数据库 position 值都不会移动续页边界。Tauri 侧栏加载首批待删除项并可按需继续加载，失败显示错误和重查入口；重查会从第一页重新开始。前端用请求序号丢弃过期的重查/续页响应，并在请求期间禁止重复续页，避免旧页面覆盖较新的检查结果。恢复行由独立只读端点提供，显式重试即使普通身份目录当前为 `identity_unverified` 也不会被通用列表门禁挡住；若当前另一个会话正在生成/暂停，该项 DELETE 无需切换 controller，仍可直接重试。原 `/v1/sessions/deletion-recovery` 无参数路由保留给旧 host，仍有 10,000 项上限。能力 `session_delete_recovery_list_v1` 纳入 host 握手。普通会话列表按待删 ID 过滤，避免 JSON fallback 中的旧 catalog 行与恢复区重复展示。显式 DELETE 在无 live runtime 时校验 profile 内预期路径，再用读写 Store 重走 `BeginDelete → RemoveSessionArtifacts → FinishDelete`；只有 artifact sweep 成功后才提交 `deleted`，失败保留 `deleting` 供下次显式重试。清理将不存在的文件视为已清理，可重复执行。另：shadow 审计把 legacy catalog 中与身份库 `deleting` 状态相符的 ID，以及 transcript 已确认不存在的 `deleted` ID，作为已解释的生命周期差异，不让一个待完成删除迫使其余会话降级到 50 条 fallback；审计仍单独报告该计数，未知缺项仍阻止 clean。证据：`cmd/reasonix-desktop-bridge/main.go` 的启动顺序、`session_delete_recovery_list.go`、`session_delete_recovery.go`、`internal/sessionidentity/delete.go`、`internal/control/controller.go`、`desktop/frontend/src/tauri/TauriChatWorkspace.tsx`。 |
| **1.2 标题变化打断分页** | **实现已修复；一条 host 测试断言仍陈旧，未执行测试** | 当前 `visibleSnapshotID` 仅覆盖 `id`、`relative_path`、`workspace_root`、`position`、`state`；`session_page_matches_shadow_snapshot` 也只比较结构字段，所以标题回填/改名本身不使游标失效。源码中的 `title_change_between_shadow_and_page_cannot_pass_the_structural_snapshot_id` 仍断言首屏应 legacy fallback、续页应报错，与当前结构快照合同相反；本轮按验证限制没有运行或改动测试。证据：`internal/sessionidentity/store.go` 的 `visibleSnapshotID`、`desktop/tauri/src/main.rs` 的分页匹配函数及该测试。 |
| **1.3 影子差异时 legacy 上限与可见性** | **dirty shadow 或 shadow 不可读时优先身份只读分页；首屏身份页也不可用时仍最多 50 条** | clean/partial identity 继续可操作分页。workspace/order、物理状态或 inventory 有差异但 shadow 可读且 identity 页逐页匹配同一 snapshot 时，新增 `identity_unverified` 来源，分页展示全部已登记身份行；旧目录独有项放在独立只读区，标题/项目元数据冲突摘要可见；打开、删除、新建均禁用，不将冲突元数据当作已核验。shadow 不可读但 identity 页可用时，也以 `identity_unverified` 分页显示身份目录，不显示无法核验的旧目录独有项，不允许会话操作或项目切换；续页必须携带且匹配同一 snapshot ID 和 total。sidecar 离线时已核验缓存仍以 `cached` 只读标签展示。只有首屏身份页不可用才使用最多 50 条 legacy JSON；identity continuation 失败则报错并要求重新读取首屏，不把两种来源拼接。UI 区分 shadow 不可用与已知差异，并保留重查入口。 |
| **1.4 每页全量 shadow 扫描** | **clean 与未核验 identity 续页都复用首屏物理盘点；新 schema 使用 DB generation/revision** | 首屏通过完整 shadow 审计后，host 缓存已审计的物理 inventory 与完整身份目录。clean/partial 页逐页验证 ID、workspace 和生命周期；结构/物理差异的 `identity_unverified` 页则逐页与缓存 shadow snapshot 对照，均不在每页重复全量 inventory。schema 6 用 SQLite trigger 对身份结构变更递增 revision；首次分页读取同事务里的 generation/revision 并统计 total，游标携带 total，续页在同一读事务中校验 revision 后只读当前页。随机 generation 与 revision、workspace filter 一起参与 snapshot ID，避免数据库替换后 revision 数值碰巧相同而复用旧游标；完整 shadow snapshot 和分页接口共用该 ID。schema 5、revision 表/行或任一 trigger 不可用时仍使用覆盖索引上的完整结构哈希和计数。分页 handler 复用 integrity-checked 的只读 Store；每次缓存命中仍复查 profile containment、SQLite sidecar 类型和主数据库文件 identity。未做性能基准；外部文件系统变化需用户显式重新检查。旧 Wails writer 停写及旧 Tauri host 发布二进制兼容仍未认证。 |
| **2.3 `Accepted` / `Synced` 计数语义** | **删除生命周期导致的 `Synced` 假失败已修正；匹配的旧 tombstone 不再阻断 legacy import** | `ImportLegacyCatalog` 保留已存在身份 metadata、保留缺失 transcript 行，因此 `Accepted = len(candidates)` 是被接纳的输入数，而非新增行数；对 ID/path 匹配的 deleting/deleted 行，legacy migration 现在保留 terminal identity 并继续处理其余输入，路径冲突仍失败。`SyncWorkbenchOrder` 把匹配的旧 host tombstone 计为已识别并返回给 host 完整性门禁，但仍将其从可见顺序及元数据更新中排除；未知 ID 仍不计入并会被 host 拒绝。证据：`internal/sessionidentity/store.go` 的 `preserveExisting` import 分支、`session_catalog_import.go`、`desktop/tauri/src/bridge.rs` 的 `import_legacy_session_catalog`、`internal/sessionidentity/catalog.go` 的 `SyncWorkbenchOrder`、`desktop/tauri/src/main.rs` 的 `require_complete_workbench_sync`。 |
| **3.1 JSON 解析失败无协议响应** | **已修复** | 当前 catalog import/sync handler 对 `decodeJSONBody` 错误统一写 `400 invalid_request` 协议错误体，不再直接返回空响应。证据：`cmd/reasonix-desktop-bridge/session_catalog_import.go`、`cmd/reasonix-desktop-bridge/session_catalog_sync.go`。 |
| **3.3 唯一路径检查重复解析** | **同一既有路径的重复 canonicalization 已减少** | `relativeTranscriptPathWithIdentity` / `resolveTranscriptPathWithIdentity` 同时返回 lexical path 与已验证的 resolved path；`ensureTranscriptPathAvailable` 复用 resolved 值作别名比较，避免再对该行执行一次 `resolveIdentityPath`。父目录逃逸检查、最终路径检查及 file identity 冲突检查仍独立保留。证据：`internal/sessionidentity/store.go` 的上述 helper 与 `ensureTranscriptPathAvailable`。 |

**后续边界**：高优先级 1.1、1.3、1.4 与标题游标问题已在 §7 以后逐项复核；identity snapshot 超限、legacy 导入/同步计数也有当前处理记录。§2.1 的物理别名检测仍需核对所有生命周期身份，缩短剩余 O(n) 需要有失效规则的文件身份索引；§2.5 的重复读取仍是快照一致性保护，只有确认所有 writer 已停写后才能重新评估。新 schema 的 DB revision 仅用于 SQLite 结构 cursor；缺少 revision 元数据时仍有哈希回退，物理 inventory 继续由首屏/手动重新检查刷新。旧版 Wails writer 停写确认、旧 Tauri host 发布二进制兼容认证与真实 profile 的停写后恢复演练仍是独立未认证门禁。

## 8. §2/§3 其余条目复核（2026-09-25）

以下是当前源码边界核对，不代表跨旧版本 writer 的认证，也未运行测试。

| 原项 | 当前结论 | 证据/边界 |
| --- | --- | --- |
| **2.1 写入唯一性检查 O(n)** | **仍存在，已去掉每个 peer 重复规范化/解析同一 profile root 的成本** | `ensureTranscriptPathAvailable` 仍逐条解析现有身份，以发现 symlink/hard-link 别名；扫描当前不按 lifecycle state 过滤，missing/deleting/deleted 行仍保留其 transcript 路径身份，不能假定可被另一 ID 重用。当前调用方把经验证的候选 resolved path 传入；peer 扫描现在每次唯一性检查只规范化 root 一次，并在有 peer 时只 `EvalSymlinks(root)` 一次，再对每个 peer 独立验证其最近存在父目录、最终路径 containment 和 file identity。并非每个写操作都扫：`Reserve` 对已存在且路径不变的 reserved 行直接返回，`MarkReady` 对 ready 行幂等返回；首次 ready、身份路径唯一性复核及删除 fence/finalize 仍需扫描。仅用 lexical UNIQUE 索引不能检测物理别名；剩余 O(n) peer 检查需要可验证的文件身份索引及失效规则才能安全减少。 |
| **2.2 WAL 打开时整库复制** | **常见路径已优化，保守 fallback 保留；ready 标记重试不再重复开库** | `validateExistingIdentityDatabase` 只在 WAL 布局完整、帧大小匹配且没有第 1 页帧时跳过拷贝校验；出现 schema 页帧或任何不确定布局仍将 DB/WAL/journal 复制到临时目录读取 `user_version`。生产 sidecar 的只读列表/清单查询通过 `readIdentityStore` 复用 integrity-checked Store，每次使用前仍调用 `ValidateReadOnlyPath` 复核位置与文件 identity，sidecar 退出时关闭；写入路由仍按操作新开 Store。`bridgeLifecycleSink` 首次成功打开 Store 后复用它重试 `MarkReady`，避免 transcript 落盘前的每个事件重复执行 Open/WAL/schema 预检；runtime 关闭时释放 Store。异常与不确定 WAL 路径仍保留复制校验，不能从静态检查推断可删除。 |
| **2.3 `Accepted` / `Synced` 计数与实际处理不符** | **当前计数语义一致；匹配的 terminal legacy 行被无副作用接纳** | `ImportLegacyCatalog` 使用 `preserveMissing=true`，缺失 transcript 会登记为 `missing`，已存在身份会逐项校验后保留，因此成功响应中的 `accepted=len(candidates)` 表示请求条目全部处理而非新增行数。对 ID/path 匹配的 `deleting/deleted` 行，legacy migration 保留 terminal identity 并继续处理其余 catalog 项；路径冲突仍报错。顺序同步只对身份 ID 与 transcript path 均匹配的 host 条目计入 `synced`；直接同步遇到匹配 tombstone 也计数，但不会把 tombstone 放回可见顺序。缺少 identity 的条目不计数，host 要求 `synced == sessions.len()`，否则报告同步不完整。 |
| **2.4 工作区路径过滤大小写不一致** | **当前列表路径未触发；未使用的 Tauri 过滤入口已移除** | Preview 请求全局分页后在前端分组，没有以 workspace root 调用分页过滤；本轮移除了未使用的 `bridge_session_directory_page` Tauri command，避免暴露有大小写语义差异的旁路。bridge 内部按 workspace 查询仍是精确匹配；若未来重新暴露项目过滤，需先统一 Windows 根路径大小写/分隔符语义。 |
| **2.5 离线快照重复读取** | **仍存在，数据一致性校验仍在** | `copyVerifiedSnapshotFile` 对源数据流式哈希后还会重新哈希源文件；快照收尾再验证目标副本和 manifest。创建快照的合同要求先停写所有 profile writer，而旧 Wails writer 停写仍未认证，因此不据此删掉验证步骤。 |
| **2.6 ApplyImportReview 锁只在注释** | **新 profilegate 门禁已由函数获取；旧 writer 外部门禁仍未认证** | `ApplyImportReview` 当前在重新审查与应用前调用 `profilegate.TryAcquire(s.profileRoot)` 并 `defer` 释放。该 gate 不能约束不遵守新锁协议的旧 writer。 |
| **2.7 同步/导入 50 条限制** | **仍是有意的 bounded recent-session 合同；不是身份分页上限** | Tauri JSON catalog、legacy import 与 workbench-order sync 仍限制 50 条；身份目录分页上限为 10,000。超过 identity snapshot 上限时首屏直接报错，不签发不可续页游标；Rust 同时拒绝超限响应，host 随后可走最多 50 条的 legacy fallback 并明确标注上限。shadow clean 时项目树可以继续从身份目录分页，见 §7 的 1.3。 |
| **2.8 删除恢复使用读写 Open** | **写恢复仍需可迁移 Open；只读恢复清单复用只读连接** | DELETE 恢复必须提交 `deleting/deleted` 状态，所以使用可迁移 schema 的 `Open`；只读 GET recovery 清单走 sidecar 生命周期内的只读 Store。旧 Tauri host 二进制能否通过新 capability 握手仍未认证。 |
| **3.2 未调用的 bridge 分页命令** | **生产 Tauri invoke 入口已移除** | 当前实现与前端检索均未发现 `bridge_session_directory_page` 定义、注册或调用；WebView 仅通过 `workbench_session_page` 获取分页，该入口执行 clean shadow 门禁。`BridgeSupervisor::session_directory_page` 是 host 内部方法，不是 Tauri invoke；workspace snapshot helper 限定为 `cfg(test)`。bridge 查询类型仍保留 workspace filter 参数，但当前生产 host 不使用 workspace-filter 分页。 |
| **3.4 大小写路径冲突缺少提示** | **已补充错误说明，保守策略仍在** | 仅当两条 canonical path 不同、但 `sameCaseInsensitivePath` 判定为大小写别名时，冲突错误会说明 macOS/Windows 的保守匹配以及 macOS 卷大小写敏感性可能造成误报；`errors.Is(..., ErrTranscriptPathConflict)` 仍成立。策略本身不放宽，以免在常见不区分大小写的卷上允许两条 identity 抢占同一 transcript。 |
| **3.5 快照包含 cache** | **已修复：排除明确可再生成的 profile 根 `cache/`** | `config.CacheDir` 的默认 `REASONIX_HOME/cache` 保存 usage/history/task/session 投影、环境探测快照和插件/模型缓存；会话 JSONL、身份 DB 与独立 host catalog 不在此目录。`CreateOfflineSnapshot` 跳过仅有的顶层 `cache/`，其他 profile 文件仍逐项复制并校验。若未来把非缓存权威数据放进该目录，需先修改其存储约定。 |
| **3.6 `previewRoot` 命名过时** | **已清理** | `Import`、`ImportLegacyCatalog`、`importCandidates` 及 `bindImportProfileRoot` 的参数名现为 `sessionDir`，与调用方传入的 transcript 目录一致；profile root 仍从 Store 独立读取/绑定。 |
| **3.7 空 catalog `Accepted` 为 0** | **未发现歧义** | 空输入接受 0 项，响应仍明确包含 `accepted: 0`；Rust 校验 protocol version 且要求 accepted 不大于输入长度。 |

## 9. 项目树与选择语义复核（2026-09-25）

- 当前分组先按保存的项目文件夹建组，再将身份目录页中的会话归入对应路径；没有会话的保存项目仍显示为空组。路径 key 保留 `/` 文件系统根，不裁掉路径内容首尾的空格；Windows key 统一分隔符、区分盘符根 `C:\` 与盘符相对路径 `C:` 并折叠大小写，其他平台保留大小写以兼容大小写敏感卷。稳定版目录导入、保存目录、身份导入与影子比较、工作区可用性探测、MCP 项目范围及新建会话参数都保留原始路径字符串；只在判断空值时将纯空白视为空。
- Tauri 项目文件夹 catalog 只接受绝对路径，持久化时去掉尾部分隔符（Windows 盘符根保留 `\\`），通过相同 project key 合并重复根；非分隔符空格保留。稳定版项目清单导入、Go sidecar 项目读取及 Rust bridge 响应校验均拒绝相对路径；稳定版导入、Go sidecar 与 Rust bridge 解码去重也复用平台 project key，避免尾分隔符/Windows 大小写别名成为重复组。Go 与 bridge 都保留 sidecar 首个根及其顺序，本地保存标题仍覆盖对应根标题。标题改名通过 key 找回原根并原子替换 JSON catalog；损坏 catalog 会保持原文件且禁止写入。保存目录与稳定版目录合并时，稳定版目录顺序优先。
- 点击有可恢复会话的项目组会打开组内最新的非缺失会话；点击空项目组只设置新会话默认工作区，不切换当前对话。工作区按钮显式标为“新对话默认工作区”，MCP 项目范围跟随所选默认工作区，输入框底部标为“当前对话工作区”。
- 项目根可用性通过分批只读探测刷新；不可用状态会标注并阻止在该根下新建，但不阻止打开已有会话。导入稳定版项目文件夹后，前端立即重读合并后的项目列表，根集合变化会触发新一轮可用性探测。2026-09-26 逐行补看 `workbench_catalog.rs`、`workbench_projects.rs` 与完整前端分页状态机，并修正了旧项目目录测试中与“保留路径空格”要求相冲突的 fixture；本轮未运行测试或访问真实 profile。

## 10. 前端分页状态机复核（2026-09-26）

- 当前页状态由 `sessionPageRevisionRef` 防止旧续页响应覆盖新首屏；续页追加前按 ID 去重，目录变更/请求失败后重新读取受 shadow 保护的首屏，重启仍失败则清空页并显示错误。标题补全失败只影响标题，不丢弃已读页面。完整审计刷新已加载页数时逐页重新取身份目录，若页间数据源退回 legacy/cached，会采用新来源并提示目录发生变化；不把兼容 catalog 误标为 identity。
- legacy 页面明确显示最多 50 条、审计数量不等于可分页数量，并提供重试导入与重新检查入口；title-only drift 和仅未认领文件差异已可从身份目录继续分页，其他可读冲突通过 `identity_unverified` 展示全量只读身份页和隔离的 legacy-only 项。shadow 不可读时，身份页可用也会以 `identity_unverified` 只读分页；首屏身份页不可用才回退到 50 条 legacy，续页失败则重新读首屏、不拼接来源。cached 页面说明可能过期且不能操作。待完成删除列表在读取期间显示 loading，失败可重查，重试状态会清理旧错误。
- cached 快照的分页结构比较忽略标题、校验 ID 顺序和 workspace；离线 catalog 仅标题变化时继续使用上次 shadow 快照中的旧标题，仍显示 cached/可能过期标签并禁用会话操作。若缓存来自 dirty shadow，UI 同时保留“目录未核验”差异和隔离出的旧目录独有行，不把该缓存标为 clean。
- 重命名先成功更新 bridge，再更新本地 workbench catalog；若 catalog 同步失败，UI 现在说明标题已保存、列表同步失败，并提示重新检查，避免显示“无法保存到最近对话”的错误上下文。
- Tauri 运行时已增加独立的 scan-only 候选接口和审核弹窗；仅 `managedProfile` 的隔离 Preview profile 可调用，需逐项确认标题/项目及 transcript 指纹，`apply` 在 sidecar 持有 profile gate 时复核并事务登记。它不会扫描稳定版 profile，也不把旧 Wails writer 的兼容性视作已认证。额外的 `reasonix-session-import` 命令仍用于停写确认后的来源快照/恢复副本演练；该 CLI 不会合并或切换运行中的 Preview，详见 `OFFLINE_SCAN_SESSION_IMPORT.md`。身份页只有完整 shadow 可验证时可操作；可读但有结构/物理差异时现在走 `identity_unverified` 全量只读分页，并隔离显示仅在旧目录中的行。shadow 不可读时若 identity page 可用，也走未核验只读分页；首屏身份页不可用才用最多 50 条兼容回退。
- 标题改名先写 bridge identity，再写 host workbench catalog；若跨存储同步失败，当前对话已更新并提示“标题已保存、列表同步失败”，让用户重新检查目录。此时 shadow 门禁仍报告差异；若差异仅为标题，分页来源会标记为 `partial_identity` 并采用身份目录标题。
- 这轮逐段复核 `TauriChatWorkspace.tsx` 的初始化、续页、审计重载、离线快照和删除恢复状态流，未发现新的可由静态证据确认的状态机缺陷。该结论是源码复核，不是并发运行认证；未运行测试或访问真实 profile。旧 writer 停写及旧 Tauri host 二进制兼容继续未认证。

## 11. Rust 工具链声明与锁文件复核（2026-09-26）

- 当前锁定的 macOS 与 Windows `normal,build` 依赖图中，最高显式 `rust-version` 为 `notify-rust 4.18.0` 的 1.89.0；`time 0.3.55`、`plist 1.10.1` 等包声明 1.88.0。原 manifest 的 1.77.2 与这些依赖声明冲突，因此已更新 `desktop/tauri/Cargo.toml` 到 1.89.0。
- 这是基于依赖 manifest 的最低版本声明，不等同于用 Rust 1.89.0 编译通过。本环境只有 Rust 1.95.0；未运行测试。若要支持低于 1.89.0 的 Rust，需要先将依赖图降级并在目标工具链上验证。

## 12. 未认领 transcript 与已验证目录分页（2026-09-26）

- 当前 shadow 比对将未认领 transcript 数与 catalog/身份元数据、物理状态和 inventory 错误分开。若 ID、workspace、顺序与物理 inventory 一致，title-only drift、被 inventory 确认的 missing identity rows 与未认领文件差异都允许 Tauri 返回 `partial_identity` 并继续分页已登记 identity rows；missing rows 标注为不可打开，未认领文件只有经过显式审核导入才会登记。若存在其他可读 shadow 差异，使用同轮核验的 `identity_unverified` 全量只读分页；shadow 不可用时也会尝试通过身份页只读分页，首屏身份页不可用才退回最多 50 条 legacy fallback。managed Preview 的审核弹窗把选择交给 bridge gate 内复核，稳定版 profile 不在该 Tauri 命令的 profile root 下；离线 CLI 快照/暂存仍提供独立恢复副本流程。
- 续页缓存记录未认领计数；bridge 离线时显示为可能过期的 cached 快照并禁用会话操作。UI 在 partial source 下说明已显示范围与未导入数量。该切片减少“只有孤儿文件”导致的既有身份目录 50 条退化，不替代剩余旧历史的逐条审核导入。

## 13. 清理旧 Preview 路径术语（2026-09-26）

- `importCandidates` 与路径校验 helper 已改用 session directory/root 错误描述，不再把 session transcript 目录称作 Preview root；当前实现中的参数名此前已统一为 `sessionDir`。
- `ensureTranscriptPathAvailable` 仍复用已验证的 resolved candidate path；现有 peer 的 O(n) symlink/hard-link 检查仍保留，未宣称 2.1 已解决。
- 非测试验证：`gofmt`、`GOCACHE=/private/tmp/reasonix-tauri-go-build-cache go build ./...`、`git diff --check`。未运行测试或访问真实 profile。

## 14. 项目分组路径 key 复核（2026-09-26）

- Go 旧目录读取、Rust 项目文件夹 catalog 与前端会话分组对 Windows `/`/`\\`、大小写、盘符根尾分隔符，以及 Unix `/` 根目录采用一致的 key 规则。三处都仅用 trim 判断空路径，不剥掉合法路径首尾空格；前端将空白 workspace 归到 rootless 会话组。
- 当前列表全局分页后在前端按 project key 分组；尚无生产 workspace-filter 分页入口，因此 Go 身份库对 workspace root 的精确过滤未被当前 UI 路径触发。
- 这是源码对照，不是 Windows/Unix 跨平台运行认证。旧 Wails writer 与旧 Tauri host 兼容门禁仍未认证。

## 15. 唯一路径扫描复用 profile root 解析（2026-09-26）

- `ensureTranscriptPathAvailable` 在扫描前规范化 profile root 一次；有 peer 时只解析 profile root 的 symlink 一次。逐条 peer 仍独立做路径/父目录安全检查、最终解析和 hard-link 比较，因此这是每次操作去除重复 root 工作，不会把 O(n) 或物理唯一性检查描述为已解决。
- 当 identity store 尚无 peer 时，检查不额外解析 profile root；候选本身仍由同一事务调用方 fresh 校验。
- 非测试验证：`gofmt`、`GOCACHE=/private/tmp/reasonix-tauri-go-build-cache go build ./...`、`git diff --check`。未运行测试或访问真实 profile。

## 16. 离线快照排除可再生成缓存（2026-09-26）

- `CreateOfflineSnapshot` 现在跳过 profile 根下的 `cache/` 子树。当前 Tauri profile 的 Go `CacheDir` 默认将环境探测快照、usage/history/task/session 投影以及插件/模型缓存放在该目录；权威 transcript、session identity DB 与独立 workbench catalog 仍纳入快照。自定义 `REASONIX_CACHE_HOME` 若指向其他路径，不会被此规则排除。
- 路径只在确认为普通目录后跳过；symlink 和其他非普通文件的拒绝规则仍先执行。manifest 仅列出实际复制的目录和文件，staging 按该 manifest 创建，因此会自然省去 cache。
- 非测试验证：`gofmt`、`GOCACHE=/private/tmp/reasonix-tauri-go-build-cache go build ./...`、`git diff --check`。未运行测试或访问真实 profile；旧 writer 停写门禁继续未认证。

## 17. cached 续页不被标题漂移打断（2026-09-26）

- 离线 cached 页面仍要求兼容目录 ID、顺序与 workspace 结构匹配，并用快照 ID 验证 cursor；只放宽 cached 页的 title 比较，继续使用最后一次核验的旧标题。cached 标签提示数据可能过期，且该来源禁用会话操作。
- 这样避免仅 JSON 标题变化造成缓存页拒绝/无法加载下一页；实时路径仍要求身份结构、workspace 与生命周期匹配；标题是可变展示数据，独立差异会标记并使用身份目录标题。
- 非测试验证：Tauri `cargo fmt --check && cargo check --locked`、前端 `npx tsc --noEmit -p tsconfig.json`、`git diff --check`。未运行测试或访问真实 profile。

## 18. 项目文件夹合并避免重复线性查找（2026-09-26）

- `merged_workbench_project_folders` 原先对每个本地项目目录重新线性扫描 sidecar 已返回的目录，最坏情况下目录合并随两侧数量相乘增长。现在先按平台规范化 project key 建索引，再合并本地目录；sidecar 顺序不变，本地匹配根的标题仍覆盖 sidecar 标题，新根仍按本地目录顺序追加。
- 该优化只改变合并查找复杂度，不改变路径 key、重复根选择或项目组显示语义。源码检查和 `cargo check` 不能替代大目录性能基准；本轮未运行测试或性能测试。

## 19. 标题差异不再单独触发 50 条 legacy 回退（2026-09-26）

- 原有安全分页门禁把标题与 workspace、顺序、物理状态同等处理。当前持久身份目录已能在全量 shadow 中确认 ID、workspace、顺序及磁盘状态；标题仅为可变展示元数据，单独不一致不会改变 ID 到 transcript 的映射。此时允许分页身份目录页，并使用 sidecar 返回的持久标题；UI 标注标题差异数量。未认领 transcript 仍只显示数量，不会自动导入。
- workspace、顺序、身份缺项、transcript/物理状态或 inventory 错误会阻止可操作的身份分页；若 shadow 与 identity page 可逐页匹配，仍以 `identity_unverified` 显示全量身份行和旧目录独有项，但禁用会话操作。shadow 不可读时，identity page 也可在仅匹配 cursor snapshot ID/total 的条件下只读分页；首屏 identity page 不可用才回到最多 50 条兼容目录，续页失败则重新读取首屏、不拼接来源。
- 非测试验证：Tauri `cargo fmt --check && cargo check --locked`、前端 `npx tsc --noEmit -p tsconfig.json`、`git diff --check`。未运行测试或访问真实 profile；旧 Wails writer 停写及旧 Tauri host 发布二进制兼容仍未认证。

## 20. 项目目录 catalog 加载去除二次方去重（2026-09-26）

- `workbench_projects::read_folders` 接受最多 10,000 个目录。之前每项都线性扫描已接受项以查重复根，构成 O(n²) 的本地 JSON catalog 解析后处理。现在用 `HashSet` 保存规范化 project key，去重处理为预期 O(n)，仍保留先出现的根/标题、并在去重前验证每一项输入的路径和标题。
- 单次 `remember`/`set_title` 仍需 O(n) 查找，目录数量上限 10,000；本轮没有将其描述为常数时间或做性能基准。

## 21. 已确认缺失的会话保留在持久目录分页中（2026-09-26）

- `missing_transcripts` 原先不区分“数据库仍标 ready、物理文件却不存在”的矛盾，与“identity 已标 missing 且 inventory 可读并确认文件不存在”的预期状态；两者都阻止整个身份目录分页。后者已有明确缺失生命周期，host 现在允许其随完整身份目录续页；前端已有缺失标记、禁止打开及显式删除入口。
- 物理 inventory 错误、`ready` 文件缺失、`missing` 文件却存在等 `physical_state_mismatches` 仍 fail closed，不会因这次调整进入 identity source。报告和分页 response 分别保留缺失数；跨进程旧 writer 仍不认证。
- 非测试验证：Tauri `cargo fmt --check && cargo check --locked`、前端 `npx tsc --noEmit -p tsconfig.json`、`git diff --check`。未运行测试或访问真实 profile。

## 22. 初始化复用首屏 shadow 报告

- 启动流程原先先调用 `workbench_session_page` 完成全量 shadow/inventory，再调用 `bridge_session_catalog_shadow` 为诊断面板重新盘点一次。现在页面响应携带同轮 path-free `SessionShadowReport`，并复用该报告填充初始诊断面板；只有页面 shadow 不可用、没有报告时才走独立重试。若首屏存在更多页，同一报告也随受安全分页检查的缓存保存，续页不增加 inventory 请求。
- 该优化消除正常初始化的重复物理盘点；用户手动刷新诊断仍执行新一轮全量审计。非测试验证：Tauri `cargo fmt --check && cargo check --locked`、前端 `npx tsc --noEmit -p tsconfig.json`、`git diff --check`。未运行测试或访问真实 profile。

## 23. 会话变更后列表与诊断只做一次完整审计

- 新建、删除等刷新路径先独立调用 shadow endpoint，再请求 `workbench_session_page`；两者都会执行全量 inventory。`refreshVisiblePage` 现在直接读取受门禁保护的首屏，使用其同轮 `shadowReport` 更新诊断，并只对已加载范围按缓存 cursor 续页。单次显式诊断刷新仍使用独立 fresh shadow 请求。
- 若续页数据源变化，首屏重新读取并同步列表/报告，避免保留另一轮审计的诊断计数。非测试验证：前端 `npx tsc --noEmit -p tsconfig.json`、`git diff --check`。未运行测试或访问真实 profile。

## 24. legacy 回退提示直接说明差异原因

- 原提示说明最多显示 50 条、身份目录计数只是审计值，并指向“运行状态”；具体差异需要用户再打开诊断面板查找。现在侧栏直接汇总身份缺项/新增、标题、工作区、顺序、transcript 缺失、物理状态、未认领文件、inventory 错误及旧目录退休行；shadow 报告不可用时明确说明无法确定原因。若报告为 clean 但同轮分页门禁失败，则提示重新检查，避免把并发/快照不一致说成已知目录差异。
- 重新检查的动作仍是重试有界旧目录导入并重新核验；未认领 transcript 仍需审核，不会自动认领。该提示提升可发现性，没有放宽 dirty shadow 的分页安全门禁或 50 条 fallback 上限。
- 非测试验证：前端 `npx tsc --noEmit -p tsconfig.json`、`git diff --check`。未运行测试或访问真实 profile。

## 25. 启动或重试导入失败不再被成功的身份分页吞掉

- 启动和“重新检查会话目录”都会先尝试有界旧 catalog 导入，再读取会话分页。原实现吞掉了启动导入失败；手动重试也只在结果回到 legacy/cached 时显示错误，若身份页成功则错误消失。
- 现在把分页结果和导入结果分开表达：启动时列表加载成功会显示可用状态及导入错误，若分页也失败则全局错误同时包含两项原因；手动重试无论列表读取成功或失败，也都会同时保留导入结果。独立 shadow 审计不会清除非阻断提示，首屏刷新或会话变更会清除旧提示。未改变列表选择，也未将导入失败误写成分页失败。
- 非测试验证：前端 `npx tsc --noEmit -p tsconfig.json`、`git diff --check`。未运行测试或访问真实 profile。

## 26. 工作区/顺序差异继续 fail closed 的依据

- 放宽 workspace/order 差异确实可以让更多 dirty 情况继续显示身份目录页，但这会直接选择 SQLite 或 JSON 中一边的项目归属和最近顺序。当前 `workbench-sessions.json` 仍是回退输入并由 host 写入；旧 host 是否会把这些更新同步到身份库，尚无发布二进制端到端认证。不能只凭新源码的写入路径推断所有旧 host 都遵守新同步合同。
- 因此 `workspace_mismatches`、`order_mismatches` 继续阻止可操作身份页；若 shadow 与分页快照逐页一致，现在可用 `identity_unverified` 只读展示全部身份行，且把仅在旧 catalog 中的行放进独立只读区。该展示不选择任何一侧作为已确认项目/顺序，不允许打开、删除或新建。shadow 不可读时只要身份页仍可读，就按 cursor snapshot ID/total 只读分页；首屏身份页不可用才回退最多 50 条，续页失败不拼接来源。旧 host 的发布二进制兼容仍未认证。
- 这是基于当前写入/审计合同的源码改动，不是旧 writer 兼容认证。非测试验证：Tauri `cargo fmt --check && cargo check --locked`、前端 `npx tsc --noEmit -p tsconfig.json`、`git diff --check`；未运行测试或访问真实 profile。旧 Wails writer 停写及旧 Tauri host 兼容仍未认证。

## 27. 未认领 transcript 的审核导入

- 现有 `PrepareImportReview` 仍只接受旧 catalog 中的 ID，不能直接审核 `InventoryFromScan` / `ClaimUnclaimed` 文件。新增独立 `reasonix-session-import` 离线流程解决了候选来源与逐条元数据确认：候选默认不选中，操作者必须核对 ID、文件名及 transcript SHA-256，并显式填写标题和 workspace（空字符串代表确认不设置）。非法命名或与 catalog/identity 冲突的文件不会进入候选。
- 流程先要求 `--writers-stopped` 明确确认，再创建独立快照和恢复副本；`review` 生成有界计划及批准 SHA-256，`apply` 要求该摘要匹配，在副本 profile 锁内重新核对 catalog、inventory、元数据、顺序、路径与 transcript 指纹，并以事务登记整批身份行。审核 JSON 读取时会核对文件类型、打开前后的文件身份、大小，以及 Unix 类系统上的私有权限。它只写恢复副本的 SQLite，不改来源 profile、transcript 或正在运行的 Preview；不会自动合并或切换 Preview。操作步骤见 [`OFFLINE_SCAN_SESSION_IMPORT.md`](OFFLINE_SCAN_SESSION_IMPORT.md)。
- managed Tauri Preview 的诊断面板已有逐项审核入口；它只使用独立的 Preview profile，候选扫描不返回 transcript 内容，应用时由 bridge gate 内重核后一次性登记。旧版 `PrepareImportReview` 仍只处理 catalog ID，scan-only 候选由新的 `ScanImportReview` API 处理。另有 `reasonix-session-import` 停写后快照/恢复副本 CLI，操作只作用于副本。内置 Preview 入口不证明可对稳定版 profile 并发操作；对稳定版做离线快照仍要求操作者确认旧 Wails writer 停写，旧发布 Tauri host 兼容也仍未认证。非测试验证覆盖 Go build、Rust check、前端 typecheck 和 diff check；未运行测试或访问真实 profile。

## 28. 项目文件夹清单来源失败现在可见

- 原 `merged_workbench_project_folders` 把 sidecar 读取失败折叠为空数组，前端启动时也静默忽略命令错误；若旧版清单不可用，用户可能只看到本地清单和当前已加载会话推导出的分组，不知道没有会话的已保存文件夹未被读取。若本地 catalog 损坏，反过来也没有说明只返回了旧版来源。
- Tauri command 现在返回合并后的 `folders` 和可选 `warning`。两侧均可读时行为不变；单侧不可用时保留另一侧清单并指出来源，双侧不可用时明确说明已保存的空文件夹可能未显示。前端侧栏直接显示该状态并提供只读重试按钮；记住/重命名操作继续先写 Tauri 本地 catalog，再返回同一合并结果，不改变旧版目录只读边界。
- 非测试验证：Tauri `cargo fmt --check && cargo check --locked`、前端 `npx tsc --noEmit -p tsconfig.json`、`git diff --check` 均通过。未运行测试或访问真实 profile；旧 Wails writer 停写及旧 Tauri host 兼容仍未认证。

## 29. 离线导入审核文件读取边界

- `reasonix-session-import` 读取 stage marker、选择模板和审核计划时，现通过已打开文件句柄校验普通文件、打开前后的文件身份和有界大小；Unix 类系统拒绝组/其他用户可读写的审核文件。这样避免 `Lstat` 后路径被替换，以及宽权限计划包含的本地绝对路径意外暴露。Windows 沿用平台文件权限语义，不声称此检查认证其 ACL。
- 非测试验证：`gofmt`、`GOCACHE=/private/tmp/reasonix-tauri-go-build-cache go build ./...`、`git diff --check`。未运行测试、CLI 导入流程或访问真实 profile。

## 30. Dirty shadow 下完整身份目录只读分页（2026-09-26）

- 原逻辑对 workspace/order/物理状态/inventory 之外的 dirty shadow 直接退回最多 50 条 workbench JSON，身份目录超过 50 条时不可见。现在 shadow 报告可读时，host 仍请求 identity page，并要求 identity snapshot ID、total、游标位置、ID、workspace、lifecycle 与同轮完整 shadow projection 逐页一致；匹配后以 `identity_unverified` 展示全量已登记身份行。旧 catalog 中找不到 identity 的条目作为独立 `unverifiedLegacySessions` 列表返回，不混入身份页游标。
- `identity_unverified` 来源打开、删除和新建均由前端禁用，并在调用函数再设门禁；项目组选择也禁用。标题/项目分组使用的是 identity metadata，但侧栏与诊断明确标记未核验并展示 shadow 差异；仅旧 catalog 项单独标记、不可操作。由此只提高可见性，不把 SQLite 或 JSON 任一方提升为冲突情况下的操作权威。
- 首屏缓存保留 safe/unsafe 标志、shadow directory projection 与 legacy-only 行。在线续页先以 bridge snapshot 与 SQLite page 对照缓存，避开每页重复完整物理盘点；sidecar 离线时同样只用 stale/cached 标签展示、禁用操作。shadow 不可读时若 bridge identity page 可读，则改用 `identity_unverified` 并按 snapshot ID/total 绑定续页；只有首屏 identity page 也不可用时才回退最多 50 条 legacy catalog。identity continuation 失败会重新读取首屏，不切换或拼接来源。
- managed Tauri Preview 的 scan-only 审核入口直接写入隔离 Preview identity store；旧 Wails profile 停写与旧 Tauri host 发布二进制兼容仍未认证。非测试验证：Tauri `cargo fmt --check && cargo check --locked`、前端 `npx tsc --noEmit -p tsconfig.json`、Go `go build ./...`、`git diff --check`；未运行测试或访问真实 profile。

## 31. Shadow 不可用时继续显示未核验身份分页（2026-09-26）

- 首屏完整 shadow audit 失败时，host 现在仍尝试读取 bridge identity page；若该页通过 bridge 自身的 schema、顺序、状态、路径 containment 和 snapshot 校验，就返回 `identity_unverified`。不读取/推断旧目录独有项，不附加 shadow 统计；前端以“shadow 报告不可用”说明边界，并禁用会话操作和项目选择。
- 续页只有在 incoming cursor 带有效 snapshot ID、非零 total，且 bridge 返回页的 snapshot ID 与 total 均匹配时才显示。继续页请求失败或版本不支持稳定 cursor 时返回错误，让前端重新取首屏，不把 identity 与 50 条 legacy 数据拼在一起。shadow 与身份页首屏都不可用时，仍显示最多 50 条 legacy fallback。
- 非测试验证：Tauri `cargo fmt --check && cargo check --locked`、前端 `npx tsc --noEmit -p tsconfig.json`、`git diff --check`。未运行测试或访问真实 profile；旧 writer 停写及旧 Tauri host 二进制兼容仍未认证。

## 32. 本次提交前复核（2026-09-26）

- §7 中的标题分页测试陈旧状态已修正：测试现在验证标题变化不会使结构快照失效。Rust 90 项测试通过。
- 修正 dirty shadow 首屏提前回退旧目录的分支。结构差异下，匹配同轮 shadow 的身份页显示为 `identity_unverified`，旧目录独有项单列；续页仍要求快照匹配。新增对应首屏与续页断言。
- 前端项目文件夹测试桩已同步 host 的 `{ folders, warning? }` 返回格式，相关前端用例和 TypeScript 类型检查通过。Go 身份库、SQLite DAG、bridge 和离线导入定向测试通过；全仓 `go test ./... -run '^$'` 编译通过。完整 Go 测试套件在当前沙箱因 `httptest` 无法监听本地端口而中断，未据此宣称全量测试通过。
