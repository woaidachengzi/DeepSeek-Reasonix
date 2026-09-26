# 第 5 项设计稿：持久会话目录与完整项目树（存储 S2）

> 状态：**Preview 迁移阶段（2026-09）**。当前工作树 identity SQLite 已到 schema v9；S2 的相对 profile-root 路径、标题意图、生命周期状态与 generation/revision 分页分别在 v4–v6 引入，v7–v9 事件表扩展属于独立的 managed Preview RFC。身份目录分页已接入；首次会话 reservation、恢复前状态检查及
> 残留 sidecar 防误复用、身份库 keyset 分页、bridge 只读列表接口、删除状态的存储 API 及旧 JSON catalog 幂等导入已实现；
> bridge 删除清理已接入（包括 `deleting` 状态下的重试、重启后续删，以及清理已完成后通过 `deleted` tombstone 幂等清掉陈旧 host catalog 行）；Rust host 影子比对门禁与身份库分页侧栏已实现。clean/可解释差异按身份页分页；可读但有结构或物理差异时，在同一 snapshot 下以 `identity_unverified` 只读分页显示身份行，并将旧目录独有项隔离展示；shadow 不可用时也尝试只读分页身份目录。只有身份页首屏不可用时才回退最多 50 条 JSON，续页失败则重新读取首屏、不拼接来源。分页 continuation 绑定可见目录快照，变化时返回 `resync_required`；子进程强制终止于删除清理中途后的重启续删也有隔离回归。
> 侧栏可通过加载更多超过旧 JSON 的 50 条上限；临时测试 profile 的快照 staging、身份路径重定位与旧 catalog 重放已有自动化演练；全 profile 权威切换、真实 profile 停写确认及其跨资源恢复/兼容演练仍未完成。
> 本文继续作为其余工作契约、验收条件、测试矩阵与回退路径。
>
> 背景与边界：[SESSION_STORAGE_IMPLEMENTATION_V3.md](./SESSION_STORAGE_IMPLEMENTATION_V3.md) §5 S2。
> 本稿不改动其中的边界，只把它落到本仓库当前的代码事实上。

## 1. 本项要解决的问题

| # | 问题 | 现状证据 |
| --- | --- | --- |
| 1 | **缺文件的会话会被静默重开成同 ID 的空会话** | 已由身份状态优先检查、missing 状态与 reservation 修复；见 §3.3。 |
| 2 | **最近列表硬上限 50，且会静默丢弃** | SQLite keyset 分页侧栏已可继续加载超出 JSON 最近列表的旧会话；JSON 仍保留最多 50 条作为兼容回退源。 |
| 3 | **列表权威在 host 的 JSON 文件，不在身份库** | Preview 首屏影子比对通过时按页读取身份库；有可读差异时身份页只读显示并明确标记未核验，shadow 不可用时也尝试身份只读分页。身份页首屏不可用仍回退 JSON；这仍是迁移期门禁，不等同唯一权威切换。 |
| 4 | **Tauri 曾只从最近会话生成工作区文件夹** | 已增加显式导入旧版文件夹清单，并合并身份目录会话；历史空文件夹可见，文件夹下会话仍按分页加载。 |

问题 1 是数据安全，问题 2 是功能上限，两者都必须在切换权威之前解决。

## 2. 可直接复用的既有能力（不要重写）

- 删除与产物清理：`control.RemoveSessionArtifacts`（`internal/control/controller.go:3241`），
  已覆盖 transcript、13 类 sidecar、guardian、inbox、checkpoint、子 agent、cleanup 标记。
  第 5 项**不得**新增第二套删除实现。
- 身份库：当前工作树 `internal/sessionidentity` 为 schema v9；S2 的 v4–v6 引入 `relative_path UNIQUE`、生命周期 `state`、
  `title_source`/`title_revision`（CAS，`SetTitle`）与目录 generation/revision。v7–v9 事件表扩展另见 `SESSION_SQLITE_EVENT_STORE_RFC.md`。`Import`、`OpenReadOnly`、`Inventory` 仍是目录迁移能力。
