# 第 5 项设计稿：持久会话目录与完整项目树（存储 S2）

> 状态：**设计稿，未实现**。本项涉及"真实数据权威切换"，按分步指令需先 review 第 2、3 项
> 的设计与测试，再决定是否动手。本文给出可执行契约、验收条件、测试矩阵与回退路径。
>
> 背景与边界：[SESSION_STORAGE_IMPLEMENTATION_V3.md](./SESSION_STORAGE_IMPLEMENTATION_V3.md) §5 S2。
> 本稿不改动其中的边界，只把它落到本仓库当前的代码事实上。

## 1. 本项要解决的问题

| # | 问题 | 现状证据 |
| --- | --- | --- |
| 1 | **缺文件的会话会被静默重开成同 ID 的空会话** | `cmd/reasonix-desktop-bridge/core_runtime.go:65-68`：`agent.LoadSession` 返回 `os.ErrNotExist` 时无条件 `controller.SetFreshSessionPath(path)`。用户看到的是"历史凭空消失"。 |
| 2 | **最近列表硬上限 50，且会静默丢弃** | `desktop/tauri/src/workbench_catalog.rs:12` `MAX_SESSIONS: usize = 50`，`remember` 用 `take(MAX_SESSIONS - 1)` 截断。第 51 个会话会挤掉最旧的记录，被挤掉的会话**不再出现在侧栏**，也没有任何提示。 |
| 3 | **列表权威在 host 的 JSON 文件，不在身份库** | host 的 `workbench_sessions` 读 `WorkbenchCatalog`（`workbench-sessions.json`）；`internal/sessionidentity` 至今只被第 3 项的只读清单使用，没有参与任何写入路径。 |
| 4 | **"项目树"只是按 workspaceRoot 分组的最近列表** | `desktop/frontend/src/tauri/workbenchSessions.ts` 的 `groupWorkbenchSessions`；没有项目级实体，也就没有"项目下全部会话"。 |

问题 1 是数据安全，问题 2 是功能上限，两者都必须在切换权威之前解决。

## 2. 可直接复用的既有能力（不要重写）

- 删除与产物清理：`control.RemoveSessionArtifacts`（`internal/control/controller.go:3241`），
  已覆盖 transcript、13 类 sidecar、guardian、inbox、checkpoint、子 agent、cleanup 标记。
  第 5 项**不得**新增第二套删除实现。
- 身份库：`internal/sessionidentity` 已是 schema v2，含 `path UNIQUE`、`missing`、
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

V3 要求 `reserved/ready/missing/deleting/deleted`。当前 schema 只有 `missing INTEGER`。
**第 5 项第一步**是把它升级为状态机（schema v3，带版本迁移，见 §3.6）：

| 状态 | 含义 | 允许的迁移 |
| --- | --- | --- |
| `reserved` | 新 ID 已登记，尚无 transcript | → `ready`（首次落盘）、→ `deleted` |
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
                    → 允许首次落盘（SetFreshSessionPath），与新建同路径
6. 无身份库记录，但磁盘存在该会话的残余（<transcript>.jsonl.meta 或 <stem>.inbox）
                    → 返回冲突错误（用于身份库尚未登记的情形）
7. 无身份库记录、无残余、文件也不存在
                    → SetFreshSessionPath（首次创建，现状不变）
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

- 身份库 `ListVisible(limit, cursor)`：按 **`(position, id)`** 排序，**分页**而非截断；
  `deleted` 不返回，`missing` 返回并带标记，`deleting` 不返回。
  游标必须是 `(position, id)` 复合键——只按 `position` 会在同 `position` 的
  并列会话上漏项或重复（SQLite 中 `position` 未强制唯一）。
- bridge 新增只读端点：

