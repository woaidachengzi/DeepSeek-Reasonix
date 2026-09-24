# 第 5 项设计稿：持久会话目录与完整项目树（存储 S2）

> 状态：**部分实现**。身份库 schema v3 生命周期状态、首次会话 reservation、恢复前状态检查及
> 残留 sidecar 防误复用、身份库 keyset 分页、bridge 只读列表接口、删除状态的存储 API 及旧 JSON catalog 幂等导入已实现；
> bridge 删除清理已接入（包括 `deleting` 状态下的重试与重启后续删）；Rust host 全量分页影子比对及
> 运行状态面板报告已实现；host 权威切换与 UI 恢复选项仍未实现。
> 侧栏当前仍受 host JSON 的 50 条上限约束。
> 本文继续作为其余工作契约、验收条件、测试矩阵与回退路径。
>
> 背景与边界：[SESSION_STORAGE_IMPLEMENTATION_V3.md](./SESSION_STORAGE_IMPLEMENTATION_V3.md) §5 S2。
> 本稿不改动其中的边界，只把它落到本仓库当前的代码事实上。

## 1. 本项要解决的问题

| # | 问题 | 现状证据 |
| --- | --- | --- |
| 1 | **缺文件的会话会被静默重开成同 ID 的空会话** | 已由身份状态优先检查、missing 状态与 reservation 修复；见 §3.3。 |
| 2 | **最近列表硬上限 50，且会静默丢弃** | `desktop/tauri/src/workbench_catalog.rs:12` `MAX_SESSIONS: usize = 50`，`remember` 用 `take(MAX_SESSIONS - 1)` 截断。第 51 个会话会挤掉最旧的记录，被挤掉的会话**不再出现在侧栏**，也没有任何提示。 |
| 3 | **列表权威在 host 的 JSON 文件，不在身份库** | host 的 `workbench_sessions` 仍读 `WorkbenchCatalog`（`workbench-sessions.json`）；身份库写入目前只负责会话生命周期登记。 |
| 4 | **"项目树"只是按 workspaceRoot 分组的最近列表** | `desktop/frontend/src/tauri/workbenchSessions.ts` 的 `groupWorkbenchSessions`；没有项目级实体，也就没有"项目下全部会话"。 |

问题 1 是数据安全，问题 2 是功能上限，两者都必须在切换权威之前解决。

## 2. 可直接复用的既有能力（不要重写）

- 删除与产物清理：`control.RemoveSessionArtifacts`（`internal/control/controller.go:3241`），
  已覆盖 transcript、13 类 sidecar、guardian、inbox、checkpoint、子 agent、cleanup 标记。
  第 5 项**不得**新增第二套删除实现。
- 身份库：`internal/sessionidentity` 已是 schema v3，含 `path UNIQUE`、生命周期 `state`、
  `title_source`/`title_revision`（CAS，`SetTitle`）、`Import`、`OpenReadOnly`、`Inventory`。
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

V3 要求 `reserved/ready/missing/deleting/deleted`。schema v3 已包含状态列与 v2 回填；`BeginDelete` / `FinishDelete` 已接入 bridge artifact sweep。清理失败后写入被隔离，重启后的同 ID 删除请求只可续删 `deleting` 记录：

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
  游标必须是 `(position, id)` 复合键——只按 `position` 会在同 `position` 的
  并列会话上漏项或重复（SQLite 中 `position` 未强制唯一）。
- bridge 只读端点已实现：

```text
GET /v1/sessions?limit=<n>&cursorPosition=<position>&cursorId=<id>&workspaceRoot=<path>
→ { protocolVersion, sessions: [...], nextCursor?{position,id}, total }
```

  `nextCursor` 仅在还有下一页时出现；每页末项的 `(position, id)` 即下一页起点
  （严格 `>` 比较：`position > cursorPosition || (position == cursorPosition && id > cursorId)`）。
  每项至少含 `id / title / titleSource / workspaceRoot / state / missing / position / updatedAtMs`，
  **不含** transcript 内容、不含凭据类字段。
- 迁移入口 `POST /v1/sessions/import-catalog` 与 host 启动调用已接入：最多接收旧 JSON 的
  50 项；仅插入身份库中不存在的记录，保留旧目录顺序、标题与工作区；缺失 transcript 作为
  `missing` 保留。重复调用不会覆盖身份库中较新的元数据，JSON 文件仍不修改。
- host 已全量读取分页目录并提供只读影子报告，比较旧 JSON 与身份库的 ID 覆盖、标题、工作区、
  顺序及缺失 transcript 数量；扫描不完整或目录在读取期间变化时不会报告为一致。运行状态面板展示汇总，
  报告不输出会话 ID、路径或标题内容。
- host 首次只取一页（例如 200 条），滚动到底再取下一页；**不再有丢弃**。
- 迁移：`workbench-sessions.json` 的现有条目一次性导入身份库（沿用第 2、3 项已交付的
  `Import` 与 `ImportWorkbenchCatalog`，路径已同源）。导入后该文件转为只读回退输入。

**兼容性**（对应 V3/APPROVAL 的 B4）：新增字段一律可选，`protocolVersion` 不变；
老 host 继续读它自己的 JSON，不需要同时升级。

### 3.5 完整项目树

"项目树"= 工作区分组 + 每个工作区下的**全部**会话（不再受列表长度限制）。

- 数据来源：身份库按 `workspace_root` 分组（`sessionidentity` 已有该列），
  加上磁盘核对结果（`Inventory` 的分类）。
- 无工作区的会话：作为"未指定项目"的平铺行显示，**不伪造项目实体**（第 1 项已定的语义）。
- 项目实体只从会话的 `workspace_root` 派生，**不新增项目注册表**——否则要处理
  "项目删除但会话还在"的一致性问题，而本项不需要这个复杂度。