- 只读盘点：`sessionidentity.Inventory` 与 `GET /v1/sessions/inventory`（第 3 项已交付），
  它已经是"缺文件"的权威观测点。
- 标题回填：host 已有 `titleFromFirstUser` 与首轮标题回填；身份库侧用 `SetTitle`
  的 `TitleFirstMessage` 语义对应，不要另立规则。
- 会话切换：`RuntimeManager.Switch`（bridge 只持有一个 Controller，切换=durable shutdown + 重建）。

## 3. 设计

### 3.1 权威边界（与 V3 §5 S2 一致）

```text
身份库（Preview state home/session-state-v1.sqlite）
  → 会话 ID、路径、标题、工作区、顺序、生命周期状态
JSONL + sidecar（不变）
  → 执行与对话真相源；身份库只记路径，不改内容
workbench-sessions.json
  → 迁移期回退输入；切换后**只读**，不再双向写
cache/session-catalog、history FTS5
  → 保持可重建
```

唯一写入者：Go sidecar。host 只经 bridge 读。

### 3.2 生命周期与状态（把 V3 §3.2 落到本仓库）

V3 要求 `reserved/ready/missing/deleting/deleted`。schema v4 保留 v3 生命周期状态并加入相对 `relative_path`；`BeginDelete` / `FinishDelete` 已接入 bridge artifact sweep。清理失败后写入被隔离，重启后的同 ID 删除请求只可续删 `deleting` 记录：

| 状态 | 含义 | 允许的迁移 |
| --- | --- | --- |
| `reserved` | 新 ID 已登记，尚无 transcript | → `ready`（首次落盘）、→ `deleting` → `deleted` |
| `ready` | 已有 transcript | → `missing`（文件消失）、→ `deleting` |
| `missing` | 曾落盘，文件已不在 | → `ready`（文件回来，需证据）、→ `deleting`（用户明确放弃） |
| `deleting` | 正在清理 | → `deleted`（清理成功） |
| `deleted` | 已删除，保留 tombstone | 终态，**ID 不得重用** |

### 3.3 缺文件不得被重开成空会话（本项最关键的修复）

`resumeBridgeSession` 的判定顺序改为（**身份状态优先于文件是否存在**）：

```text
1. 身份库有该 ID 的 deleting/deleted 记录
                    → 返回冲突错误，绝不 Resume、绝不新建；host 显示墓碑原因
2. 身份库有该 ID 的 missing 记录
                    → 返回冲突错误（文件已不在），host 显示"该会话的文件已不在"
3. 身份库有该 ID 的 ready 记录，且文件存在
                    → Resume（现状不变）
4. 身份库有该 ID 的 ready 记录，但文件不存在
                    → 返回冲突错误，绝不新建；host 显示"该会话的文件已不在"
5. 身份库有该 ID 的 reserved 记录
                    → 文件不存在时 SetFreshSessionPath；若已有 transcript，校验后转 ready 并 Resume
6. 无身份库记录、transcript 存在
                    → 校验并 Resume，再登记为 ready（兼容升级前创建的 Tauri 会话）
7. 无身份库记录、transcript 不存在
                    → 有 `.jsonl.meta` / `.inbox` 残余则冲突；否则先登记 reserved，再 SetFreshSessionPath
```

**为什么 1–2 必须先于“文件存在”**：墓碑/删除中的 ID 若仍残留 transcript 文件，
按“文件存在就 Resume”会绕过墓碑状态，让已删除会话复活。身份库是生命周期权威；
文件只是内容载体。

第 6 条要有测试固定：第 3 项的 `TestDeletingATranscriptLeavesSidecarEvidence`
已证明删除后 `.jsonl.meta` 与 `.inbox` 会留存，这就是可用的证据。

**host 侧要求**：打开失败时显示明确原因与三个选项——"选择其他会话"、"在 Finder 中查看"
（不实现也行）、"删除该记录"（走 `deleting`）。**不得**提供"以空会话继续"这类一键补救，
那正是要消灭的行为。

