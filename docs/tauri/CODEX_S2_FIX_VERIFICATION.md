# Codex S2 修复核对报告

> 对照文档：[CODEX_S2_REVIEW.md](./CODEX_S2_REVIEW.md)（我上一轮的审查报告，19 条问题）
> 初始核对时间：2026-09-25；后续复核更新至：2026-09-26
> 核对方式：先按原报告做只读核对，再于 2026-09-25 复核现有工作区改动；本次修正过时结论，并记录后续小幅产品代码修复。
> 状态：审查基线为 `c8a2a8deb`；已核对阶段改造于 `0b3bf8fba` 提交（44 文件 / +5,026 −343）。
> 下文 2.5 的流式哈希优化、中断删除恢复入口与 CI 门禁修复是该提交后的跟进实现。
> 回归验证：`go test ./internal/sessionidentity/... ./cmd/reasonix-desktop-bridge/` **全部通过**。

## 0. 结论摘要

| 严重度 | 总数 | 已修 | 部分缓解 | 仍存在 | 未证实 |
| --- | --- | --- | --- | --- | --- |
| 高（功能） | 4 | **2** | **1** | 1 | 0 |
| 中（性能/一致性） | 8 | **1** | **1** | 5 | 1 |
| 低（细节） | 7 | **1** | 0 | 5 | 1 |
| **合计** | **19** | **4** | **2** | **11** | **2** |

**下表和 §7.4 的分类是 2026-09-25 初始核对时点的历史快照；当前状态以 §8–§63 的后续复核及 `CODEX_S2_REVIEW.md` §7–§46 为准。** 尤其 1.4 后续新增了 schema 6 generation/revision 续页快路径，不再是“每页全量审计仍在”；缓存续页、启动诊断和新建/删除后的可见列表刷新会复用首屏 shadow 报告，减少重复 inventory。title-only drift、inventory 确认的 missing identity rows 或仅有未认领文件差异时，partial identity 来源会继续分页已验证的身份行；其他可读结构/物理冲突通过 `identity_unverified` 只读分页呈现。shadow 或旧兼容 catalog 不可读时也会尝试分页显示未核验身份目录，只有首屏身份页不可用才退回最多 50 条 legacy fallback；identity continuation 出错会重新读取首屏，不拼接来源。未核验身份页在刷新时会保留已加载页数，离线缓存续页按有序 cursor 二分定位，详见 §51–§52。首次目录核验前及 cached/unverified 来源下的会话操作现由 handler 与 UI 双重拒绝，详见 §53。项目目录文件启动读取失败后，侧栏重新检查会重新访问本地文件，详见 §60；没有 cursor 的 legacy fallback 项目组也会保持“历史未核验”，详见 §61。managed Tauri Preview 已接入 scan-only 用户审核弹窗；稳定版 Wails profile 与旧 host 发布二进制仍未认证，见 Review §26–§27。最近加入的 WAL checksum fast path 以拒绝当前 salt 下的损坏帧、隔离检查未知或旧帧尾为界，详见 §49。项目文件夹清单读取失败现在会显示来源告警并支持重新读取，见 Review §28、§44。Review §46 的历史 host 源码隔离构建仍只是编译证据，没有升级旧发布二进制的认证状态。

**初始核对时已修的 4 条**：

- **1.1 中断删除缺少独立发现入口** —— 已增加仅含 ID 和标题的恢复清单；普通目录页失败时仍可显示，用户确认后续做删除。
- **1.2 标题变化导致分页失效** —— 已修，且测试同步更新并新增反向用例。
- **2.6 `ApplyImportReview` 的 TOCTOU 只靠注释约束** —— 已修，改为函数内强制获取 profile 锁。
- **3.1 两个 catalog 端点对坏 JSON 返回空响应** —— 阶段提交后的跟进修复已在 `session_catalog_import.go` 与 `session_catalog_sync.go` 返回协议化 `invalid_request`，`TestSessionCatalogEndpointsReturnProtocolErrorForMalformedJSON` 覆盖两个端点。

**另有 1 处是原审查结论有误**，见 §3：1.3 中"用户得不到任何提示"不成立，
UI 实际上已有明确提示与重试按钮。原报告的 1.1 与 1.4 在 2026-09-25 初始复核时已按当时改动更新：
1.1 增加独立的显式恢复入口且启动期不自动删除；1.4 改用单次有界快照接口，但当时续页仍重复完整影子审计。
后续 schema 6 generation/revision 快路径见 §12–§13。阶段提交后，2.5 另做了一次安全的读放大优化：复制时流式计算目标摘要，保持源文件复读比对与最终
`VerifyOfflineSnapshot`；该项由四遍整文件读取降为三遍，属于部分缓解。
2.3 与 3.7 经复核未证实为问题：导入会保留缺失条目为 `missing` identity；空 catalog 响应中的 `Accepted` 由 JSON 正常编码为 `0`。

---

## 1. 已修复（逐条验证）

### 1.1 [编号 1.2] 标题不再使分页失效 ✅

**修改前**：`visibleSnapshotID` 把 `title`、`updated_at_ms` 一起哈希，任何重命名或首轮标题回填
都会让续页返回 `ErrDirectoryChanged`。

**修改后**（`internal/sessionidentity/store.go:928-947`）：

```sql
SELECT id, relative_path, workspace_root, position, state FROM sessions
WHERE state NOT IN ('deleting','deleted') AND (?='' OR workspace_root=?)
ORDER BY position, id
```

只取**结构列**（`id` / `relative_path` / `workspace_root` / `position` / `state`），
`title` 与 `updated_at_ms` 已从快照剔除；哈希逻辑抽到 `visibleSnapshotHasher`（`:963-985`）。

**测试同步更新**：

- 原用例改为**结构性变更**触发失效：`store_test.go:397` 现在是
  `UPDATE sessions SET position=position+10 WHERE id=?`，期望仍是 `ErrDirectoryChanged` —— 语义正确。
- 新增反向用例 `TestListVisibleAllowsTitleOnlyChangeDuringPagination`（`store_test.go:405` 起），
  专门锁定"仅标题变化不得使分页失效"。

这条修得干净：既保留了对外部目录变更的检测，又消除了日常使用中的误报。

### 1.2 [编号 2.6] 导入审查现在强制持有 profile 锁 ✅

**修改前**：`ApplyImportReview` 只在注释里要求"调用方先静默写入者并持锁"，无代码强制。

**修改后**（`internal/sessionidentity/reviewed_import.go:226-235`）：

```go
releaseProfile, err := profilegate.TryAcquire(s.profileRoot)
if err != nil {
    return ImportReviewResult{}, fmt.Errorf("session profile ownership: %w", err)
}
defer releaseProfile()
```

并且入参前置校验也补上了（`:226-228`：`plan.SessionDir` / `plan.CatalogPath` 为空即返回
`ErrImportReviewChanged`）。注释也相应改为准确表述：**函数自己获取 gate**，
调用方只需另行静默"不参与该 gate 的旧 Wails 版本写入者"（`:221-223`）——这个残余前提是合理的，
因为旧进程确实无法遵守新锁。

---

## 2. 未完全解决或仍存在（13 条：2 条部分缓解，11 条仍存在；2.3、3.7 未证实）

### 2.1 高严重度（1 条恢复入口已补足，1 条仍存在，1 条部分缓解）

| 编号 | 问题 | 核对证据 |
| --- | --- | --- |
| **1.1** | **没有启动期自动续做（有意保留），直接可发现性已补足** | 启动时仍不无提示地自动删除；新增 path-free `GET /v1/sessions/deletion-recovery`、受 health capability 保护的 Rust 命令和侧栏“待完成删除”区域。用户可显式确认后重试；不在普通列表开放或恢复会话。 |
| **1.3** | **shadow 完全不可用时的 legacy 回退仍不提供分页** | legacy JSON 合同最多 50 条。shadow 可读但有结构/物理差异时，当前走 `identity_unverified` 并分页完整身份目录，旧目录独有项单独显示，所有会话操作禁用；shadow 不可用或 page 与同轮 shadow snapshot 不匹配时，才回退至 `next_cursor: None` 的最多 50 条列表。侧栏说明该上限与审计数量的区别。 |
| **1.4** | **每页仍重复全量影子审计（后续已进一步缓解）** | 首屏仍执行 fresh `compare_session_catalog_with_directory` 与物理 inventory；同一有效快照下的续页校验 SQLite revision 并读当前页，不重做物理盘点。前端启动、会话变更刷新复用页面携带的 shadow 报告，不再紧接着重复请求 inventory。显式诊断重查仍执行 fresh audit；外部文件系统变化要靠首屏重查发现。 |

### 2.2 中严重度（5 条仍存在，1 条部分缓解，1 条未证实）

