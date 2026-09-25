# Codex S2 修复核对报告

> 对照文档：[CODEX_S2_REVIEW.md](./CODEX_S2_REVIEW.md)（我上一轮的审查报告，19 条问题）
> 核对时间：2026-09-25
> 核对方式：先按原报告做只读核对，再于 2026-09-25 复核现有工作区改动；本次为校正过时结论，不改产品代码。
> 状态：`git log` 显示 HEAD 未变（仍 `c8a2a8deb`），修复都在未提交工作区中；
> 当前 diff 规模 40 文件 / +4,254 −341（审查时为 39 文件 / +3,415 −283）。
> 回归验证：`go test ./internal/sessionidentity/... ./cmd/reasonix-desktop-bridge/` **全部通过**。

## 0. 结论摘要

| 严重度 | 总数 | 已修 | 部分缓解 | 仍存在 |
| --- | --- | --- | --- |
| 高（功能） | 4 | **1** | **2** | 1 |
| 中（性能/一致性） | 8 | **1** | 0 | 7 |
| 低（细节） | 7 | 0 | 0 | 7 |
| **合计** | **19** | **2** | **2** | **15** |

**已修的 2 条**：

- **1.2 标题变化导致分页失效** —— 已修，且测试同步更新并新增反向用例。
- **2.6 `ApplyImportReview` 的 TOCTOU 只靠注释约束** —— 已修，改为函数内强制获取 profile 锁。

**另有 1 处是原审查结论有误**，见 §3：1.3 中"用户得不到任何提示"不成立，
UI 实际上已有明确提示与重试按钮。原报告的 1.1 与 1.4 也需按现有改动更新：前者已有显式
legacy 重试路径但没有启动期自动续做；后者改用单次有界快照接口，仍会按列表页重复做完整影子审计。

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

## 2. 未完全解决或仍存在（17 条：2 条部分缓解，15 条未修）

### 2.1 高严重度（1 条仍存在，2 条部分缓解）

| 编号 | 问题 | 核对证据 |
| --- | --- | --- |
| **1.1** | **中断删除没有启动期自动续做** | `session_delete_recovery.go` 中 `retryInterruptedSessionDelete` 仍仅由删除端点路径调用，没有启动期 reconciler；身份分页仍排除 `deleting` 行。不过当前 legacy 回退保留 JSON catalog 条目，用户可从兼容列表显式再次删除，Rust 回归 `interrupted_delete_remains_visible_for_explicit_legacy_retry` 覆盖可见性。因此“完全无法重试”不成立；缺口是自动续做和非 legacy fallback 下的直接可见性。 |
| **1.3** | **legacy 回退不提供分页** | `main.rs:498-506`、`:510-518` 两处 legacy 分支仍是 `next_cursor: None`，调用 `page_entries_from_legacy`。上限仍来自 `workbench_catalog.rs:12`（`MAX_SESSIONS = 50`）。**注意**：提示与重试按钮已存在（见 §3），因此本条现在只是"无法翻到 50 条之后"，不再是"无任何说明" |
| **1.4** | **每页仍重复全量影子审计（已部分缓解）** | `main.rs` 的列表页仍调用 `compare_session_catalog_with_directory`，并拉取完整物理 inventory；但工作区已有 `GET /v1/sessions/snapshot` 与 `session_directory_snapshot_full_v1`，把原先按 200 条循环请求改成一次有界快照（上限 10,000）。因此每页的重复全量工作仍存在，但不再是旧报告描述的重复分页请求 + 额外完整目录清单；不能称已完全解决。 |

### 2.2 中严重度（7 条仍存在）