### 3.4 解除 50 条上限

上限的根因是 host 的 JSON 文件被当作权威列表。切换后 host 不再持有权威列表：

- 身份库 `ListVisible(limit, cursor)`：已按 **`(position, id)`** 排序分页；
  `deleted` 不返回，`missing` 返回并带标记，`deleting` 不返回。
  `deleting` identity 另由 `GET /v1/sessions/deletion-recovery/page` 按不可变 ID 游标返回每页最多 200 条、仅含 ID/标题的恢复清单；无参数 `GET /v1/sessions/deletion-recovery` 保留给旧 host，最多 10,000 条。Tauri 侧栏将其独立展示并可续页，用户二次确认后才重试 DELETE，不在启动时无提示地自动删除。
  游标必须是 `(position, id)` 复合键——只按 `position` 会在同 `position` 的
  并列会话上漏项或重复（SQLite 中 `position` 未强制唯一）。
- bridge 只读端点已实现：

```text
GET /v1/sessions?limit=<n>&cursorPosition=<position>&cursorId=<id>&workspaceRoot=<path>
→ { protocolVersion, sessions: [...], nextCursor?{position,id,snapshotId}, total, snapshotId }
```

  `snapshotId` 是 SQLite 同一只读事务中对完整可见目录投影计算的 SHA-256。`nextCursor`
  仅在还有下一页时出现；每页末项的 `(position, id)` 即下一页起点
  （严格 `>` 比较：`position > cursorPosition || (position == cursorPosition && id > cursorId)`），
  同时携带首屏的 `snapshotId`。下一页的目录指纹若变化（即使总数没变），bridge 返回 HTTP 409
  `resync_required`；host 对 shadow scan 与实际读取页也交叉核对指纹，不拼接不同快照。
  每项至少含 `id / title / titleSource / workspaceRoot / state / missing / position / updatedAtMs`，
  **不含** transcript 内容、不含凭据类字段。
- Host 做 shadow 比对时使用 `GET /v1/sessions/snapshot?workspaceRoot=<path>` 一次性读取最多 10,000 条；该端点在一个 SQLite 只读事务中返回完整目录与同一结构快照 ID，避免为组装完整目录按 200 条逐页请求并重复 count/hash 全表。它由 `session_directory_snapshot_full_v1` capability 保护；完整快照完成影子比对后，host 仍会单独请求所展示页并核对 snapshot ID。
- 迁移入口 `POST /v1/sessions/import-catalog` 与 host 启动调用已接入：最多接收旧 JSON 的
  50 项；仅插入身份库中不存在的记录，保留旧目录顺序、标题与工作区；缺失 transcript 作为
  `missing` 保留。重复调用不会覆盖身份库中较新的元数据，JSON 文件仍不修改。
- host 已全量读取分页目录并提供只读影子报告，比较旧 JSON 与身份库的 ID 覆盖、标题、工作区、
  顺序及缺失 transcript 数量；扫描不完整或目录在读取期间变化时不会报告为一致。运行状态面板展示汇总，
  报告不输出会话 ID、路径或标题内容。
- 旧 JSON catalog 文件缺失按首次启动处理为空目录；文件存在但不可读或 JSON 格式无法解析时，读取与写入均 fail-closed，不能静默当成空目录后覆盖原文件。
- host 首次只取一页（例如 200 条），滚动到底再取下一页；**不再有丢弃**。
- 迁移：`workbench-sessions.json` 的现有条目一次性导入身份库（沿用第 2、3 项已交付的
  `Import` 与 `ImportWorkbenchCatalog`，路径已同源）。导入后该文件转为只读回退输入。

**兼容性**（对应 V3/APPROVAL 的 B4）：新增字段一律可选，`protocolVersion` 不变；
老 host 继续读它自己的 JSON，不需要同时升级。

### 3.5 完整项目树

"项目树"= 已保存的工作区文件夹 + 每个工作区下的**全部**会话（不再受列表长度限制）。