| 编号 | 问题 | 核对证据 |
| --- | --- | --- |
| **2.1** | 每次变更 O(n) 次 syscall | `store.go:418` `ensureTranscriptPathAvailable` 仍 `SELECT id, relative_path FROM sessions WHERE id<>?`（全表），逐行 `resolveTranscriptPath`（内部 `EvalSymlinks`）＋ `os.Stat` |
| **2.2** | 打开库可能整库拷贝 | `path.go:147` 仍调用 `inspectWALSchemaOnCopy`（`:153`），WAL 非空即复制 `.sqlite`+`-wal`+`-journal`。`Open` 仍在热路径（`core_runtime.go:108`、`:135`） |
| **2.3** | `Accepted`/`Synced` 计数与实际写入不符（未证实） | 当前代码以 `Accepted` 表示接纳的目录项，而非新插入行；`ImportLegacyCatalog` 使用 `preserveMissing=true`，缺失 transcript 也会建为 `StateMissing`。`SyncWorkbenchOrder` 只排除 deleting/deleted，故 missing 行仍计入 `listedPosition`。`session_catalog_import_test.go` 与 `store_test.go` 有保留缺失项的回归覆盖；现有证据不支持“合法缺失会被跳过并导致计数不匹配”。 |
| **2.4** | 路径归一化三层不一致 | `store.go:819`、`:831` 等处仍是大小写敏感的 `workspace_root=?`；Rust 组键小写化（`workbench_projects.rs:158-173`）、前端同样（`workbenchSessions.ts:15-25`）。目前仍未接工作区过滤（`tauriBridge.ts:270` 只传 limit/cursor），故仍是**潜在**问题 |
| **2.5** | 离线快照多次哈希（部分缓解） | `copyVerifiedSnapshotFile` 现在在复制时流式计算副本 SHA-256，仍复读源文件比对，并由 `CreateOfflineSnapshot` 最终执行完整 `VerifyOfflineSnapshot`。与原先复制后分别重读目标和源相比，去掉了一遍目标读取，尚保留源稳定性检查和发布前端到端验证。 |
| **2.7** | `SyncWorkbenchOrder` 50 条硬限制 | `catalog.go:32` 仍 `len(entries) > 50` 直接报错（`:159` 是 `ImportWorkbenchCatalog` 的同款限制） |
| **2.8** | 恢复路径用读写 `Open`（有意取舍） | `retryInterruptedSessionDelete` 是 DELETE 写路径：`BeginDelete` 与 `FinishDelete` 要持久化 fence/tombstone，故使用经 schema/路径预检的读写 `Open`，旧 schema 可能在失败重试前迁移。独立的 GET 恢复清单与目录清单仍使用 `OpenReadOnly`；代码注释已明确区别。 |

### 2.3 低严重度（5 条仍存在，1 条已修，1 条未证实）

| 编号 | 问题 | 核对证据 |
| --- | --- | --- |
| 3.1 | 解析失败返回空 400（已修） | 两个 catalog handler 现在在 `decodeJSONBody` 出错时返回协议化 `invalid_request`；`session_catalog_request_errors_test.go` 分别验证状态码和响应 envelope。 |
| 3.2 | 死代码（已清理） | `bridge_session_directory_page` 没有前端调用方，本轮已从 Tauri command handler 与 `invoke_handler` 注册表中移除；仅用于测试包装器的 workspace snapshot helper 限定为 `cfg(test)`。 |
| 3.3 | 重复计算 | `store.go:407` 仍丢弃 `relativeTranscriptPath` 的返回值 |
| 3.4 | 平台策略无提示 | `path.go:33-38` `sameCaseInsensitivePath` 未变，错误信息未点明平台策略 |
| 3.5 | 快照包含可重建缓存 | `offline_snapshot.go:82-117` 仍整树拷贝 profile（含 `cache/`） |
| 3.6 | `previewRoot` 命名过时 | `store.go:521`、`:583` 仍用 `previewRoot`，实际传的是 session dir |
| 3.7 | 空数组提前返回语义含糊（未证实） | `Accepted` 是非 `omitempty` 的 `int` 字段，JSON 会明确输出 `"accepted":0`；空目录成功导入 0 条，现有响应语义明确且可解析。 |

---

## 3. 我上一轮审查的**错误**（需更正）

### 3.1 [编号 1.3] "用户得不到任何提示" —— 不成立 ❌

我在报告里写"用户得不到任何'数据其实更多'的提示"。实际代码**已经有明确提示**：

当时侧栏已明确写出“最多 50 条”与“列表可能不完整”，并提供手动重新检查入口；因此“无任何说明”的判断不成立。后续 §31 又把可用的 shadow 差异类别直接加入侧栏提示。具体当前实现见 `desktop/frontend/src/tauri/TauriChatWorkspace.tsx`，不再保留已过时的行号示例。

**更正后的结论**：1.3 的真实问题只剩"legacy 分支无法翻到 50 条之后"，
而不是"静默降级"。该容量限制仍影响目标，但其用户提示与重试路径已存在。

（这条误判的原因：我只读了 Rust 侧 `main.rs` 的分支逻辑，没有追前端如何消费 `source` 字段。
教训是"未覆盖部分"里我自己标注过的 `TauriChatWorkspace.tsx` 分页状态机，
恰恰就是这条结论的依据。）

---

## 4. 当前状态与建议

**当前建议（按剩余风险排序）**：

1. **旧 Wails writer 停写确认与旧 Tauri host 发布二进制兼容认证**仍是独立发布门禁；不能用当前源码或测试 harness 替代。
2. **2.1 / 2.2 的热路径开销**仍存在；任何进一步优化都需保留 transcript 物理唯一性检查及 WAL/schema 保守校验，并先有可信失效机制。
3. **1.3 legacy fallback**在 shadow 不可用或身份页无法匹配审计 snapshot 时仍最多 50 条；可读的 dirty shadow 使用全量 `identity_unverified` 只读分页，并同时显示旧目录独有项。

**1.4 当前状态**：每页不再重复物理 inventory；schema 6 使用 generation/revision 验证续页，schema 5 或 revision 元数据/trigger 缺失仍有 O(n) 哈希回退。没有新性能基准，不宣称实测延迟收益。1.1 有独立发现与用户确认重试入口，不在启动时静默删除；2.3、3.7 原判定未证实。

**仍有意保留**：2.4 工作区过滤尚未接线；2.5 离线快照维持源稳定性与完成后验证；2.7/2.8 维持 bounded catalog 与写入恢复语义；旧 writer、旧 host 及真实 profile 认证均不从源码推断。

**回归状态**：阶段提交前 `go test ./internal/sessionidentity/... ./cmd/reasonix-desktop-bridge/`、Rust host
81 项、前端专项 95 项与前端 typecheck 均通过。提交后的 2.5 优化通过 `go test ./internal/sessionidentity`
与 `go test -race ./internal/sessionidentity`；3.1 修复后 bridge 与 sessionidentity 两个 Go 包通过，格式与 diff 检查通过。

---

## 7. 独立复核（DeepSeek，2026-09-25 追加）

本节由 DeepSeek 追加，用于**独立验证**上文（含 Codex 对本文的修订）中的状态判定。
未改动上文任何内容。核对对象：`0b3bf8fba` 提交 + 当前工作区（6 个未提交改动）。

### 7.1 已逐条独立确认的修订

| 上文判定 | 我的独立核验 | 结论 |
| --- | --- | --- |
| **3.1 已修** | 两个 handler 现在都调用 `writeProtocolError(..., "invalid_request", ...)`（`session_catalog_import.go:31-33`、`session_catalog_sync.go:25-27`），并有新测试 `TestSessionCatalogEndpointsReturnProtocolErrorForMalformedJSON`（`session_catalog_request_errors_test.go:11`，断言 `body.Error.Code != "invalid_request"`） | **确认属实** |
| **2.5 部分缓解** | `copyVerifiedSnapshotFile` 改为 `io.Copy(io.MultiWriter(writer, streamHash), ...)`（`offline_snapshot.go:450-451`），副本摘要在复制时流式产生；源复读比对（`:474`）与最终 `VerifyOfflineSnapshot` 仍在 | **确认属实**（4 遍 → 3 遍） |
| **2.3 未证实** | `ImportLegacyCatalog` 调用 `importCandidates(..., requirePresent=false, preserveMissing=true, preserveExisting=true)`（`store.go:554`）；`:642-643` 表明 `preserveMissing=true` 时缺失条目**仍会插入**，且插入使用 `incoming.State`（`:655-660`，即 `StateMissing`）；`SyncWorkbenchOrder` 的 `listedPosition` 只排除 `deleting/deleted`（`catalog.go:108-133`），故 missing 行**会计入** | **确认原 2.3 不成立**，我的原判定有误 |
| **3.7 未证实** | `Accepted int \`json:"accepted"\``（`session_catalog_import.go:26`）**无 omitempty**，JSON 恒输出 `"accepted":0`；空数组时"尝试 0 条"与"写入 0 条"本就一致 | **确认原 3.7 不成立** |
| **1.4 部分缓解** | `session_directory_snapshot_for_workspace` 现在只发**一次** `GET /v1/sessions/snapshot`（`bridge.rs:819-824`），旧的按 200 条循环已移除；但 `workbench_session_page` 仍**每页**调用 `compare_session_catalog_with_directory`（`main.rs:462-463`） | **确认属实**：循环请求已修，每页重复全量审计仍在 |
| **1.1 需修正限定** | 见 7.3：重试路径确实存在（经 legacy 回退使其可见），但仍无启动期自动续做 | **确认属实，我原表述需更正** |

### 7.2 当时新增发现：CI clippy 门禁失败（现已修复）

本阶段修复前，`cargo clippy --locked -- -D warnings`（CI 门禁位于 `.github/workflows/ci.yml:617-619`）报 2 个错误并编译失败：