| 编号 | 问题 | 核对证据 |
| --- | --- | --- |
| **2.1** | 每次变更 O(n) 次 syscall | `store.go:418` `ensureTranscriptPathAvailable` 仍 `SELECT id, relative_path FROM sessions WHERE id<>?`（全表），逐行 `resolveTranscriptPath`（内部 `EvalSymlinks`）＋ `os.Stat` |
| **2.2** | 打开库可能整库拷贝 | `path.go:147` 仍调用 `inspectWALSchemaOnCopy`（`:153`），WAL 非空即复制 `.sqlite`+`-wal`+`-journal`。`Open` 仍在热路径（`core_runtime.go:108`、`:135`） |
| **2.3** | `Accepted`/`Synced` 计数与实际写入不符 | `session_catalog_import.go:86` 仍 `Accepted: len(candidates)`；`catalog.go:127-145` 的 `listedPosition` 仍只统计存在条目，而 host `main.rs:651-657` 要求等于 legacy 条数 |
| **2.4** | 路径归一化三层不一致 | `store.go:819`、`:831` 等处仍是大小写敏感的 `workspace_root=?`；Rust 组键小写化（`workbench_projects.rs:158-173`）、前端同样（`workbenchSessions.ts:15-25`）。目前仍未接工作区过滤（`tauriBridge.ts:270` 只传 limit/cursor），故仍是**潜在**问题 |
| **2.5** | 离线快照多次哈希 | 当前 `copyVerifiedSnapshotFile` 仍在拷贝后分别哈希目标和源，`CreateOfflineSnapshot` 最后仍执行完整 `VerifyOfflineSnapshot`；复制时尚未采用流式哈希。 |
| **2.7** | `SyncWorkbenchOrder` 50 条硬限制 | `catalog.go:32` 仍 `len(entries) > 50` 直接报错（`:159` 是 `ImportWorkbenchCatalog` 的同款限制） |
| **2.8** | 恢复路径用读写 `Open` | `session_delete_recovery.go:48` 仍用 `sessionidentity.Open`（会迁移 schema），而列表/清单用 `OpenReadOnly`（`session_list.go:90`） |

### 2.3 低严重度（7 条全部未修）

| 编号 | 问题 | 核对证据 |
| --- | --- | --- |
| 3.1 | 解析失败返回空 400 | `session_catalog_import.go:31-33`、`session_catalog_sync.go:25-27` 仍直接 `return` |
| 3.2 | 死代码 | `bridge_session_directory_page`（`main.rs:575`）仍已注册但前端无调用点 |
| 3.3 | 重复计算 | `store.go:407` 仍丢弃 `relativeTranscriptPath` 的返回值 |
| 3.4 | 平台策略无提示 | `path.go:33-38` `sameCaseInsensitivePath` 未变，错误信息未点明平台策略 |
| 3.5 | 快照包含可重建缓存 | `offline_snapshot.go:82-117` 仍整树拷贝 profile（含 `cache/`） |
| 3.6 | `previewRoot` 命名过时 | `store.go:521`、`:583` 仍用 `previewRoot`，实际传的是 session dir |
| 3.7 | 空数组提前返回语义含糊 | `session_catalog_import.go:38-41` 仍返回 `Accepted` 默认 0 |

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

1. **1.1 中断删除的续做入口** —— 唯一的"数据可见性"缺口，且与设计稿明确要求不符。
2. **1.4 每页全量重扫** —— 它会随会话数放大，且与本项"解除 50 条上限"的目标正面冲突：
   真到了几百上千条，翻页会不可用。
3. **2.1 / 2.2 的热路径开销** —— 与 1.4 同源（都在"每次操作/每页"上），建议合并处理。
4. **1.3 的 legacy 分页** —— 现在有提示，因此不紧急；但"最多 50 条"仍是能力上限。
5. **2.3 计数语义** —— 会把"合法跳过"误判为"同步不完整"，属于正确性问题，建议随 1.3 一起改。

**可以推迟**：2.4（潜在，未接线）、2.5、2.7、2.8 与全部低严重度项。

**回归状态**：原核对记录称 `go test ./internal/sessionidentity/... ./cmd/reasonix-desktop-bridge/`
通过。本次复核仅做静态核验，Rust 与前端测试未在本次复核中重跑；提交前应重跑本阶段相关套件。