- 用户明确选择导入后，只把旧桌面 `desktop-projects.json` 中的根路径和显示名称复制到 Preview；不自动读取稳定配置目录，也不复制 topic/session 元数据。bridge endpoint 只读 Preview 副本。
- Preview 另有 host-owned `workbench-project-folders.json`，保存用户在 Tauri 中选择或打开过的工作区根目录；它只含 root/title，不接管 Wails 数据。
- 会话根目录按身份库 `workspace_root` 分组（`sessionidentity` 已有该列）。Tauri 合并两个 Preview 文件夹清单与分页会话，因此尚无会话或最近会话已删除的文件夹仍可显示。
- 会话侧栏按当前已加载的全局会话页构造分组；有后续 cursor 或使用最多 50 条的 legacy fallback 时，组内数量都只表示当前可见数据。某组还没有已加载的可打开会话时，不把它当作空组：有 cursor 时提示继续加载；legacy fallback 时提示先重新检查目录；两种情况下都暂时禁用组选择，组内新建入口仍可用。只有实时身份分页完成后，确认空的已保存项目才按空项目语义选择为新对话默认工作区。
- Tauri 不写 Wails 持有的项目文件；旧写者的停写确认仍是切换写入行为前的独立门禁。
- 磁盘核对结果（`Inventory` 的分类）只决定文件夹可用状态，不改变文件夹身份或隐藏其下的会话。
- 无工作区的会话：作为"未指定项目"的平铺行显示，**不伪造项目实体**（第 1 项已定的语义）。
- 侧栏分组键仍是规范化后的 workspace 路径；隐藏/折叠状态继续只存在 host（`collapsedProjects`），
  不写入身份库。
- 工作区已被删除（目录不存在）的会话：项目节点标为"工作区不可用"，会话仍可打开
  （transcript 在 Preview 自己的目录里，与工作区是否存在无关）。

### 3.6 schema 迁移（v1–v3 → v4）

- v1→v2 在事务中加入标题来源与 revision；非空旧标题标为 `legacy_unknown`，空标题保留 `fallback`。
- v2→v3 在事务中加入生命周期 `state`，按旧 `missing` 标记回填为 `missing`/`ready`，并建 `(state, position, id)` 索引。
- v3→v4 先按传入的 canonical profile root 校验全部旧绝对路径及 transcript 命名，再在事务中把 `path` 改为相对 `relative_path` 并设置 `user_version=4`。任一路径越界或不符合共享路径契约时拒绝迁移；现有测试固定了 v3 数据与版本值不变的拒绝语义。
- 每个版本迁移各自使用事务；当前代码对高于所支持 schema v9 的库或完整性检查失败均拒绝打开，不把库修成空库。现有测试还验证 future schema 的版本与数据库文件不被改写。
- 已存在的旧库在 bridge 启动、开放只读盘点前完成迁移；只读盘点自身不创建数据库。bridge 首次创建 Tauri 会话时会建库并先写入 `reserved`，保证 sidecar
  产生前身份已稳定登记；这不等于切换会话列表权威。

## 4. 交付切片与验收条件