```text
GET /v1/sessions?limit=<n>&cursorPosition=<position>&cursorId=<id>&workspaceRoot=<path>
→ { protocolVersion, sessions: [...], nextCursor?{position,id}, total }
```

  `nextCursor` 仅在还有下一页时出现；每页末项的 `(position, id)` 即下一页起点
  （严格 `>` 比较：`position > cursorPosition || (position == cursorPosition && id > cursorId)`）。
  每项至少含 `id / title / titleSource / workspaceRoot / state / missing / position / updatedAtMs`，
  **不含** transcript 内容、不含凭据类字段。
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

- 迁移必须**带版本号、单事务**：`ALTER TABLE` 增 `state TEXT`，由 `missing` 回填
  （`missing=1 → 'missing'`，否则 `ready`），然后建 `(state, position, id)` 索引。
- 打开时若 `user_version > schemaVersion` → 拒绝打开且**不改动文件**（现有测试语义保留）。
- **数据库不存在**时不创建：仍由导入（S1 收尾）负责建库；切换权威只读取已存在的库。

## 4. 交付切片与验收条件

| 步骤 | 内容 | 验收条件 | 回退 |
| --- | --- | --- | --- |
| **5.0** | 修 `resumeBridgeSession`：身份状态优先于文件存在（§3.3 的 1–7 分支） | 删除 transcript 后再打开 → 明确错误；从未存在过的 ID 仍可新建；`deleted`/`deleting` 即使文件残留也不 Resume；两侧都有测试 | 纯代码，撤销即回退 |
| **5.1** | 身份库 schema v3（状态机）+ `ListVisible` 按 `(position, id)` 分页 | v2 库可原地迁移；future schema 拒绝打开；`deleted` 不可重用有测试；同 `position` 并列不漏项 | 保留 v2 备份文件即可降级读取 |
| **5.2** | bridge `GET /v1/sessions` 分页列表 | 200+ 会话全部可见，无截断；`deleted` 不出现；契约测试覆盖可选字段；老客户端不受影响 | 端点只是新增，host 不读即无影响 |
| **5.3** | host 切换到 bridge 列表，JSON 作为回退 | 新旧列表 diff 为零或可解释；重启后 ID 不变；侧栏项目树完整 | host 改回读 JSON（一个常量开关） |
| **5.4** | missing/deleting/deleted 的用户可见处理 + 重启续做清理 | 删除中途 kill → 重启后清理完成或可重试；missing 会话有明确提示且不可"以空会话打开" | 状态回退为 `missing` 等待用户决定 |

5.0 可以先单独落地（它是数据安全修复，不依赖状态机）；5.1–5.4 按序。

## 5. 测试矩阵

**5.0**
- transcript 被删 → 打开返回错误；返回信息可区分"文件缺失"与其他失败。
- 从未存在的 ID → 仍新建（回归）。
- 只有 `.jsonl.meta` 残余（无身份库）→ 拒绝新建。
- 身份库记录为 `deleted` 或 `deleting` → 拒绝 Resume/新建，**即使 transcript 文件仍在**（墓碑优先）。
- 身份库记录为 `missing` → 拒绝 Resume/新建；host 显示"该会话的文件已不在"。

**5.1**
- v2 → v3 迁移：`missing=1` 变 `missing`，其余变 `ready`，行数与 path 不变。
- 迁移中途失败 → 整体回滚，`user_version` 不变。
- future schema（v99）→ 拒绝打开且文件未被改写（沿用现有测试）。
- `ListVisible` 分页：200 条分 3 页无重复无遗漏；`deleted` 永不出现。
- 同一 `position` 下多条会话用 `(position, id)` 游标翻页：无漏项、无重复。

**5.2**
- 209 条会话的列表返回 209 条（分页合计），证明 50 条上限不再适用。
- 老客户端（只读 `workbench-sessions.json`）行为不变。
- 响应不含 transcript 内容与凭据类字段。

**5.3**
- 同一切换点：host 列表与身份库逐项比对（ID/标题/工作区/顺序/状态）差异为零。
- 侧栏：多项目、项目内多会话、无工作区会话、工作区已删除，四种展示均正确。
- 重启后 ID 与顺序不变；折叠状态仍只在本机。

**5.4**
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
