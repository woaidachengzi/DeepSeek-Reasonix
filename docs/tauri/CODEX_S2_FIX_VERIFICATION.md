# Codex S2 修复核对报告

> 对照文档：[CODEX_S2_REVIEW.md](./CODEX_S2_REVIEW.md)（我上一轮的审查报告，19 条问题）
> 核对时间：2026-09-25
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

**已修的 4 条**：

- **1.1 中断删除缺少独立发现入口** —— 已增加仅含 ID 和标题的恢复清单；普通目录页失败时仍可显示，用户确认后续做删除。
- **1.2 标题变化导致分页失效** —— 已修，且测试同步更新并新增反向用例。
- **2.6 `ApplyImportReview` 的 TOCTOU 只靠注释约束** —— 已修，改为函数内强制获取 profile 锁。
- **3.1 两个 catalog 端点对坏 JSON 返回空响应** —— 阶段提交后的跟进修复已在 `session_catalog_import.go` 与 `session_catalog_sync.go` 返回协议化 `invalid_request`，`TestSessionCatalogEndpointsReturnProtocolErrorForMalformedJSON` 覆盖两个端点。

**另有 1 处是原审查结论有误**，见 §3：1.3 中"用户得不到任何提示"不成立，
UI 实际上已有明确提示与重试按钮。原报告的 1.1 与 1.4 也需按现有改动更新：前者现在有独立的显式
恢复入口，启动期不会自动删除；后者改用单次有界快照接口，仍会按列表页重复做完整影子审计。
在阶段提交后，2.5 另做了一次安全的读放大优化：复制时流式计算目标摘要，保持源文件复读比对与最终
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
| **1.3** | **legacy 回退不提供分页** | `main.rs:498-506`、`:510-518` 两处 legacy 分支仍是 `next_cursor: None`，调用 `page_entries_from_legacy`。上限仍来自 `workbench_catalog.rs:12`（`MAX_SESSIONS = 50`）。**注意**：提示与重试按钮已存在（见 §3），因此本条现在只是"无法翻到 50 条之后"，不再是"无任何说明" |
| **1.4** | **每页仍重复全量影子审计（已部分缓解）** | `main.rs` 的列表页仍调用 `compare_session_catalog_with_directory`，并拉取完整物理 inventory；工作区已有 `GET /v1/sessions/snapshot` 与 `session_directory_snapshot_full_v1`，把原先按 200 条循环请求改成一次有界快照（上限 10,000）。同一分页请求现在也只读取一次 host JSON catalog，影子比较与回退展示共用该快照。每页的完整物理盘点仍存在，不能称已完全解决。 |

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
| 3.2 | 死代码 | `bridge_session_directory_page`（`main.rs:575`）仍已注册但前端无调用点 |
| 3.3 | 重复计算 | `store.go:407` 仍丢弃 `relativeTranscriptPath` 的返回值 |
| 3.4 | 平台策略无提示 | `path.go:33-38` `sameCaseInsensitivePath` 未变，错误信息未点明平台策略 |
| 3.5 | 快照包含可重建缓存 | `offline_snapshot.go:82-117` 仍整树拷贝 profile（含 `cache/`） |
| 3.6 | `previewRoot` 命名过时 | `store.go:521`、`:583` 仍用 `previewRoot`，实际传的是 session dir |
| 3.7 | 空数组提前返回语义含糊（未证实） | `Accepted` 是非 `omitempty` 的 `int` 字段，JSON 会明确输出 `"accepted":0`；空目录成功导入 0 条，现有响应语义明确且可解析。 |

---

## 3. 我上一轮审查的**错误**（需更正）

### 3.1 [编号 1.3] "用户得不到任何提示" —— 不成立 ❌

我在报告里写"用户得不到任何'数据其实更多'的提示"。实际代码**已经有明确提示**：

`desktop/frontend/src/tauri/TauriChatWorkspace.tsx:1635-1640`

```tsx
{sessionPageSource === "legacy" && <>
  <p className="tauri-sidebar__page-note" role="status">
    当前使用本地兼容目录，最多 50 条；持久会话目录未通过校验，列表可能不完整。
  </p>
  <button ... onClick={() => void retryWorkbenchSessionDirectory()}>
    重新检查会话目录
  </button>
</>}
```

它明确写出了"最多 50 条"与"列表可能不完整"，并提供手动重试（`retryWorkbenchSessionDirectory`，
`:846-861`）。前端也维护了 `sessionPageSource: "identity" | "legacy" | "unavailable"` 三态
（`:278`），并在多处据此切换行为（`:818`、`:871`、`:1049`、`:1296`）。

**更正后的结论**：1.3 的真实问题只剩"legacy 分支无法翻到 50 条之后"，
而不是"静默降级"。该容量限制仍影响目标，但其用户提示与重试路径已存在。

（这条误判的原因：我只读了 Rust 侧 `main.rs` 的分支逻辑，没有追前端如何消费 `source` 字段。
教训是"未覆盖部分"里我自己标注过的 `TauriChatWorkspace.tsx` 分页状态机，
恰恰就是这条结论的依据。）

---

## 4. 当前状态与建议

**仍然建议优先处理（按影响排序）**：

1. **1.4 每页全量重扫** —— 它会随会话数放大；在旧 Wails writer 停写确认前保留物理 inventory 与 shadow 门禁，不应靠缓存跳过校验。
2. **2.1 / 2.2 的热路径开销** —— 需在不削弱 transcript 唯一性与 WAL/schema 校验的条件下测量、优化。
3. **1.3 的 legacy 分页** —— 现在有提示，因此不紧急；fallback 自身仍受 JSON catalog 的 50 条上限约束。

1.1 的独立恢复入口已补齐；启动期不无提示地自动删除是当前明确的产品行为。2.3、3.7 的原判定未证实，不建议按原建议改动。

**可以推迟**：2.4（潜在，未接线）、2.5 剩余读取、2.7、2.8 与其余低严重度项。

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