| 步骤 | 内容 | 验收条件 | 回退 |
| --- | --- | --- | --- |
| **5.0** | `resumeBridgeSession` 按身份状态优先判定；新 ID 先 reservation；识别缺文件与残留 sidecar | 已覆盖 `reserved/ready/missing`、v1–v3 → v4 迁移、missing 文件恢复后仍拒绝复活、sidecar-only 残留拒绝新建 | 当前 schema v9 仍保留 transcript 与旧 JSON 清单；回退需保留离线 profile 快照 |
| **5.1** | 完成删除 tombstone 状态写入 | `deleting/deleted` 由删除流程写入且不可重用；身份库分页已实现 | 回退时保留当前身份库及 transcript，不降级或重建数据库；使用经验证的权威 profile 数据快照（不含可再生成的顶层 `cache/`） |
| **5.2** | bridge `GET /v1/sessions` 分页列表 | 身份库分页与 bridge 接口已实现，由 5.3 的 shadow 审计和来源标记门禁后的侧栏读取 | 端点保留兼容，host 可回退旧 JSON |
| **5.3** | 影子审计通过时读取 bridge 身份目录；可读差异和 shadow 不可用时只读显示同快照身份页 | 旧 JSON 幂等导入、身份 keyset 分页、dirty-shadow `identity_unverified` 只读页、shadow 不可用时的身份页尝试、首屏最多 50 条 legacy fallback 与差异提示已接入；身份页无法读取或续页快照失效时不拼接来源。移除 JSON 回退并宣布整个 profile 唯一权威仍需独立发布门禁 | 保留 JSON 输入与有界回退；未核验页禁用会话操作 |
| **5.4** | missing/deleting/deleted 的用户可见处理 + 中断清理恢复 | missing 行不可打开但可直接删除；deleting 通过独立 path-free 清单展示，并仅在用户确认后用 DELETE 续做；不做启动期无提示自动删除。若 host 旧 catalog 留有 deleted tombstone 行，重复 DELETE 幂等成功以完成 host 行清理；明确错误提示与恢复路径已覆盖 | 保留 `missing` 记录，等待用户处理 |
| **5.5** | SQLite 崩溃恢复与删除清理中的进程崩溃恢复 | 子进程在 WAL 有已提交和未提交更新时强制退出，重开保留已提交状态并回滚未提交更新；bridge 子进程在 `deleting` 已提交、artifact sweep 已删 transcript 但尚未删 `.meta` 时强制终止，重启后拒绝打开为新空会话、继续清理并写入 `deleted` tombstone（`TestSessionIdentityRecoversAfterAbruptProcessExit`、`TestBridgeDeleteRecoversAfterForcedProcessExit`） | 仅在临时隔离 profile 演练；真实 profile 仍需停写确认和离线恢复演练 |

5.0–5.5 的后端接口、shadow 门禁、分页侧栏和 lifecycle/崩溃恢复流程均已接入。临时测试 profile 的跨资源 snapshot/staging/catalog replay 自动化已覆盖，但 SQLite 仍处于可回退的 Preview 读取阶段；dirty shadow 与 shadow 不可用时的只读身份分页不构成权威选择。移除 JSON 回退前仍须完成稳定窗口零差异、真实离线 profile 的停写后恢复演练和旧写者停写确认。重启后续删采用显式重试，不在启动时无提示地自动删除。

## 5. 测试矩阵

**已实现的 5.0 验收**
- transcript 被删 → 打开返回错误；返回信息可区分"文件缺失"与其他失败。
- 从未存在的 ID → 仍新建（回归）。
- 只有 `.jsonl.meta` 残余（无身份库）→ 拒绝新建。
- 身份库记录为 `deleted` 或 `deleting` → resolver 拒绝 Resume/新建；bridge 删除流程先持久化
  `deleting`，清理成功后持久化 `deleted`，失败时保留可重试状态且关闭时不重建 transcript。
- 身份库记录为 `missing` → 拒绝 Resume/新建；host 显示"该会话的文件已不在"。

**5.1 / 5.2**
- v1、v2 → v4：旧标题溯源、`missing` 生命周期和相对 transcript 路径正确回填；行数与 ID 不变（`TestOpenMigratesV1TitlesWithoutAssumingUserIntent`、`TestOpenMigratesV2MissingFlagToLifecycleState`）。
- v3 → v4 路径越界：拒绝迁移且原 `path` 与 `user_version=3` 保持不变（`TestV3PathMigrationRejectsPathsOutsideProfileWithoutChangingDatabase`）。
- future schema（v99）→ 拒绝打开且文件未被改写（沿用现有测试）。
- 迁移步骤各自在版本事务内提交；迁移错误不得提交该步骤的 DDL、数据更新或版本号。
- `ListVisible` 分页：209 条分 3 页无重复无遗漏；`deleted` 永不出现。
- 同一 `position` 下多条会话用 `(position, id)` 游标翻页：无漏项、无重复（已实现）。
- bridge 接口认证、缺库空列表、分页 DTO 不含 transcript 路径和内容（已实现）。