| # | 错误 | 位置 | 说明 |
| --- | --- | --- | --- |
| 1 | `method 'session_directory_snapshot' is never used` | `desktop/tauri/src/bridge.rs:800` | 该薄封装已无调用方（`compare_session_catalog_with_directory` 用的是 `session_directory_snapshot_with_id`）。是重构到单次快照接口后的遗留死代码，与 3.2 同类问题 |
| 2 | `current MSRV is 1.77.2 but this item is stable since 1.83.0` | `desktop/tauri/src/main.rs:275` | 使用了 `std::io::ErrorKind::NotADirectory`；`Cargo.toml:7` 声明 `rust-version = "1.77.2"`，因此在声明的 MSRV 上**根本无法编译** |

**归因（已核实，非本次提交引入）**：

- `git log -S "NotADirectory" -- desktop/tauri/src/main.rs` → 由 **`c1ddc5bb4 feat(tauri): show unavailable project workspaces`**（2026-09-24 16:24）引入，距 HEAD **26 个提交**，不是 `0b3bf8fba` 带入的。
- `session_directory_snapshot` 的 dead code 也不在 `0b3bf8fba` 的 diff 中。
- 这两个错误此前已在分支上存在，与阶段提交 `0b3bf8fba` 无关。

本阶段移除了只供测试调用的 `session_directory_snapshot` 薄封装，两个测试改用现有的 `session_directory_snapshot_with_id`；工作区可用性检查通过逐级检查非目录父节点处理相同场景，避开 Rust 1.83 才稳定的 `ErrorKind::NotADirectory`。本地 clippy 门禁现已通过；这不等于用 Rust 1.77.2 编译器完成了独立 MSRV 验证。

**其余门禁实测结果**：

| 门禁 | 结果 |
| --- | --- |
| `cargo fmt --check` | 通过 |
| `cargo test --bin reasonix-tauri` | **82 passed**（本阶段更新） |
| `cargo clippy --locked -- -D warnings` | **通过**（本阶段更新） |
| `go test ./internal/sessionidentity/... ./cmd/reasonix-desktop-bridge/` | 通过 |
| 前端 `tsc --noEmit` | 通过 |

### 7.3 对 1.1 的更正（原报告表述及本阶段后续处理）

我原先写"会话从 UI 消失且**无法重试**"。经核实，完整链条是：

1. 删除中断 → identity 行为 `state='deleting'`；
2. `deleting` 被排除出身份库目录（`store.go:819`）；
3. 但 host 的 legacy catalog **仍保留该行**（`forgetTauriWorkbenchSession` 只在删除成功后才移除）；
4. 于是影子比对 `missing_from_directory > 0`（`session_shadow.rs:63-68`）→ `legacy_matches_directory = false`（`:114`）；
5. UI 因此**回退到 legacy 列表**（`main.rs:498-506`），并在侧栏显示"最多 50 条；列表可能不完整"的提示；
6. 该行在 legacy 列表中**可见**，用户再点删除即可续做（前端会在 switch 失败后直接调 DELETE，`TauriChatWorkspace.tsx:1029-1040`）；测试 `TestBridgeRetriesInterruptedDeleteAfterRestart` 覆盖了这条续做路径。

**更正后的准确表述**：中断删除**不是**"静默黑洞"，而是让整个侧栏降级为 legacy 模式（50 条上限 + 警告），并依赖用户手动重试该行才能收敛。
本阶段已补充 path-free 独立恢复列表与显式确认重试，因此即使 identity 不在 legacy catalog 中也可发现；仍不做启动期无提示自动删除，这是为了保留用户确认。

### 7.4 复核后的状态汇总（本阶段更新：19 条 = 4 + 2 + 11 + 2）

| 状态 | 条数 | 编号 |
| --- | --- | --- |
| 已修 | 4 | 1.1（补足中断删除的独立发现与显式重试）、1.2（标题不再使分页失效）、2.6（导入强制持锁）、3.1（协议化错误响应） |
| 部分缓解 | 2 | 1.4（循环请求已改为单次快照，每页全量审计仍在）、2.5（流式哈希，4 遍 → 3 遍） |
| 仍存在 | 11 | 1.3、2.1、2.2、2.4、2.7、2.8、3.2、3.3、3.4、3.5、3.6 |
| 我的原判定不成立 | 2 | 2.3、3.7 |
| **另有新增发现（已修）** | **1** | **CI clippy 门禁失败（§7.2，分支既有、非阶段提交引入）** |

其中需要说明的两条：

- **1.3** 归入"仍存在"而非"部分缓解"：侧栏提示与重试按钮**在我上一轮审查时就已经存在**
  （见 §3 的更正），并非本次修复带来的缓解；本条待办仍是"legacy 回退无法翻到 50 条之后"。
- **1.1** 已从遗留项移至已修：恢复入口独立于 legacy catalog；启动期自动续做仍不属于本阶段行为。

## 8. 后续工作区复核（2026-09-25，基线 HEAD `c95688000`）

本节更新 §7 的后续源码状态；§7 保留当时的审查记录。此次只做非测试编译、类型检查和 diff 检查，没有运行测试或访问真实 profile。

| 编号 | 当前状态 | 当前证据 |
| --- | --- | --- |
| **1.1** | 删除中断可独立发现并由用户显式续做；不自动删除 | GET recovery 清单和侧栏重试行仍存在。 |
| **1.2** | 标题变化不使 cursor 失效 | `visibleSnapshotID` 只覆盖结构列。 |
| **1.3** | 可读 dirty shadow 改为全量只读身份分页；legacy fallback 仍最多 50 条 | `identity_unverified` page 与同轮 shadow snapshot 逐页核对，旧目录独有项另列，所有会话操作禁用。只有 shadow 不可用或 page/snapshot 不匹配才回退到 bounded legacy JSON。 |
| **1.4** | 续页不再重复完整物理 inventory；schema 6 由 SQLite revision 验证结构 cursor，旧 schema/trigger 缺失回退 O(n) 哈希 | `SessionShadowSnapshotCache` 只在 clean 首屏建立；schema 6 同事务读取 revision 并由触发器覆盖结构列变化，续页携带首屏总数并复用 revision 验证。schema 5 或 revision 表/行/触发器不完整时用覆盖索引重算结构哈希；文件系统外部变化需显式重新检查。性能尚未基准验证。 |
| **2.3** | tombstone catalog 行现在按 ID/路径识别为已同步，但继续排除可见顺序 | `SyncWorkbenchOrder` 将匹配的 `deleting/deleted` 行计入完整性返回值，未知 ID 仍拒绝。 |
| **3.2** | 未使用的生产 Tauri invoke 已移除 | `bridge_session_directory_page` 已从 command 与 `invoke_handler` 删除；测试专用 workspace snapshot helper 标为 `cfg(test)`。 |
| **3.3** | 重复 canonicalization 已减少 | `resolveTranscriptPathWithIdentity` 将 lexical 和已校验 resolved path 一起返回，唯一性检查复用后者。 |
| **3.6** | 命名已清理 | session directory 导入参数现名为 `sessionDir`。 |

当前组合工作区非测试验证：`go build ./...`、`cargo fmt --check`、`cargo check`、前端 `tsc --noEmit`、`git diff --check` 均通过。旧 Wails writer 停写、旧 Tauri host 发布二进制兼容和真实 profile 停写后恢复仍未认证。

## 9. 继续复核（2026-09-25）

- legacy 回退下的“重新检查会话目录”现在重试有界、幂等的 legacy catalog 导入，然后重新读取受保护的首屏；导入失败且页面仍来自 legacy 时会显示错误。50 条 fallback 上限仍未解除。
- 回退页现在附带最近一次审计读到的 identity 目录条数；只要审计数量与当前展示条数不同，侧栏就显示该数并说明它仅为审计数量，不代表兼容回退列表可分页。它不会让未通过 shadow 校验的身份记录进入分页或会话打开路径。
- 完整 shadow 审计通过且目录有续页时，当前进程保留无 transcript 路径的已验证快照。shadow 请求不可用（无论 bridge 是否仍报告运行）时可从快照继续分页，数据源标为“上次已校验快照”；成功返回的 dirty 审计会清除此快照，首次不可用启动仍使用受限的 50 条 legacy 列表。缓存快照可能陈旧，cached 模式禁用普通会话的打开、删除、新建；独立待完成删除列表仍可显式重试。服务恢复后需重新检查。
- 该字段经 Tauri `cargo fmt --check && cargo check`、前端 `tsc --noEmit` 和 `git diff --check` 验证通过；本次增量未改 Go 源码，因此未重跑 Go 编译。未运行测试。
- macOS/Windows 上仅大小写不同的路径冲突会解释保守别名策略；macOS 卷大小写行为可能不同。冲突策略未放宽。
- 本轮仅做 `go build ./...`、Tauri `cargo fmt --check && cargo check`、前端 `tsc --noEmit` 与 `git diff --check`；未运行测试，未访问真实 profile。旧 writer 停写及旧 host 二进制兼容仍未认证。

## 10. bridge 离线时的进程内目录续页（2026-09-25）