- 侧栏分组键仍是规范化后的 workspace 路径；隐藏/折叠状态继续只存在 host（`collapsedProjects`），
  不写入身份库。
- 工作区已被删除（目录不存在）的会话：项目节点标为"工作区不可用"，会话仍可打开
  （transcript 在 Preview 自己的目录里，与工作区是否存在无关）。

### 3.6 schema 迁移（v2 → v3）

- v2→v3 迁移必须**带版本号、单事务**：`ALTER TABLE` 增 `state TEXT`，由 `missing` 回填
  （`missing=1 → 'missing'`，否则 `ready`），然后建 `(state, position, id)` 索引。
- 打开时若 `user_version > schemaVersion` → 拒绝打开且**不改动文件**（现有测试语义保留）。
- 已存在的旧库在 bridge 启动、开放只读盘点前完成迁移；只读盘点自身不创建数据库。bridge 首次创建 Tauri 会话时会建库并先写入 `reserved`，保证 sidecar
  产生前身份已稳定登记；这不等于切换会话列表权威。

## 4. 交付切片与验收条件

| 步骤 | 内容 | 验收条件 | 回退 |
| --- | --- | --- | --- |
| **5.0** | `resumeBridgeSession` 按身份状态优先判定；新 ID 先 reservation；识别缺文件与残留 sidecar | 已覆盖 `reserved/ready/missing`、v2→v3 迁移、missing 文件恢复后仍拒绝复活、sidecar-only 残留拒绝新建 | 当前 schema 仍保留 transcript 与旧 JSON 清单；回退需保留 v2 DB 备份 |
| **5.1** | 完成删除 tombstone 状态写入 | `deleting/deleted` 由删除流程写入且不可重用；身份库分页已实现 | 保留 v3 备份文件即可回退读取 |
| **5.2** | bridge `GET /v1/sessions` 分页列表 | 已实现身份库分页和 bridge 接口；host 尚未切换读取 | 端点只是新增，host 不读即无影响 |
| **5.3** | host 切换到 bridge 列表，JSON 作为回退 | 旧 JSON 幂等导入与只读影子比对已接入；侧栏权威切换仍待实现 | host 改回读 JSON（一个常量开关） |
| **5.4** | missing/deleting/deleted 的用户可见处理 + 重启续做清理 | 删除中途 kill → 重启后清理完成或可重试；missing 会话有明确提示且不可"以空会话打开" | 状态回退为 `missing` 等待用户决定 |

5.0、5.1、5.2 后端接口及 5.3 的旧目录导入/影子比对已落地；5.3 权威切换与
5.4 UI 恢复选项仍待实施。重启后续删采用显式重试，不在启动时无提示地自动删除。

## 5. 测试矩阵

**已实现的 5.0 验收**
- transcript 被删 → 打开返回错误；返回信息可区分"文件缺失"与其他失败。
- 从未存在的 ID → 仍新建（回归）。
- 只有 `.jsonl.meta` 残余（无身份库）→ 拒绝新建。
- 身份库记录为 `deleted` 或 `deleting` → resolver 拒绝 Resume/新建；bridge 删除流程先持久化
  `deleting`，清理成功后持久化 `deleted`，失败时保留可重试状态且关闭时不重建 transcript。
- 身份库记录为 `missing` → 拒绝 Resume/新建；host 显示"该会话的文件已不在"。

**5.1 / 5.2**
- v2 → v3 迁移：`missing=1` 变 `missing`，其余变 `ready`，行数与 path 不变。
- 迁移中途失败 → 整体回滚，`user_version` 不变。
- future schema（v99）→ 拒绝打开且文件未被改写（沿用现有测试）。
- `ListVisible` 分页：209 条分 3 页无重复无遗漏；`deleted` 永不出现。
- 同一 `position` 下多条会话用 `(position, id)` 游标翻页：无漏项、无重复（已实现）。
- bridge 接口认证、缺库空列表、分页 DTO 不含 transcript 路径和内容（已实现）。

**5.3**
- 209 条会话的列表返回 209 条（分页合计），证明 50 条上限不再适用。
- 老客户端（只读 `workbench-sessions.json`）行为不变。
- 响应不含 transcript 内容与凭据类字段。
- 影子盘点：身份目录额外行不误判为旧目录缺失；标题/工作区/顺序差异及 missing、物理状态漂移均计数。
- 重复 ID、分页总数漂移、未登记 transcript 或盘点错误不能得到“完全一致”结果。
- 影子报告只返回汇总数，不返回会话 ID、标题或文件路径。

**5.4**
- 同一切换点：host 列表与身份库逐项比对（ID/标题/工作区/顺序/状态）差异为零。
- 侧栏：多项目、项目内多会话、无工作区会话、工作区已删除，四种展示均正确。
- 重启后 ID 与顺序不变；折叠状态仍只在本机。

**5.5**
- 删除中途强制终止 → 重启后要么清理完成，要么仍为 `deleting` 并可重试；**绝不出现空会话**。
- `missing` 会话在 UI 上不可被当作新会话打开。

## 6. 风险与回退

| 风险 | 控制 |
| --- | --- |
| 切换权威后 host 与身份库不一致 | 5.3 要求 diff 为零并持续一个窗口；不一致即回退读 JSON |
| 单条坏数据挡住整个列表 | `ListVisible` 逐行容错，坏行计入诊断而非使整页失败 |
| 迁移把 v2 库写坏 | 迁移单事务；动手前复制 `.sqlite`（停机后，不用 WAL 活跃拷贝） |
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