**5.3**
- 209 条会话的列表返回 209 条（分页合计），证明 50 条上限不再适用。
- 老客户端（只读 `workbench-sessions.json`）行为不变。
- JSON catalog 缺失按空目录启动；损坏 catalog 返回显式错误，且被 `remember` 保留的原始字节不变（`caps_catalog_and_fails_closed_on_corrupt_files`）。
- 响应不含 transcript 内容与凭据类字段。
- 影子盘点：身份目录额外行不误判为旧目录缺失；标题/工作区/顺序差异及 missing、物理状态漂移均计数。
- 重复 ID、分页总数漂移、未登记 transcript 或盘点错误不能得到“完全一致”结果。
- 影子报告只返回汇总数，不返回会话 ID、标题或文件路径。

**5.4**
- 同一切换点：host 列表与身份库逐项比对（ID/标题/工作区/顺序/状态）差异为零。
- 侧栏：多项目、项目内多会话、无工作区会话、工作区已删除，四种展示均正确。
- missing 会话行不可打开、提供删除入口；删除时直接退休身份，不要求先成功切换（`tauri-project-navigation.test.tsx`、`tauri-chat-workspace-delete.test.tsx`）。
- `missing/deleting/deleted` lifecycle 错误有明确提示；删除中断后可再次调用 DELETE 续做（`tauri-session-lifecycle-error.test.ts`、`TestBridgeRetriesInterruptedDeleteAfterRestart`）。
- 重启后 ID 与顺序不变；折叠状态仍只在本机。

**5.5**
- SQLite writer 子进程在 WAL 中同时留有已提交身份和未提交标题更新时强制退出 → 重开保留已提交 `ready` 身份与标题，未提交标题不可见（`TestSessionIdentityRecoversAfterAbruptProcessExit`）。
- 删除 bridge 子进程在 `deleting` 已提交、artifact sweep 已删 transcript 但遗留 `.meta` 时强制终止 → 重启确认身份仍为 `deleting`，尝试打开会话仍被拒绝，不会变成空会话；再次 DELETE 后剩余 artifacts 被清理、状态变为 `deleted`，ID 不重用（`TestBridgeDeleteRecoversAfterForcedProcessExit`）。
- `missing` 会话在 UI 上不可被当作新会话打开。

## 6. 风险与回退

| 风险 | 控制 |
| --- | --- |
| 切换权威后 host 与身份库不一致 | 5.3 要求 diff 为零并持续一个窗口；不一致即回退读 JSON |
| 单条坏数据挡住整个列表 | `ListVisible` 逐行容错，坏行计入诊断而非使整页失败 |
| 旧身份库升级失败 | 每个版本迁移单事务；真实迁移前停写并验证 profile 权威数据快照（不含可再生成的顶层 `cache/`），不单独复制活跃 WAL 下的 `.sqlite` |
| 与上游 1.38.10 会话重构冲突 | 本项只动 Preview 身份库与 host 列表；transcript 格式冻结 |
| 用户看到"会话消失"却无法自救 | `missing` 必须可解释、可删除记录；恢复路径写进 UI 文案 |

**回退总原则**：身份库是**新增**的权威，不是唯一副本。JSONL 与 `workbench-sessions.json`
在切换窗口内保持可读，因此 5.0–5.3 的任一步都能在不丢会话的前提下回退。

## 7. 需要你 review 的两个决定

1. **5.0 是否先单独合并？** 它是数据安全修复（缺文件不再变空会话），不依赖状态机，
   也不需要切换权威。建议先合，风险最低、收益最直接。
2. **50 条上限解除后，首屏取多少条？** 我倾向 200 条 + 滚动分页：与现有
   `history` 的 200 条上限一致，且单页足以覆盖绝大多数用户。若你希望首屏更小
   （例如 50 条但可继续加载），分页契约不变，只改常量。