- 仅在 clean shadow 审计通过且目录超出首屏时，Tauri 保留当前进程内的 path-free 目录快照；sidecar 明确离线后可显示/翻页，来源标为 `cached`。
- 快照可能过期，因此 cached 模式禁用普通会话的打开、删除和新建；独立 pending-delete recovery 仍可显式重试。实时 shadow 恢复后显式重新检查，再回到实时来源。成功返回的 dirty shadow、目录结构变化或首次不可用启动均不使用缓存，仍受原 legacy 50 条限制。
- 本轮增量通过 `cargo fmt --check`、`cargo check`、前端 `tsc --noEmit`、`git diff --check`；Go 源码未变，未运行测试或访问真实 profile。旧 Wails writer 停写与旧 Tauri host 二进制兼容仍未认证。

## 11. 续页哈希覆盖索引（2026-09-25）

- 新增 `sessions_visible_structure_snapshot(position,id,state,relative_path,workspace_root)` 覆盖索引；`visibleSnapshotID` 在索引存在时显式使用它，继续对同一批结构字段计算原 SHA-256，因此 cursor 语义未变，但每页结构扫描不再逐行回表。扫描复杂度仍是 O(n)，还未引入数据库 revision 快速路径。
- visible total 现在在同一次覆盖索引哈希扫描中累计，去掉独立 `COUNT(*)` 全目录遍历；本轮未做性能基准，因此不声称有可测的延迟改善。
- 本轮只改 Go 与文档；非测试验证为 `gofmt`、`GOCACHE=/private/tmp/reasonix-tauri-go-build-cache go build ./...`、`git diff --check`。未运行测试或访问真实 profile。

## 12. 续页结构 revision（2026-09-25）

- schema 6 新增随机 generation 与 revision 行，并安装对 `sessions` 插入、删除及目录结构列更新的 SQLite triggers。游标 snapshot ID 绑定 generation、revision 与 workspace filter；标题和时间字段不参与 revision，因此标题修改不打断分页。revision 元数据和结构页在同一个只读事务快照中读取。
- 首屏/无总数的旧游标仍需一次 visible count；新续页游标携带 total，可用 revision 做 O(1) 结构一致性检查再读取有限页面。schema 5、revision 行或任一预期 trigger 不存在时，继续覆盖索引结构哈希及计数回退。此机制不认证旧 Wails writer 停写，也不认证旧 Tauri host 发布二进制兼容；不涵盖外部文件系统 transcript 变化。
- 本轮非测试验证：`GOCACHE=/private/tmp/reasonix-tauri-go-build-cache go build ./...`、Tauri `cargo fmt --check && cargo check`、前端 `tsc --noEmit -p tsconfig.json`、`git diff --check`。未运行测试、未访问真实 profile，未做性能基准。

## 13. Revision 快路径 snapshot ID 对齐（2026-09-26）

- 复核发现 schema 6 下 `/v1/sessions` 使用 generation/revision snapshot ID，但 `/v1/sessions/snapshot` 仍返回完整结构哈希；Tauri shadow 门禁会将两者判为不同目录，拒绝身份分页。`ListVisibleSnapshot` 现与分页接口共用 `visibleSnapshotID`，并在同一 SQLite 只读事务中读取 ID 和快照行。schema 5/缺 revision 元数据时两者都走相同完整结构哈希回退。
- 本轮非测试验证：`gofmt`、`GOCACHE=/private/tmp/reasonix-tauri-go-build-cache go build ./...`、`git diff --check`。未运行测试或访问真实 profile。

## 14. 复用候选 transcript 的已验证解析（2026-09-26）

- `CheckTranscriptPathUnique`、`Reserve`、`MarkReady`、删除 fencing 与导入在执行唯一性扫描前，现将同一写事务中 `relativeTranscriptPathWithIdentity` 得到的 resolved candidate path 传给 `ensureTranscriptPathAvailable`；后者不再重复解析候选 final path。删除和 ready 路径仍在访问 transcript 前保留父目录/profile containment 校验，唯一性检查仍解析全部既有身份并检查 symlink/hard-link 冲突。
- 这是常数级重复解析减少，不代表 2.1 的 O(n) peer 检查已解决，也不替代旧 Wails writer 停写认证。非测试验证：`gofmt`、`GOCACHE=/private/tmp/reasonix-tauri-go-build-cache go build ./...`、`git diff --check`；未运行测试或访问真实 profile。

## 15. 删除恢复续页索引（2026-09-26）

- schema 6 新增 `sessions_deletion_recovery_cursor` 部分索引，只收录 `state='deleting'` 的 ID，与 `/v1/sessions/deletion-recovery/page` 的 `id>? ORDER BY id` keyset 查询匹配，避免恢复分页为无关身份行排序。schema 5 的只读兼容仍允许该查询退化执行；当前 bridge 启动期迁移会在开放 listener 前升级已有身份库。
- 本轮仅增加恢复游标索引并更新源码复核文档；未运行测试或访问真实 profile。旧 Wails writer 停写和旧 Tauri host 发布二进制兼容仍未认证。

## 16. 合并分页身份库探测与只读打开（2026-09-26）

- 会话目录分页、完整快照及待删除恢复路由原先先调用 `IdentityDatabaseExists`，随后再调用 `OpenReadOnly`，重复检查相同路径、profile containment 和 SQLite sidecar。新增 `OpenReadOnlyIfExists`，同一次调用完成位置校验、数据库/sidecar 文件检查及只读打开；身份库不存在时仍返回空目录，遗留 sidecar、路径越界、损坏 schema 与完整性错误仍失败关闭。
- SQLite 官方说明 `PRAGMA quick_check` 为 O(N)。生产 sidecar 现在惰性打开一个只读 Store，并复用于分页、shadow、inventory 与恢复查询；打开连接时仍运行完整性检查，sidecar 收到关闭信号并完成 HTTP handler 收尾后关闭该 Store。连接池保持单连接，避免并发检查/状态切换；每次 SQL 查询仍通过 SQLite 读取最新提交，不缓存行或分页结果。每次使用缓存都复查 profile containment、sidecar 类型和主数据库文件 identity；替换时当前请求失败关闭并丢弃缓存，下次请求重新完整打开。测试构造的 bridge 实例保留原有请求级打开关闭路径。此项没有移除旧 Wails writer 或旧 Tauri host 的独立认证门禁。
- 非测试验证：`gofmt`、`GOCACHE=/private/tmp/reasonix-tauri-go-build-cache go build ./...`、`git diff --check`。未运行测试或访问真实 profile。旧 writer 停写及旧 host 二进制兼容仍未认证。依据：[SQLite PRAGMA 文档](https://www.sqlite.org/pragma.html#pragma_quick_check)。

## 17. 稳定版项目文件夹标题兼容

- 稳定版项目文件夹导入不再把名称额外截短至 256 字符；现在与 Preview 项目 catalog 的 1024 字符上限一致，并过滤控制字符，避免后续 catalog 读取因单个标题使整份目录不可用。根路径、去重 key 和稳定版目录顺序保持原有语义。
- 导入目标改为同目录临时文件完整写入、权限收紧并同步后，以 `persist_noclobber` 原子发布；中断不会留下半份目标文件，已存在的目标仍拒绝覆盖。
- stable 项目文件读取和 Preview 本地项目 catalog 读取，在打开后重新检查路径类型并核对路径与已打开句柄是否仍指向同一文件；若打开期间文件被替换或变成 symlink，拒绝读取。
- 非测试验证：Tauri `cargo fmt --check && cargo check`、`git diff --check`。未运行测试，也未读取稳定版或 Preview 真实 profile。

## 18. Tauri Rust 最低版本兼容性

- 修复较高标准库版本 API 时，manifest 当时声明 `rust-version = "1.77.2"`，会话分页 metadata 和删除恢复 cursor 校验曾调用 Rust 1.82 才稳定的 `Option::is_none_or`。现已替换为等价的 `map_or(true, ...)`，保留缺失项视为通过的原判断语义。
- 后续依赖图核对发现，当前 `Cargo.lock` 的 macOS 与 Windows `normal,build` 依赖中，`notify-rust 4.18.0` 声明 `rust-version = 1.89.0`；`time 0.3.55`、`plist 1.10.1` 等声明 1.88.0。仅替换标准库 API 不足以让锁定依赖图支持 Rust 1.77.2，因此 Tauri host manifest 已更新到 1.89.0。
- 该最低版本结论来自锁定依赖元数据；本环境只有 Rust 1.95.0，未用 Rust 1.89.0 编译，也未运行测试。若要恢复更低 MSRV，需要降级并锁定整条依赖图，再使用目标工具链验证。

## 19. 未认领文件不再遮蔽已验证身份目录分页（2026-09-26）

- shadow 报告现区分“身份目录与 legacy metadata/物理盘点一致”与“整份 inventory clean”。若 `missingFromDirectory`、标题/工作区/顺序差异、缺失 transcript、物理状态差异和 inventory errors 均为 0，仅 `unclaimedCount` 大于 0，host 返回 `partial_identity`，继续分页已经登记且逐项核验的 identity rows；未认领文件只显示数量，不会自动分配或登记 ID。其他 dirty shadow 仍清理缓存并退回最多 50 条 legacy 页面。
- partial 快照续页继续匹配 shadow snapshot/cursor；缓存也保留未认领计数。bridge 离线时仍标为 cached 并禁用普通会话的打开、删除和新建，UI 同时提示快照可能过期及未导入文件数量；待完成删除恢复仍可独立显式重试。运行中的目录统计和完整旧历史仍可能不完整；新增的逐条审核导入仅通过独立离线 CLI 写入隔离恢复副本，不接入 Tauri/sidecar 在线流程，也不自动切换 Preview，见 `OFFLINE_SCAN_SESSION_IMPORT.md`。
- 非测试验证：Tauri `cargo fmt --check && cargo check --locked`、前端 `npx tsc --noEmit -p tsconfig.json`、`git diff --check`。未运行测试或访问真实 profile。旧 Wails writer 停写及旧 Tauri host 发布二进制兼容仍未认证。

## 20. 清理旧 Preview 路径术语（2026-09-26）

- `importCandidates` 与路径校验 helper 的错误说明已从旧 Preview root 改为 session directory/root；导入函数参数名也已是 `sessionDir`。
- `ensureTranscriptPathAvailable` 仍复用已验证的候选 resolved path，避免重复解析；既有 identity 的 O(n) symlink/hard-link 检查仍保留，未宣称 2.1 已解决。
- 非测试验证：`gofmt`、`GOCACHE=/private/tmp/reasonix-tauri-go-build-cache go build ./...`、`git diff --check`。未运行测试或访问真实 profile。

## 21. 唯一路径扫描复用 profile root 解析（2026-09-26）

- `ensureTranscriptPathAvailable` 将 profile root 规范化移到 peer 循环外，并在存在 peer 时只解析 root symlink 一次。每个 peer 的最近存在父目录、最终路径 containment 和 hard-link identity 检查仍逐项执行；O(n) 扫描仍在。
- 无 peer 时不额外解析 profile root；候选 path 仍由调用方在写事务内 fresh 校验。
- 非测试验证：`gofmt`、`GOCACHE=/private/tmp/reasonix-tauri-go-build-cache go build ./...`、`git diff --check`。未运行测试或访问真实 profile。

## 22. 离线快照跳过 profile 根缓存（2026-09-26）

- `CreateOfflineSnapshot` 跳过 `profileRoot/cache/`，该目录由 `REASONIX_HOME` 派生的默认 `CacheDir` 使用，包含可再生成的环境、usage/history/session/task 与插件/模型投影。profile 中其余文件和单独的 workbench catalog 仍复制并校验。
- Symlink 检查先于目录跳过；manifest/staging 只包含实际复制内容。自定义 cache home 不在 profile 根 `cache/` 时不受影响。
- 非测试验证：`gofmt`、`GOCACHE=/private/tmp/reasonix-tauri-go-build-cache go build ./...`、`git diff --check`。未运行测试或访问真实 profile。

## 23. cached 续页允许标题漂移

- cached 快照页继续验证兼容目录的 ID、顺序、workspace 与 snapshot cursor，但不再因当前 JSON catalog 的 title-only 更新拒绝快照页。页面继续使用快照标题并明确标记可能过期；cached 来源的普通会话操作仍禁用，pending-delete 恢复仍可独立显式重试。
- 在线 identity 页的 shadow 与逐页 metadata 校验未改变。
- 非测试验证：Tauri `cargo fmt --check && cargo check --locked`、前端 `npx tsc --noEmit -p tsconfig.json`、`git diff --check`。未运行测试或访问真实 profile。

## 24. 重命名后的 catalog 同步错误提示

- bridge 标题更新成功但本地 workbench catalog 写入/同步失败时，前端改为提示“标题已保存、列表同步失败”，并指向重新检查；新建/打开/恢复会话沿用“无法保存到最近对话”的原提示。
- 非测试验证：前端 `npx tsc --noEmit -p tsconfig.json`、`git diff --check`。未运行测试或访问真实 profile。

## 25. 项目文件夹合并使用规范化 key 索引

- 项目文件夹合并现在先构造规范化 project key 到结果位置的索引，避免每个本地目录反复扫描 sidecar 目录。sidecar 原始顺序、本地标题覆盖行为及本地新增根的顺序保持不变。
- 非测试验证：Tauri `cargo fmt --check && cargo check --locked`、`git diff --check`。未运行测试或性能基准，也未访问真实 profile；旧 Wails writer 停写和旧 Tauri host 发布二进制兼容仍未认证。

## 26. title-only shadow 差异继续使用身份目录分页

- 安全分页门禁现在要求 legacy ID 全部在身份目录中，且 workspace、顺序、缺失状态、磁盘状态和 inventory 检查通过；title mismatch 单独不再阻止分页。返回持久目录标题，并在首屏、续页与 cached 页面上报告标题差异数量。若同时有未认领 transcript，也分别显示其数量并保持不自动认领。
- 其他 shadow 差异仍走最多 50 条的兼容目录 fallback。非测试验证：Tauri `cargo fmt --check && cargo check --locked`、前端 `npx tsc --noEmit -p tsconfig.json`、`git diff --check`。未运行测试或访问真实 profile。

## 27. Rust 项目目录 catalog 线性去重

- Rust 本地项目目录 JSON 读取改用规范化 project key 的 `HashSet` 去重，避免最多 10,000 项的 catalog 在启动加载时逐项扫描已接收项。保留首项优先及每项路径/标题校验语义。
- 非测试验证：Tauri `cargo fmt --check && cargo check --locked`、`git diff --check`。未运行测试或性能基准，也未访问真实 profile。

## 28. 已确认缺失的身份行不阻断其余目录分页

- 若身份行状态为 `missing` 且 inventory 可读并确认 transcript 不存在，ID/workspace/order 与物理状态仍可核验。此类行随完整身份目录分页，标记缺失、禁止打开但可显式删除；host 与 UI 分别报告缺失数量。若 `ready` 行文件缺失或 inventory 状态不匹配，`physicalStateMismatches` 仍阻断身份目录分页并触发兼容 fallback。
- 非测试验证：Tauri `cargo fmt --check && cargo check --locked`、前端 `npx tsc --noEmit -p tsconfig.json`、`git diff --check`。未运行测试或访问真实 profile。

## 29. 启动诊断复用首屏 shadow 报告

- `workbench_session_page` 现在返回本轮 path-free shadow 统计报告；前端用它初始化运行状态诊断，避免加载首屏后立即再调用一次 shadow audit/inventory。shadow 不可用且没有报告时仍保留原独立重试；诊断手动刷新继续执行完整审计。
- 非测试验证：Tauri `cargo fmt --check && cargo check --locked`、前端 `npx tsc --noEmit -p tsconfig.json`、`git diff --check`。未运行测试或访问真实 profile。

## 30. 列表刷新复用页面自带的 shadow 报告

- 新建/删除等 `refreshVisiblePage` 路径不再先调完整 shadow endpoint 再请求完整 shadow 首屏；首屏结果同时更新诊断报告，续页使用 host 缓存的 snapshot。独立诊断刷新仍执行新的 shadow audit。
- 非测试验证：前端 `npx tsc --noEmit -p tsconfig.json`、`git diff --check`。未运行测试或访问真实 profile。

## 31. legacy 回退提示就地展示差异类别

- 侧栏 legacy 提示新增 path-free 差异摘要，包括身份目录缺项/新增、标题、工作区、顺序、transcript 缺失、物理状态、未认领 transcript、inventory 错误与旧目录退休行；报告缺失时说明原因不可用，报告 clean 但分页门禁失败时建议重新检查。继续注明最多 50 条与审计计数不能用于回退列表翻页，并指出重试导入不会自动认领 transcript。
- 非测试验证：前端 `npx tsc --noEmit -p tsconfig.json`、`git diff --check`。未运行测试或访问真实 profile。

## 32. 分页成功时保留导入错误

- 启动或用户重查目录时，旧 catalog 导入可能失败而当前分页仍成功。此时显示非阻断状态提示；若分页也失败，则错误提示同时呈现导入与分页原因。手动重试无论分页结果如何都会保留导入错误，独立 shadow 审计不会清除非阻断提示。加载更多、重新检查或会话变更会清掉旧提示。
- 非测试验证：前端 `npx tsc --noEmit -p tsconfig.json`、`git diff --check`。未运行测试或访问真实 profile。

## 33. 离线导入审核 JSON 的安全读取

- `reasonix-session-import` 原先先 `Lstat` 再 `os.ReadFile`，存在检查与打开之间的路径替换窗口，且读取时未检查 Unix 文件权限。现在读取 stage marker、选择模板和审核计划时，通过已打开句柄重新验证普通文件及 `os.SameFile`，限制最大字节数；Unix 类系统拒绝组/其他用户可读写的文件。Windows 不把 Go 权限位误当作 ACL 认证。
- 非测试验证：`gofmt`、`GOCACHE=/private/tmp/reasonix-tauri-go-build-cache go build ./...`、`git diff --check`。未运行测试、CLI 导入流程或访问真实 profile。

## 34. 可读 dirty shadow 下的只读身份分页

- workspace/order/物理状态/inventory 差异仍阻止可操作身份目录页，但不再隐藏所有已登记身份行。Tauri 仅在 shadow 可读、页 snapshot 一致且每页结构字段精确匹配时返回 `identity_unverified`；仅在旧目录中的条目另外显示。UI 与 `activateSession`/`deleteSession`/`createSession` 均禁用未核验来源操作。
- 首屏保存 shadow directory projection、safe 标志与 legacy-only 行；在线续页按 cached projection 对照 SQLite page，不重复物理 inventory。sidecar 离线时只返回 cached、只读快照。shadow 不可读时现也尝试通过 identity page 只读分页，并要求 cursor 的 snapshot ID 与 total 匹配；身份页首屏不可用时才保留 legacy 50 条 fallback，续页错误不拼接来源。
- 现有 managed Preview scan-import 弹窗仍要求逐项选择并确认元数据，导入只登记隔离 Preview profile；旧 Wails writer 停写与旧 Tauri host 二进制兼容仍未认证。非测试验证：Tauri `cargo fmt --check && cargo check --locked`、前端 `npx tsc --noEmit -p tsconfig.json`、Go `go build ./...`、`git diff --check`。未运行测试或访问真实 profile。

## 35. Shadow 不可用时继续分页未核验身份目录

- 首屏 shadow audit 失败时，host 现在尝试通过 bridge 的 session identity page；成功时返回 `identity_unverified`，附身份页 snapshot total/cursor，不返回旧目录独有行或 shadow 统计。UI 明确显示 shadow 尚未核验，并继续禁用打开、删除、新建与项目切换。
- 续页仅接受携带 snapshot ID 和非零 total 的游标，并要求 bridge 返回页的 snapshot ID/total 与游标一致；续页失败会拒绝拼接来源并由前端重读首屏。只有首屏 shadow 与身份页均不可用时才回退最多 50 条 legacy JSON。
- 非测试验证：Tauri `cargo fmt --check && cargo check --locked`、前端 `npx tsc --noEmit -p tsconfig.json`、`git diff --check`。未运行测试或访问真实 profile。

## 36. S2 高优先级恢复、回退与续页成本复核（2026-09-26）

- 重新逐项对照当前工作树与 `CODEX_S2_REVIEW.md` §1.1、§1.3、§1.4：中断删除由独立 pending-delete keyset page 展示并显式重试；会话目录只有 identity 首屏也不可读时才退回最多 50 条 legacy，并在侧栏说明上限、差异和审计计数不能继续翻页；已缓存首屏 shadow projection 的 identity continuation 以 snapshot ID/total 和逐页结构字段校验，不重复全量 physical inventory。无下一页时不会建立续页缓存。
- 续页的 SQLite cursor revision/generation 与缓存仍不能检测不经受支持 Tauri 写入路径发生的文件系统外部变化；用户显式重新检查会重新做首屏审计。旧 Wails writer 停写、旧 Tauri host 发布二进制兼容及真实 profile 演练仍未认证，不据此扩大结论。
- §2.1 的物理别名扫描涵盖所有 lifecycle state：`missing`、`deleting`、`deleted` 仍保留路径身份；当前不能只扫 `ready` 行或把这些路径交给其他 ID。剩余 O(n) 需要可验证的文件身份索引与失效规则。
- 当前工作树非测试验证：`GOCACHE=/private/tmp/reasonix-tauri-go-build-cache go build ./...`、Tauri `cargo fmt --check && cargo check --locked`、前端 `npx tsc --noEmit -p tsconfig.json`、`git diff --check` 均通过。未运行测试或读取/修改真实 profile。

## 37. S2 设计文档同步 dirty-shadow 只读分页（2026-09-26）

- 复核发现 S2 设计稿与实现总览仍将所有目录差异/dirty shadow 描述为回退 JSON，且把 cached 来源限定为此前 clean 的 shadow 快照。这与当前 host 不符：shadow 可读且 identity page 与完整 projection 同 snapshot 匹配时，返回 `identity_unverified` 只读分页并隔离 legacy-only 行；shadow 不可用时也尝试身份页；匹配的 dirty 快照可继续以 cached/只读/未核验标签显示。只有身份页首屏不可用才使用最多 50 条 legacy fallback；不匹配或续页失败不会拼接来源。
- 同步更新 `SESSION_STORAGE_S2_PERSISTENT_CATALOG_DESIGN.md` 的状态、目标和 5.3 验收契约，以及 `SESSION_STORAGE_IMPLEMENTATION_V3.md` 对 dirty/cached 页面和重新盘点行为的描述。未改产品代码。
- 本轮验证：`git diff --check` 通过；未运行测试或读取/修改真实 profile。

## 38. 实现总览对齐身份库迁移版本与回退语义（2026-09-26）

- 当时 `internal/sessionidentity/store.go` 的 `schemaVersion` 升至 8：S2 相对路径、标题意图与目录 generation/revision 分别在 v4–v6 引入；v7/v8 是另行记录的事件流表与 checkpoint 水位。该时点更新了 V3 总览，避免继续称当前库为 v6，并明确事件流扩展不能作为旧 host 兼容认证。当前 schema 后续已升至 v9，见 §45。
- 再修正 S2 表格中“clean 门禁”旧说法，以及 V3 总览中将其余首屏差异一概回退 JSON 的陈述；现在描述与同快照 identity unverified 只读分页、matched dirty cached continuation 及首屏身份页不可用时的有界 legacy fallback 一致。
- 本轮为文档同步；验证 `git diff --check`。未运行测试或读取/修改真实 profile。

## 39. 标题游标、identity 容量与项目分组合约复核（2026-09-26）

- `visibleStructureRevisionTriggers` 与无 revision 时的 `visibleSnapshotID` digest 都只覆盖 ID、relative path、workspace、position、state；标题变化不会改变 keyset snapshot。标题仍由当前页返回，identity rows 的 title-only drift 单独标记，不要求重启分页。
- Tauri JSON workbench catalog 的 50 条限制仅约束兼容目录与 legacy fallback；identity page 每页最多 200 条，完整 shadow/identity snapshot 上限为 10,000 条。前端侧栏对 legacy 明示 50 条及不能续页，对 identity page 则逐页追加并按 session ID 去重；项目树从全部已加载 `tabs` 分组，并先合并已保存的空项目文件夹。这里仍保留 10,000 条全量审计上限，不宣称无限目录。
- 源码复核证据：`internal/sessionidentity/store.go` 的 revision trigger/hash 与分页常量、`desktop/tauri/src/workbench_catalog.rs` 的 JSON 上限、`TauriChatWorkspace.tsx` 的页追加与项目组 memo、`workbenchSessions.ts` 的项目分组。未运行测试或访问真实 profile；旧 Wails writer 和旧 host 二进制兼容仍未认证。
- 本轮只更新核对记录；`git diff --check` 通过。

## 40. identity snapshot 上限拒绝不可续页首屏（2026-09-26）

- 发现 identity directory 首屏可能报告超过 10,000 的 total、却仍返回带游标的前 200 条；后续请求会被同一上限拒绝，用户反复重试也只能看到首屏。Go `ListVisible` 现在在签发任何页/游标前拒绝超限目录；Rust host 对 bridge 返回的 `total` 再做一次 10,000 上限校验，超限响应进入已有的安全 fallback 流程，而不会展示不可续页的身份首屏。
- revision 快路径仅在 cursor 带有效 snapshot ID 和 total 时复用 total；旧式/缺少 snapshot ID 的 cursor 重新 COUNT，避免信任未绑定快照的旧 total。有效续页仍由当前结构 revision 与 snapshot ID 检查变更。
- 本轮非测试验证：`GOCACHE=/private/tmp/reasonix-tauri-go-build-cache go build ./...`、Tauri `cargo fmt --check && cargo check --locked`、前端 `npx tsc --noEmit -p tsconfig.json`、`git diff --check`。未运行测试或访问真实 profile；旧 Wails writer 停写和旧 host 发布二进制兼容仍未认证。

## 41. legacy 导入与 workbench 同步计数语义复核（2026-09-26）

- 重新追踪 Rust 的 legacy import 命令、Go `/import-catalog` 和 identity Store：导入对缺失 transcript 采用 `preserveMissing=true`，在一个事务内登记为 `missing`；既有 identity 经 ID/path 校验后保留。成功的 `accepted` 因而是“请求条目全部处理”，不是新插入行数。
- import 成功后 Rust 才调用顺序同步。Go `Synced` 对请求中 ID 与 identity transcript path 匹配的每条记录计数；直接同步遇到匹配 tombstone 仍计数，但不会放回可见顺序。legacy import 对 ID/path 匹配的 tombstone 现在无副作用地保留 terminal identity 并继续处理其他条目；路径冲突显式失败，不返回虚高 `accepted`。合法缺失文件会登记为 `missing` 并可完整同步。直接同步缺失 identity 时 Go 不计该行，随后 host 的 `synced == sessions.len()` 检查会明确报告不完整。Review §8 已记录当前合同。
- 本轮只更新核对文档。非测试验证 `git diff --check`；未运行测试或访问真实 profile。旧 Wails writer 停写与旧 host 发布二进制兼容仍未认证。

## 42. 标题游标测试与诊断文案复核（2026-09-26，历史记录）

- 当前 Go snapshot ID 和 Rust 页/审计投影匹配都只比较结构字段；title-only 更新可随当前页返回，不使已签发游标失效。
- 当时源码已将该测试改为 `title_change_between_shadow_and_page_preserves_structural_snapshot_id`，首屏和续页都断言 `identity` 来源并检查更新后的标题；首屏 `expect` 文案仍遗留旧的回退描述。这个历史发现后来在 §53 修正：诊断文字改为结构分页语义。当前源码中的两个 `expect` 均与成功断言一致。
- 本节只记录当时发现，不代表当前源码仍有该问题；§53 记录修正，未因此在本轮运行 Rust 测试或访问真实 profile。

## 43. 遗留 catalog 不再被已完成删除的 tombstone 阻断（2026-09-26）

- 启动和“重新检查会话目录”都会重试 legacy catalog import。若进程在 identity 已提交 `deleted`、但 host JSON catalog 尚未忘记 ID 的窗口退出，原 `ImportLegacyCatalog` 会因该 tombstone 拒绝整批导入，尽管目录页能单独显示 identity snapshot。
- 对 ID/path 已匹配的既有 identity，legacy import 的合同本来就是 `preserveExisting`。现在该分支在 terminal-state conflict 前短路：`deleting/deleted` 状态保持不变，旧行视为已处理，其他 legacy 项继续导入；ID/path 不匹配仍 fail closed。这样不会复活会话，也不会让一次已完成删除长期产生整批导入失败。
- 非测试验证：`GOCACHE=/private/tmp/reasonix-tauri-go-build-cache go build ./...`、`git diff --check`。未运行测试或访问真实 profile；旧 Wails writer 停写与旧 host 发布二进制兼容仍未认证。

## 44. identity_unverified 列表不再阻断独立删除恢复入口（2026-09-26）

- pending-delete 区的数据来自独立、只读的身份恢复端点；它与普通目录的 shadow 信任状态不同。前端通用 `deleteSession` 门禁此前在 `sessionPageSource === "identity_unverified"` 时拒绝所有删除，包括已由 pending-delete page 明确标为 `deletionInterrupted` 的行，使恢复入口可见但不可执行。
- 现在只对普通未核验目录行保留该门禁；pending-delete 恢复行可继续走显式 DELETE 重试。若另一个 ID 的当前会话正在生成/暂停，这条恢复路径也不切换 controller，故可绕过仅约束会话切换的门禁；若恢复 ID 本身就是当前会话，或另一个 UI 操作正在进行，仍按原门禁阻止。恢复行不会打开会话，重试的身份 ID 与路径检查及删除 fencing 由 bridge/identity Store 执行。
- 非测试验证：前端 `npx tsc --noEmit -p tsconfig.json`、`git diff --check`。未运行测试或访问真实 profile；旧 Wails writer 停写及旧 host 发布二进制兼容仍未认证。

## 45. S2 review 与当前组合工作区复核（2026-09-26）

- 当前身份库 `schemaVersion=9`。v4–v6 包含 S2 相对路径、标题意图及目录 generation/revision；v7 添加 SQLite 会话事件表，v8 增加 checkpoint 水位，v9 增加事件来源验证标记。事件存储是独立的 managed Preview capability，不据此改变 S2 的兼容认证结论。
- 当时工作树包含会话事件存储和 scan-import 等并行改动；Go build、Tauri `cargo fmt --check && cargo check --locked`、前端 `npx tsc --noEmit -p tsconfig.json` 与 `git diff --check` 均通过。本轮未运行测试，也未读取或修改真实 profile。
- 旧 Wails writer 停写、旧 Tauri host 发布二进制兼容及真实 profile 演练仍未认证。源码版本提高、隔离构建与类型检查都不能替代这些门禁。

## 46. 恢复 staging 核对实际目标文件（2026-09-26）

- `copyVerifiedSnapshotFile` 从复制流计算摘要并确认源在读取期间保持稳定，但它本身不重新读取目标。`CreateOfflineSnapshot` 会在发布 manifest 前执行完整 `VerifyOfflineSnapshot`；`StageOfflineSnapshot` 现在也逐个重新读取 staging 目标并比对 manifest 大小/SHA-256，同时检查目标文件打开前后仍是同一普通文件，才返回 staging 成功。
- 这补足恢复副本的落盘核验，不移除快照创建时的源文件复读；旧 Wails writer 未确认停写前仍保留该一致性检查。本轮非测试验证：`gofmt`、`GOCACHE=/private/tmp/reasonix-tauri-go-build-cache go build ./...`、`git diff --check` 通过；未运行测试或访问真实 profile。

## 47. Preview bridge turn 完成后刷新 transcript checkpoint（2026-09-26）

- 当前 bridge runtime 在 `SubmitHTTP` 返回后按 controller 的 `ClassifySubmitRoute` 决定是否标记 `snapshotPending`：会记录会启动模型回合的 `/mcp__`、自定义斜杠命令和普通输入；管理命令、空输入、被 HTTP 拒绝的 `!` shell 输入及 memory quick-add/remember 不会被误记。取消、审批、问答和 prompt 重放也会在 controller 调用返回后标记待保存。runtime monitor 每 20ms 检查状态，只有不在运行且没有 pending prompt 时才尝试消费标志；消费后再读一次状态，若期间有新 turn/prompt 则恢复标志。
- 若 `SnapshotActivity` 失败，待保存标志会恢复，并设置一秒退避后重试，避免失败后漏存及每个 monitor tick 重复报错。`State()` 使用相同逻辑并返回复读后的状态；关闭时先停 monitor，再执行 shutdown snapshot。这是完成后的轮询/状态查询刷新机制，不是 `TurnDone` sink 同步回调；durable inbox 的快照仍由 controller 自己的确认路径处理。
- 项目文件夹 bridge 回归样例现明确验证绝对路径尾部空格属于路径内容、不得被裁掉；标题仍按约定 trim。非测试验证：`gofmt`、`GOCACHE=/private/tmp/reasonix-tauri-go-build-cache go build ./...`、Tauri `cargo fmt --check && cargo check --locked`、前端 `npx tsc --noEmit -p tsconfig.json`、`git diff --check` 通过；未运行测试或访问真实 profile。

## 48. 新建缺失 transcript 时减少同目录 peer filesystem 检查（2026-09-26）

- `ensureTranscriptPathAvailable` 发现 candidate 不存在且父目录在 peer 枚举前后都解析到调用方已验证路径时，同 lexical parent 的 peer 仍各做一次 `Lstat`；absent/regular 项通过完整路径/大小写别名比较后不再执行 peer symlink/path resolve 和后续 file stat。symlink 与非普通项仍走完整校验。各 peer 路径仍保留在最终候选 identity recheck 集合中，候选若在扫描期间出现仍会与所有 peer 比较 file identity；期间 parent symlink 改变则拒绝操作。
- 候选已存在、父路径变化或 peer 位于其他目录时，原完整检查保留；身份行 SQL/字符串枚举仍为 O(n)，此项只减少常见 flat session directory 新预留的文件系统调用。非测试验证：`gofmt`、`GOCACHE=/private/tmp/reasonix-tauri-go-build-cache go build ./...`、`git diff --check` 通过；未运行测试或访问真实 profile。

## 49. WAL checksum 验证后复用主库 schema header（2026-09-26）

- `walLeavesSchemaPageUntouched` 现在验证 WAL header checksum、magic 对应的 checksum 字节序、活跃帧滚动 checksum 与 salt，并检查扫描期间文件 identity/size/mtime 稳定。只有完整且无 page-1 frame 的 WAL 才跳过整库副本检查；page-1、salt mismatch（可能是未截断 WAL 的旧帧尾）、未知或不完整布局、文件变化仍保留隔离副本 schema 检查。当前 salt 下损坏的 header/frame checksum 会拒绝打开，避免 SQLite 回放损坏帧或改写原 DB/WAL。
- 工作树新增了损坏 WAL 拒绝测试，但本轮没有运行测试。官方 SQLite 规范确认 magic `0x377f0682` 的 checksum 输入按小端解释，`0x377f0683` 按大端解释，而保存的 checksum 字段始终为大端。旧 Wails writer 并发仍未认证。
- 非测试验证：Go `go build ./...`、Tauri `cargo fmt --check && cargo check --locked`、前端 `npx tsc --noEmit -p tsconfig.json`、`git diff --check` 通过；未访问真实 profile。

## 50. 旧会话 JSON catalog 损坏时保留身份页只读可见性（2026-09-26）

- Rust host 不再因 `WorkbenchCatalog::list()` 失败而提前中断会话目录加载；改用空 legacy projection 尝试 identity pagination，并把可读身份页降为 `identity_unverified`、附带 catalog warning。未核验页继续关闭打开、删除、新建操作。若侧栏桥接离线，已有的匹配 identity snapshot 仍可只读续页；identity 首屏不可用则保留空兼容页及 warning，不会伪造旧 catalog 内容。
- 前端新增兼容 catalog warning 字段，在侧栏提示需修复旧目录并重启 Preview；page unavailable 时清除过期 warning。
- 非测试验证：Go `go build ./...`、Tauri `cargo fmt --check && cargo check --locked`、前端 `npx tsc --noEmit -p tsconfig.json`、`git diff --check` 均通过；未运行测试或访问真实 profile。

## 51. 刷新时续读已加载的未核验身份页（2026-09-26）

- `refreshCatalogAudit` 新增“可分页身份来源”判断，将 `identity_unverified` 纳入已加载页恢复流程；未核验来源可续读以维持会话目录完整性，但仍不进入安全身份来源分支。刷新续页失败或来源变化时仍重新读取首屏。
- 非测试验证：Go `go build ./...`、Tauri `cargo fmt --check && cargo check --locked`、前端 `npx tsc --noEmit -p tsconfig.json`、`git diff --check` 通过；未运行测试或访问真实 profile。

## 52. 离线缓存身份页的 cursor 二分定位（2026-09-26）

- 缓存目录按 `(position,id)` 有序，离线 `SessionShadowSnapshotCache::page` 现用二分查找 cursor 起点，避免每个缓存续页从快照首行线性扫描。缓存查找和附带的 shadow 元数据都要求 cursor 的 snapshot ID 与 total 匹配；页面结构校验仍只检查当前页，并保留身份只读状态。
- 非测试验证：Go `go build ./...`、Tauri `cargo fmt --check && cargo check --locked`、前端 `npx tsc --noEmit -p tsconfig.json`、`git diff --check` 均通过；未运行测试或访问真实 profile。

## 53. 首次目录读取完成前与只读来源下关闭操作入口（2026-09-26）

- Tauri sidebar 的 page source 初始值改为 `unavailable`，避免首次异步 shadow 核验前按 `identity` 放行新建。`unavailable`、`cached`、`identity_unverified` 下普通目录项的打开、重命名、删除和新建操作在 UI 和业务 handler 都拒绝；pending-delete recovery 行仍有独立显式重试路径。
- 修正 pending-delete 行的 UI 门禁：普通目录处于 `cached` 来源时，独立恢复行仍可进入确认和显式重试；页面目录只读状态不再遮住恢复操作。另将标题游标源码测试的过时 `expect` 诊断文字改为结构分页语义，未运行测试。
- 非测试验证：Go `go build ./...`、Tauri `cargo fmt --check && cargo check --locked`、前端 `npx tsc --noEmit -p tsconfig.json`、`git diff --check` 通过；未运行测试或访问真实 profile。

## 54. 复核 legacy fallback 上限与 identity 续页扫描成本（2026-09-26）

- 当前 host 的正常续页 cache hit 先校验旧 catalog 结构和 cursor 的 snapshot ID/total，再获取、核对当前身份页；不重新获取完整物理 inventory。cache miss、目录结构变化或 cursor/page 不匹配会走重新 shadow audit。identity snapshot 超过 10,000 条不签发页或 cursor；首屏 identity 不可用时的 legacy fallback 仍受 50 条上限约束，UI 同时说明无法从 fallback 翻页并提供重新检查入口。Review §39 记录当前边界。
- 非测试验证：`GOCACHE=/private/tmp/reasonix-tauri-go-build-cache go build ./...`、Tauri `cargo check --locked`、前端 `npx tsc --noEmit -p tsconfig.json`、`cargo fmt --check`、`git diff --check` 均通过。未运行测试或访问真实 profile；旧 Wails writer 停写及旧 Tauri host 发布二进制兼容继续未认证。

## 55. 复核 transcript 路径唯一性扫描边界（2026-09-26）

- `ensureTranscriptPathAvailable` 已为缺失候选的同目录 regular/absent peer 跳过完整解析与后续文件 stat，但仍逐条枚举 identity，并保留 `Lstat` 以发现 symlink/non-regular peer，以及候选在扫描期间出现时的 file identity recheck。未发现可在无外部 filesystem identity 失效机制下安全删除这些 peer 检查的方式；详见 Review §40。此为源码边界复核，没有修改唯一性算法。
- 验证 `git diff --check`；未运行测试或访问真实 profile。旧 Wails writer 停写及旧 Tauri host 发布二进制兼容仍未认证。

## 56. 复核离线 review 锁与 snapshot 源复读边界（2026-09-26）

- `ApplyImportReview` 的 profile gate 在函数内覆盖重审及 identity transaction import；`CreateOfflineSnapshot` 复制后重读源摘要，并在发布 manifest 后验证完整副本；staging 重新校验写入目标。profilegate 文档和 snapshot API 合同仍要求调用方独立停掉不遵守新 gate 的旧 writer，源码检查不证明实际 writer 已停止。Review §41 已记录此范围。
- 验证 `git diff --check`；未运行测试、未读取真实 profile。旧 Wails writer 停写和旧 Tauri host 发布二进制兼容继续未认证。

## 57. 区分未加载项目组与确认空项目组（2026-09-26）

- 项目分组由已加载的全局会话页构成。还有后续 cursor 时，保存项目文件夹若当前已加载页没有可打开会话（含所有已加载行均为 missing），不再被当成完整空组：显示“历史未加载”并禁用组选择，直到用户继续分页；组内新建入口保持可用。分页未结束时，侧栏说明项目组计数仅是已加载数量。没有 cursor 且无可恢复会话时，仍可将该文件夹设为新对话默认工作区。Review §42 与 S2 设计稿 §3.5 记录状态语义。
- 非测试验证：前端 `npx tsc --noEmit -p tsconfig.json`、`git diff --check` 通过；未运行测试或访问真实 profile。旧 Wails writer 停写及旧 Tauri host 发布二进制兼容仍未认证。

## 58. 对齐 V3 总览的 shadow 差异回退描述（2026-09-26）

- `SESSION_STORAGE_IMPLEMENTATION_V3.md` 原有两个阶段摘要仍称一般差异/盘点错误直接回退 JSON。已同步为当前行为：shadow 可读且 identity page 与同轮 projection 匹配时提供全量 `identity_unverified` 只读分页；shadow 不可读但身份页可用时也尝试只读分页；仅不匹配或首屏身份页不可用时回退有界 JSON。未改变实现或安全门禁。
- 验证 `git diff --check`；未运行测试或访问真实 profile。旧 Wails writer 停写和旧 Tauri host 发布二进制兼容继续未认证。

## 59. 复核待完成删除分页不受旧清单总数上限阻断（2026-09-26）

- 当前 Tauri host 使用按 ID 排序的 `/v1/sessions/deletion-recovery/page`，每次最多 200 条；旧无参数列表的 10,000 条限制只留给兼容旧 host，不约束新分页路由。Rust 校验页大小、递增 ID 和 next cursor；前端页失败保留已有项并显示重查入口。Review §43 记录边界。
- 验证 `git diff --check`；未运行测试或访问真实 profile。旧 Tauri host 发布二进制兼容继续未认证。

## 60. 项目文件夹清单重试恢复磁盘读取（2026-09-26）

- 项目文件夹目录在 Preview 启动时若因损坏、权限或瞬时 I/O 错误无法读取，之前本地 catalog 会一直返回内存中的旧 `load_error`；侧栏“重新检查”不会重新访问磁盘。现在仅对已失败的本地目录读取重试 `read_folders`，有效文件被修复后立即恢复文件夹显示，正常已加载目录仍走内存状态。
- 非测试验证：Tauri `cargo fmt --check`、`cargo check --locked`、前端 `npx tsc --noEmit -p tsconfig.json`、`git diff --check`。未运行测试或访问真实 profile；旧 Wails writer 停写及旧 Tauri host 发布二进制兼容仍未认证。

## 61. Legacy fallback 不再把未知项目组当成空组（2026-09-26）

- `legacy` 首屏没有 continuation cursor，但最多只含兼容目录的 50 条，不能证明无可见会话的保存文件夹在身份目录中也是空的。前端现将 `legacy` 与“仍有后续 cursor”一并视为项目历史未完整；对没有已加载可打开会话的项目组禁用切换，并按来源给出重新检查或加载更多的提示。已有可打开会话仍可选择，组内新建入口保持独立可用。
- 非测试验证：前端 `npx tsc --noEmit -p tsconfig.json`、`git diff --check`。未运行测试或访问真实 profile；旧 Wails writer 停写及旧 Tauri host 发布二进制兼容仍未认证。

## 62. 历史 host 与当前 sidecar 的非测试源码构建（2026-09-26）

- 在临时目录导出历史 Tauri host 源码提交 `50c1b9bc6`，以当前工作树 `cmd/reasonix-desktop-bridge` 构建 sidecar 并复制到历史 host 的资源位置；离线 `cargo build` 成功。候选二进制 SHA-256：`4290bea369739b5cb633203b391079bc105f726f89a27ef10cde79eb6e833238`。
- 没有运行 host 或 Cargo 测试，没有访问真实 profile；该构建不证明已发布旧二进制兼容，也不改变旧 Tauri host 兼容和旧 Wails writer 停写仍未认证的结论。

## 63. 当前工作树 S2 优先项复核（2026-09-26）

- 重新对照 Review §1.1–§1.4 与当前实现：待完成删除由独立 ID keyset 页面暴露并可显式重试；legacy 回退维持 50 条上限且侧栏说明无法从该回退继续翻页；常规续页 cache hit 复用首屏物理盘点、按 snapshot ID/total 和当前页结构字段校验；结构快照忽略 title-only 更新。未发现这四项在当前工作树回归。
- 抽核 §2、§3 仍开放的源码边界：profile gate 仍不声称约束旧 Wails writer；离线 snapshot 源复读仍保留；identity path 唯一性仍枚举全部身份项以核实 symlink/hard-link 别名；旧 host 源码隔离构建不等同于已发布二进制认证。它们仍分别是保守正确性边界或外部未认证门禁，没有用源码推断替代认证。
- 当前非测试验证：`GOCACHE=/private/tmp/reasonix-tauri-go-build-cache go build ./...`、Tauri `cargo fmt --check && cargo check --locked`、前端 `npx tsc --noEmit -p tsconfig.json`、`git diff --check` 通过。未运行测试、未运行 host、未访问真实 profile；旧 Wails writer 停写和旧 Tauri host 发布二进制兼容仍未认证。
