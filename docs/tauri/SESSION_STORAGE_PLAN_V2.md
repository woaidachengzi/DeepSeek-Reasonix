# 会话存储改造方案 v2（供评审）

> 状态：设计草案，尚未实现。本文取代
> [MIMO_STYLE_SESSION_STORAGE_PROPOSAL.md](./MIMO_STYLE_SESSION_STORAGE_PROPOSAL.md)
> 的落地部分；原提案的 MiMo 职责划分分析、以及本地源码核对结论见
> [MIMO_SOURCE_COMPARISON.md](./MIMO_SOURCE_COMPARISON.md)。
>
> 与 [MIMO_STYLE_SESSION_STORAGE_APPROVAL.md](./MIMO_STYLE_SESSION_STORAGE_APPROVAL.md)
> 是并行产出的两份稿：**编号（A/B/C/Q）沿用 APPROVAL 稿**，本方案只在第 13 节声明
> "采纳 / 覆盖 / 补充"，不另起一套编号。评审请一并读第 13 节，否则会看到两套结论。
>
> 基线：`experiment/tauri`，基于 `MIMO_STYLE_SESSION_STORAGE_REVIEW.md` 的修订。
>
> 评审请重点看第 4、6、13 节：那里是"必须成立否则白做"的路径与身份规则，
> 以及两份稿之间尚未闭合的分歧。

## 0. 本版相对上一版改了什么

| # | 上一版的问题 | 本版处理 |
| --- | --- | --- |
| 1 | 导入器假定"带工作区的会话存于 `projects/<slug>/sessions/`"，与真实写入路径不一致，会把带工作区的会话**全部标记为 missing** | 第 4 节：路径由**唯一一个**共享构造函数决定，导入器不再自行推导 |
| 2 | `ErrPathChanged` 是死路：登记错误路径后无法纠正 | 第 6.3 节定义 Move。**（§13.5 已按 V3 修正：alias 表推迟到 Move 语义确定，阶段 1 仍靠 `ErrPathChanged` 防守）** |
| 3 | 只有"身份库"没有"会话→路径"的权威，host 与 sidecar 各自持有推导规则 | 第 4 节：`TranscriptPath` 成为唯一权威，host 不再拼路径 |
| 4 | 未定义"带什么证据才算同一个会话" | 第 6.4 节。**（§13.5 已按 V3 重写：原列的三级证据中前两条不成立，改为阶段 1 不做自动重连）** |
| 5 | 数据库 pragma 顺序与耐久性取舍未说明 | 第 6.2 节（含 `synchronous=FULL` 的理由） |
| 6 | 交付顺序缺少"如何验证导入正确" | 第 7.1 节：导入必须产出可核对清单并逐条比对 |
| 7 | 导入输入只有 `workbench-sessions.json`，**扫不到任何不在最近列表里的会话** | 第 6.6 节：新增只读孤儿扫描器（确定性 ID 推导、只读 SHA-256、未认领文件必须列出） |
| 8 | 未与并行的 APPROVAL 稿对齐，评审会读到两套结论 | 第 13 节：决议索引，逐条声明采纳/覆盖/补充与待裁定冲突 |
| 9 | **§4.1 断言 `internal/config` 是 leaf（错）；§4.2 把 `projects/` 说成凭空推导（过头）；§6.4 把 `.jsonl.meta` 的 ID 当独立证据（循环）** | 第 13.5 节：三条均经核实为 V3 正确，已逐条更正；同时采纳 V3 的 `relative_path`、生命周期状态机、alias 延后 |
| 10 | 未发现 `resumeBridgeSession` 的**静默变空会话**缺陷 | 第 13.5 节记录该缺陷与代码位置；§6.5 的 `missing` 升级为阻断条件 |

## 1. 目标与非目标

**目标**：让 Tauri Preview 的会话身份、标题、工作区和最近列表脱离"文件名 + 可重建索引"，进入独立、持久、可备份的 SQLite；同时**不动**现有 JSONL 执行格式。

**非目标（本方案不承诺）**：

- 不把消息、工具事件、推理内容搬进 SQLite（那是第 10 节的可选后续 RFC）。
  这条不是保守，而是对齐 MiMo 的实际职责划分——**第 14 节是直接读 MiMo 源码得到的
  逐条核实结果**，其中也包括我们明确**不跟**的地方（如 `synchronous=NORMAL`）。
- 不迁移稳定版 Wails 用户数据；不在稳定目录上就地升级。
- 不改变 Wails / CLI / Serve 的会话格式或身份语义。
- 不承诺"索引可全量重建"与"ID 永不变"同时成立——两者冲突，本方案选择**ID 永不变**（见 3.2）。

## 2. 事实基线（可复核）

以下每条都能在仓库或运行数据里验证。评审若发现任一条不成立，本方案的前提即失效。

### 2.1 路径由谁决定

全仓只有**两处**构造会话 transcript 路径：

| 位置 | 代码 |
| --- | --- |
| sidecar（唯一写入方） | `cmd/reasonix-desktop-bridge/core_runtime.go:64` → `bridgeSessionPath(controller.SessionDir(), sessionID)` |
| 身份导入器 | `internal/sessionidentity/catalog.go:58` → 自行拼 `projects/<slug>/sessions/` |

`controller.SessionDir()` 在 bridge 未设置 `opts.SessionDir` 时落到
`config.SessionDir()`（`internal/boot/boot.go:569-571`），即
`<REASONIX_HOME>/sessions`（`internal/config/paths.go:429-435`）。

`bridgeSessionPath`（`core_runtime.go:80-93`）的产物恒为：

```text
<sessionDir>/tauri-<sessionID>.jsonl
```

**它从不含 `projects/` 分段，也从不理会 `workspaceRoot`。**

### 2.2 运行数据核对

真实 Preview profile（本机实测）：

```text
~/Library/Application Support/io.reasonix.desktop.preview/
├── workbench-sessions.json          # host 的最近列表（当前 3 条）
├── window-state.json
└── reasonix-core/                   # = REASONIX_HOME
    ├── cache/{usage-catalog,environment,history-search}
    ├── sessions/                    # 全部 transcript 与 sidecar 都在这里，扁平
    └── stats/
```

- `reasonix-core/` 下**没有** `projects/` 目录，profile 根下也没有。
- 3 条 catalog 中 2 条带 `workspaceRoot`；这两条的 transcript **实际存在于**
  `reasonix-core/sessions/tauri-tauri-<UUID>.jsonl`。
- 按导入器推导的路径 `reasonix-core/projects/<slug>/sessions/...` **不存在**。

结论：**`workspaceRoot` 只是"新对话默认用哪个工作区"的 UI 提示**，不参与落盘位置。

### 2.3 其他既有事实

- 身份库路径：`config.DesktopSessionIdentityPath()` =
  `<userSupportDir>/desktop/session-state-v1.sqlite`；
  `userSupportDir()` 先看 `REASONIX_STATE_HOME`，否则 `REASONIX_HOME`
  （`internal/config/paths.go:146-151, 553-555`）。Tauri 把 `REASONIX_HOME`
  设为 `<preview-app-data>/reasonix-core`（`desktop/tauri/src/data_profile.rs:11,77,231`）。
  → 身份库落在 **`<preview-home>/reasonix-core/desktop/session-state-v1.sqlite`**，
  即 state home 内，不是 cache。
- `internal/sessioncatalog`、`internal/historycatalog` 已存在，是可重建投影。
- `internal/sessionidentity` 已存在且**没有任何调用方**（未接入）。
- 真实的 Preview profile 下**尚无** `session-state-v1.sqlite`，即**未执行过导入**。

### 2.4 一个会话实际拥有哪些文件

`store.SessionSidecarFiles`（`internal/store/session.go:238-257`）返回 13 个 sidecar
路径（meta、goal-state、event-log 及其 damaged/rotating 变体、turn-event-log、
event-index、display-index、conflict-log、recovery-state、context、pinned-context）。
在此之上，会话还拥有：

- `<base>.inbox/`（`sessioninbox`，删除时用 `RemoveDir`）
- `<base>.ckpt/`（checkpoint 目录，删除时 `os.RemoveAll`）
- `.reasonix/attachments/` 里被该会话引用的副本
- 子 agent 记录（按 parent 关联，`agent.DeleteSubagentsByParent`）
- cleanup-pending 标记（`agent.ClearCleanupPending`）

删除的唯一权威实现是 `control.RemoveSessionArtifacts`（`internal/control/controller.go:3241-3286`）。
**任何新代码都不得重新实现这套清理**，只能调用它。

## 3. 设计原则

### 3.1 单一权威

| 事实 | 唯一写入权威 | 其他角色 |
| --- | --- | --- |
| 会话身份（ID）、标题、工作区、最近顺序、缺失标记 | **Go sidecar 的 SQLite 身份库** | Tauri host 只经 bridge 读 |
| transcript 与执行记录 | 现有 `*.jsonl` + sidecar | 身份库只记路径，不改内容 |
| 目录投影、历史检索 | `sessioncatalog` / `historycatalog`（可重建） | 可从 JSONL 重建，丢了不影响身份 |
| 长期记忆 | `internal/memory` 的 Markdown | 独立 FTS5 索引（后续） |

同一事实只有一个写入者。**host 不直接改身份库**；host 的
`workbench-sessions.json` 在迁移期只作为导入输入，核对完成后停止读取（不删除，留作回退）。

### 3.2 ID 与索引的关系

"索引可全量重建"意味着"ID 可重新分配"，与"ID 永不变"矛盾。本方案：

- 身份库**不是**索引，是权威存储，进入备份范围。
- `sessioncatalog` / `historycatalog` 继续是可重建投影。
- 身份库丢失时：从备份恢复；若只剩 JSONL 且无可验证的 ID 证据，**可以恢复内容，但不能承诺原 ID**。这句话必须出现在用户可见的恢复提示里。

### 3.3 Preview 隔离

- Preview 的 `REASONIX_HOME` 默认隔离（`data_profile.rs`）。
- 禁止在稳定版目录上就地迁移；导入 = 备份 + 复制到 Preview + 为副本登记身份。
- 单实例锁保证同一 profile 不被两个宿主同时写（Phase 2 交付）。

## 4. 路径规则（本方案的核心修正）

### 4.1 唯一构造函数

新增一个导出函数，作为**全仓唯一**的会话路径构造点：

```go
// internal/sessionpath（新包，不依赖任何 reasonix/ 包，见下方分层说明）
//
// TranscriptPath 返回一个会话 ID 在给定 session dir 下的 transcript 路径。
// 它是唯一权威：bridge 用它写，身份库用它核对，host 不得自行拼接。
func TranscriptPath(sessionDir, sessionID string) (string, error)
// → <sessionDir>/tauri-<sessionID>.jsonl
// 校验：sessionDir 非空；sessionID 非空、≤128、仅 [A-Za-z0-9_-]
```

`cmd/reasonix-desktop-bridge/core_runtime.go` 的 `bridgeSessionPath` 改为调用它
（或整体迁移过去），语义与错误信息保持不变，现有测试继续通过。

`internal/sessionidentity` 的导入器改为调用它，**删除 `projects/<slug>/` 分支**。

**分层结论（已核对 `tools/repolint/layers.go`）**：`internal/store` 在 `leaves`
列表里——utility 层包禁止 import 任何 `reasonix/` 包（`violates` 的第一条分支）。

> **更正（V3 指出，已核实）**：`internal/config` **不在** `leaves` 里，全表为
> ablation / agentpreset / billing / diff / extension/rpcwire / extensioncontract /
> filelock / fileref / fileutil / fileutil/encoding / frontmatter / i18n / mcpdiag /
> nilutil / planmode / proc / releaseasset / retrieval / shellparse / store / sysproxy /
> textutil。本方案上一版写成"两者都是 leaf"是错的。结论不变、论据修正：

- 新包**必须自带**这段路径构造，不能为复用而 import `internal/store`（`store` 是
  leaf，leaf 之间互导立刻违规）。
- 新包自身也必须对 `reasonix/` 零依赖，才能被 `cmd/` 与 `internal/sessionidentity`
  同时 import 而不引入环。`internal/config` 虽然可以 import，但引它会把 config
  的依赖图带进这个纯字符串函数，没有必要。
- 若评审倾向不加新包，替代方案是**让身份库只存 `file_name`**（由导入方/调用方
  提供完整路径），本方案不推荐：那会把路径推导又散回各调用方，正是本次要消除的问题。

### 4.2 由此得到的推论

- `workspaceRoot` 与 **Tauri bridge 的** transcript 路径**无关**，只写入身份行的元数据列。
- 会话 ID 允许不含前缀的裸 UUID。现状是 host 生成 `tauri-<uuid>`，构造函数再加一层
  `tauri-` 前缀，得到 `tauri-tauri-<uuid>.jsonl`。**这个双前缀是既有事实，不是笔误**：
  磁盘上已有这类文件，改前缀等于改名，需要走显式 move。评审若认为该命名应修正，
  必须作为独立的、带 move 的迁移提出，不能混进本次身份改造。
- 导入器与 sidecar 的路径若仍不一致，**导入必须失败并报出两侧路径**，不允许静默标记 missing。

> **重要澄清（V3 指出，已核实并收窄了本方案的说法）**：本方案上一版把导入器的
> `projects/<slug>/sessions/` 分支说成"凭空推导"，这个措辞**过头了**。
> `config.ProjectSessionDir()`（`internal/config/paths.go:451-462`）是**真实存在且在用**的
> 工作区会话目录，`internal/cli/session_machine.go:156`、
> `internal/cli/sessions_catalog.go:306,315`、`internal/doctor/session_bundle.go:341`
> 都在用，bot 侧也有自己的 `botSessionDir(workspaceRoot)`（`internal/bot/gateway.go:2358`）。
>
> 准确的说法是：**不同前端用不同的 session dir**——CLI / desktop 工作区会话按项目分目录，
> 而 **Tauri bridge 走的是扁平全局 `config.SessionDir()`**（`core_runtime.go:64`，未设置
> `opts.SessionDir`）。所以缺陷不是"这个目录不存在"，而是**导入器按 `workspaceRoot`
> 自行推导目录，而不是使用真正写入方使用的那个 session dir**。
>
> 因此 §4.1 的修正原则要写成：**导入器必须接收调用方传入的实际 session dir**
> （bridge 的 `controller.SessionDir()`），不得从 `workspaceRoot` 推导。未来若 Tauri
> 改用工作区目录，导入器不需要改代码，只换传入的 dir——这也是 V3 的主张。

### 4.3 回归测试（防止再次漂移）

必须有一条测试，把"真实目录布局"钉住：

```text
给定 sessionDir=<tmp>/sessions、ID=tauri-abc、workspaceRoot=/w/x
  TranscriptPath(...) == <tmp>/sessions/tauri-tauri-abc.jsonl
  且：用 bridgeSessionPath(...) 产出相同字符串
  且：ImportWorkbenchCatalog 对同一 catalog 条目登记的 Path 与之相同
```

同时替换 `store_test.go:167` 那条把错误路径写成期望值的测试。

## 5. 身份库 schema（v1，已并入 V3 的 relative_path 与状态机）

> 本节已按 §13.5 的裁定更新：路径改为**相对 state root** 的 `relative_path`，
> `missing` 布尔列升级为**生命周期状态机**，alias 表**推迟**到 Move 语义确定。

```sql
PRAGMA user_version = 1;

CREATE TABLE sessions (
  id             TEXT PRIMARY KEY,          -- 会话 ID，登记后不得变更；进入 deleted 后不得重用
  relative_path  TEXT NOT NULL UNIQUE,      -- 相对 state root，如 sessions/tauri-tauri-abc.jsonl
  workspace_root TEXT NOT NULL DEFAULT '',  -- UI 元数据，不参与路径
  title          TEXT NOT NULL DEFAULT '',
  title_source   TEXT NOT NULL DEFAULT 'user'
                 CHECK (title_source IN ('user','generated','fallback')),
  title_revision INTEGER NOT NULL DEFAULT 0,
  position       INTEGER NOT NULL DEFAULT 0,-- 越小越新（与 workbench_catalog 一致）
  state          TEXT NOT NULL
                 CHECK (state IN ('reserved','ready','missing','deleting','deleted')),
  created_at_ms  INTEGER NOT NULL,
  updated_at_ms  INTEGER NOT NULL
);

-- 可见列表的稳定排序（与 host 的"越小越新"一致）
CREATE INDEX sessions_visible_order ON sessions(state, position, id);

-- alias 表推迟到 Move 语义确定后再加（§13.5）。在此之前 ErrPathChanged 是防线。
```

**存储 API 必须按动作分开，不得用一个通用 Upsert 兼办多件事**（对齐 V3 §3.2）：

```text
ImportKnown(candidates)   只读校验后登记；不改已有行的路径
ReserveNew(id)            新 ID 登记为 reserved（尚无 transcript）
ResolveForOpen(id)        解析身份行 → 定位文件；区分"无记录的新建"与"已登记但 missing"
MarkPersisted(id)         reserved → ready（首次真正落盘后）
MarkMissing(id)           ready → missing（文件消失）；绝不回落到"新建空会话"
BeginDelete(id)           → deleting，阻断新写
FinishDelete(id)          → deleted（保留 tombstone，禁止 ID 重用）
ListVisible()             给 host 的最近列表（排除 deleting/deleted）
```

设计说明：

- **不存绝对路径作为主键**：路径会变，ID 不变。`relative_path` 是相对 state root 的
  定位，由 §4.1 的构造函数与根目录解析共同得出；拼接结果与 `TranscriptPath` 不一致
  即数据损坏信号。
- **为什么是相对路径而不是绝对 `session_dir`**：这是 V3 的改进（§13.5 已采纳）。
  恢复整份 profile 到另一个绝对位置时，相对路径**不需要批量改库**；绝对路径必须
  整体 move。唯一性约束（`UNIQUE`）强度不变。
- **相对路径必须做逃逸校验**：拒绝 `..` 段、绝对路径、经符号链接逃逸的目录。
  根目录由宿主与 sidecar 的**同一个** profile 解析得出，**不得从库内旧值反推**。
- **状态机取代 `missing` 布尔列**：`missing` 无法表达"正在删"与"已删不可重用"，
  而这两者正是 §13.5 记录的静默变空会话缺陷与删除非原子性的解药。
- 上一版的 `path NOT NULL UNIQUE` 被替换：它把"路径"升格为身份的一部分，
  正是第 2.1 节缺陷的温床。
- **`title_source` 是照 MiMo 源码补的**（`session.sql.ts:30-31`：`title_source` +
  `title_revision`）。理由落到本项目：Preview 将来若接入自动生成的会话标题，
  **生成结果绝不能覆盖用户手改的标题**。MiMo 的守卫是显式的
  （`session/projectors.ts:95-99`）：`user` 标题不可被降级为 `generated`，
  `fallback` 不可从非 `fallback` 回退。本项目已有
  `agent.RenameSessionIfTitleUnchanged` 提供"读取-比较-写入"的原子性；
  `title_revision` 是它的库内版本号，供 CAS：`UPDATE ... WHERE id=? AND title_revision=?`，
  冲突则放弃本次写入并重新读取。**没有这一列，写入权威仍然只有"最后写的赢"**。
- 删除语义：`BeginDelete` → 调用 `control.RemoveSessionArtifacts` → `FinishDelete`
  留 tombstone。文件系统与 SQLite **无法组成一个原子事务**，所以不承诺"同时消失"，
  而是用持久状态机 + 重启重试收敛（见 §13.5 采纳 V3 的第 2 条）。

## 6. 行为规格

### 6.1 导入（Phase 1）

输入有两个来源，合并后一次事务写入：

| 来源 | 提供 | 权威性 |
| --- | --- | --- |
| `workbench-sessions.json` | ID、标题、工作区、顺序 | 标题/工作区/顺序**以此为准** |
| 只读扫描器（6.6 节） | 仅 workbench 未覆盖的 ID | 只补条目；标题留空、排在末尾 |

步骤：

1. **预扫描阶段（事务之外，只读）**：校验 catalog（≤50 条、ID 合法、无重复）；
   为每条候选推导 `Path = TranscriptPath(<home>/sessions, id)`；`Lstat` 判定 missing
   （**不创建任何文件**）；扫描 `sessions/` 补齐孤儿 ID（规则见 6.6）；记录每条来源。
2. **冲突判定阶段（事务之外）**：已登记 ID 的路径若与推导值不同 → 返回错误并
   **列出两侧路径**，不写任何东西。这与 6.6 的"跳过"是两类不同的失败，见下。
3. **写入阶段（单事务）**：只做 INSERT/UPDATE，不做文件 I/O。
4. 未落盘且未登记的条目跳过（避免把"没写过的新标签"变成空会话）。
5. 输出导入清单（第 7.1 节），并打印一份只读核对视图供评审独立复核。

**两类容错必须分开，不得混用**（对齐 MiMo `json-migration.ts` 的实际做法）：

| 情况 | 处理 | 依据 |
| --- | --- | --- |
| 已登记 ID 的推导路径与库中不同 | **整体失败**，一条不写 | 这是数据完整性信号，猜测会静默改写身份 |
| 单条候选不可读 / 校验不过 / 父实体缺失 | **跳过该条并计入错误清单**，其余照常提交 | MiMo 对孤儿会话即跳过 + 计数 + warn；整体失败会让一个坏文件挡住全部导入 |

无论走哪条，**错误清单都必须随导入结果返回并打印**；"跳过了 N 条"绝不能只写日志。

### 6.2 数据库打开（修上一版的顺序与耐久性）

按此顺序，每一步失败即关闭连接并返回错误（不留半初始化状态）：

1. 建目录并 `chmod 0700`。
2. `busy_timeout` 先装，再 `journal_mode=WAL` —— 顺序与 MiMo `storage/db.ts:92-97`
   一致，其注释写明"WAL 本身要拿锁，先装 busy handler，避免并发连接在到达写路径前就失败"。
3. `synchronous=FULL` —— 身份库丢了就没法保证 ID 不变；代价是每次提交多一次 fsync，
   而身份写入频率是"每次开/改/删会话一次"，可接受。**MiMo 用 NORMAL**（他们的库是唯一
   权威，用回滚风险换吞吐）；本方案不跟，理由是写入频率低而 ID 不变量优先级更高。
   若评审要求对齐 MiMo，必须同时接受"崩溃可能丢掉最近若干条身份行"。
4. `foreign_keys=ON`。
5. `quick_check` —— 放在 WAL 建立之后，检查的是实际会用的那份文件。
6. schema 迁移：采用**带时间戳的有序迁移目录**（`migration/<timestamp>_<name>.sql`），
   按名字排序应用，并记录已应用版本；而不是把 DDL 写死在 Go 代码里。
   阶段 0 只有 v1 一个文件，但目录结构现在就按可演进设计。
   大于已支持版本 → 拒绝打开且**不改动文件**（沿用现有测试语义）。

> 上一版把 `busy_timeout` 放在 WAL 之前。go-sqlite3 系驱动是在连接建立时注册该
> 超时，顺序影响有限，但显式排在锁操作之后更不易误读；真正需要评审确认的是第 3 步的
> 耐久性取舍，而不是这个顺序本身。

### 6.3 显式移动（修上一版死路）

`Move(id, newSessionDir)`：

- 目标拼接路径必须存在且为 regular file；不得被其他 ID 的当前路径或 alias 占用。
- 旧路径入 alias，更新 `session_dir`，`updated_at_ms` 刷新。
- 找不到 ID → 明确错误（不静默新建）。
- **并发容错照抄 MiMo 的 rename 写法**（`session/checkpoint-paths.ts:48-60`）：
  两个写者可能都通过了前置存在性检查，后者的操作会看到目标已不存在——
  `ENOENT` 视为**对方已成功**，其余文件系统错误照常抛出。这条直接适用，
  因为 alias 写入与移动同样存在"并发都可能通过预检"的窗口。

### 6.4 自动重连的匹配证据（**已被 V3 推翻并重写**）

> 本节上一版列了三级"证据"，其中第 1、2 条**经核实不成立**，现改为"阶段 1 不做
> 自动重连"。以下按核实结果重写，保留推翻过程以便评审复核。

**第 1 条不成立（循环证据）**：`.jsonl.meta` 的 `BranchMeta.ID` **恒等于文件名**。
`BranchID()` 就是 `filepath.Base` 去掉扩展名（`internal/agent/branch.go:186-195`），
而所有写入 `meta.ID` 的地方都直接调用它：`branch.go:223`、`321`、`409`、`596`、`602`。
所以 meta 里的 ID 只是文件名的副本，不能证明"这个文件是原来的会话"。

**第 2 条不成立（已逐字段核实）**：两个索引里都没有会话 ID。

- `sessionEventIndex`（`internal/agent/session_events.go:122-130`）字段为
  `schema_version / log_size / message_count / revision / content_digest / writer_id /
  updated_at`——`writer_id` 是**写者**身份（机器/进程），不是会话身份。
- `SessionDisplayIndex`（`internal/agent/session_display_index.go:33-53`）字段为
  `schema_version / revision / revision_known / content_digest / transcript_size /
  message_count / authored_turns / listing_preview(_known) / entries`，同样没有会话 ID。

因此这两个索引只能证明"文件与自身的哪个版本一致"，**不能证明它属于哪个会话**。

**结论（采纳 V3）**：文件名 `tauri-<id>.jsonl` 只提供**候选 ID**；复制、改名、
同名冲突时都无法自动断定身份。**阶段 1 不做自动重连或物理移动**：歧义一律保持
`missing`，由用户在 UI 显式选择认领。

**若要为将来建立真正的独立证据，正确做法不是往 `.jsonl.meta` 里加字段**：
`saveBranchMetaContext` 走的是类型化 `BranchMeta` 结构
（`internal/agent/branch.go:314-344`，只额外 preserve 了 persistence 相关字段），
**未知 JSON 字段会被丢弃**。可选方案是本项目自有的一个 sidecar，例如
`<transcript>.preview-id`，只由 bridge 写、只由 bridge 读，并纳入
`RemoveSessionArtifacts` 的清理范围（`controller.go:3241-3286`）。
这需要独立设计，不在阶段 1。

**禁止**用文件大小或 mtime 判定"同一个会话"；它们只能用于跳过未变文件的扫描。

**禁止**用文件大小或 mtime 判定"同一个会话"；它们只能用于跳过未变文件的扫描。

### 6.5 生命周期

| 操作 | 进入方式 | 退出条件 |
| --- | --- | --- |
| 新建 | host 生成 ID → bridge 登记 → 返回身份行 | 首次提交后 view 落盘 |
| 打开/切换 | 由 bridge 解析 ID → `TranscriptPath` → resume | 路径或 alias 命中唯一记录 |
| 重命名 | 写 `title`（≤120 rune、无控制字符），同时写 `title_source`；点"重命名"入口写入 `user`，自动生成写入 `generated` | JSONL 不动；`user` 不可被 `generated` 覆盖；CAS 用 `title_revision` |
| 删除 | 先 `RemoveSessionArtifacts` 成功，再删行 | 文件与行同时消失 |

`running`/`paused` 这类**进程状态**不写入身份库：启动后必须从 Controller 与
运行记录重新核实，避免把上次退出时的状态当成事实。

### 6.6 孤儿 JSONL 的发现（合并 APPROVAL B1 / Q3）

只读 `workbench-sessions.json` **不足以**建立完整身份：它最多 50 条，且
**不包含任何未进入最近列表的会话**——包括用户用 CLI 在同一个 `REASONIX_HOME` 下创建的
会话、被列表挤出的旧会话、以及删除/移动后残留的文件。上一版把这部分留空，
等于"身份库只能覆盖最近用过的那些会话"。

因此阶段 1 必须同时提供一个**只读扫描器**：

```text
ScanOrphans(sessionDir) -> []Candidate
```

规则：

1. 只认 `sessionDir` 下的**一层**普通文件，名称通过 `store.IsSessionTranscriptName`
   判定（该函数已存在，`internal/store/session.go:23-30`，会排除
   `.events.jsonl` / `.turns.jsonl` / `.conflicts.jsonl` / `.guardian.jsonl`）。
   不递归、不跟随符号链接。
2. **ID 从文件名确定性推导**，不随机分配：
   - 文件名形如 `tauri-<id>.jsonl` → ID = `<id>`（与 `TranscriptPath` 互为逆运算）。
   - 其他形状 → **不导入**，列入"未认领文件"清单并请用户显式处理（Q3 的"拒绝"分支）。
   - 推导出的 ID 必须通过 `TranscriptPath` 往返校验；不等则拒绝该文件。
3. **绝不分配新 ID 给无法推导的文件**。理由：随机 ID 会让"重扫幂等"失效——
   同一个文件二次扫描会得到不同 ID，这正是本方案要避免的漂移。
4. 与 workbench 导入的关系：**workbench 优先**。同一 ID 同时出现在两处时，
   以 workbench 的标题/工作区/顺序为准；扫描只补它没有的 ID，且这些补入项
   `position` 统一排在 workbench 之后，`title` 留空（由 UI 显示派生名）。
5. 扫描**只读**：前后对全部被扫描文件做 SHA-256 比对，必须逐字节不变；
   不创建、不重命名、不删除任何文件，也不为缺失的 sidecar 补文件。
6. 扫描结果必须能导出成**导入清单**（第 7.1 节）供人工核对，并且清单里要能区分
   `source=workbench` 与 `source=scan`。

> 评审焦点：第 2 条的"确定性推导"是"重扫幂等"与"不重分配 ID"两条不变量的交点。
> 若评审倾向给孤儿文件分配新 ID（APPROVAL Q3 的另一种倾向），必须同时给出
> "同一文件二次扫描得到同一 ID"的持久化依据（例如把文件名指纹写进库），
> 否则请在 §13 明确否决本方案的第 2 条。

## 7. 交付顺序与退出条件

| 阶段 | 内容 | 通过条件 | 回退 |
| --- | --- | --- | --- |
| 0 | 冻结本文第 3–6 节；`sessionpath` 抽取 + 回归测试；修正 `store_test.go`；**补 managed Preview 的 state-home 覆盖检查**（§13.5 的隔离项） | 两侧路径测试同源；旧测试全绿；旧 catalog 仍可用；**不打开也不创建真实身份库** | 纯重构，无数据变更 |
| 1 | 身份库只读导入：workbench 导入 + **孤儿 JSONL 只读扫描**（6.6）产出**候选清单**，未接入启动流程 | 重扫幂等；中断可重跑；JSONL 字节不变（SHA-256 比对）；清单逐条与磁盘比对一致；未认领文件被列出而非静默忽略 | 删库重导入（仅限**从未被引用**的测试库） |
| 2 | shadow 先行：新会话 `reserved`→`ready`；打开/切换/重命名/删除都经 resolver；host 仍读 JSON 并**后台比对**差异，零差异后才切读 bridge 列表 | 重启 ID 不变；**缺文件不变空会话**（§13.5 的 `resumeBridgeSession` 缺陷已修）；删除中途失败可续做；两进程同 profile 被阻止；新旧列表差异可解释 | 恢复旧读路径，但**保留**已成为权威的身份库与 tombstone，不删行、不重分配 ID |
| 3 | 记忆与检索：复用历史 FTS5；Markdown 记忆独立可重建索引；按预算注入 notes/progress/checkpoint | 手工编辑可重新索引；只注入最新有效快照；诊断不泄露消息与密钥 | 关闭索引，退回纯文件检索 |
| 4（可选 RFC） | SQLite 消息/工具事件表、顺序号、提交确认、JSONL 导入；先影子校验再切新会话写入 | 崩溃/重放/审批/撤销/分支/压缩/导出回滚全通过；旧 JSONL 仍可读 | 保留 JSONL 双写一个周期 |

**阶段 1–3 完成后不得对外宣称"已完成 SQLite 迁移"**；只有阶段 4 才意味着整段对话进 SQLite。

### 7.1 阶段 1 的导入清单（新增要求）

导入必须生成一份可人工核对的清单，至少包含：

```text
id | 来源(workbench/scan) | 推导路径 | 磁盘是否存在 | 判定(missing/ok) | 标题 | workspace_root
```

并且提供一条**只读核对命令**（不写库）打印同样内容，供评审独立复核。
上一版没有这一步，导致错误路径无法在接入前被发现。

清单还必须单列一节 **"未认领文件"**：扫描到但无法确定性推导 ID、或推导后往返校验
失败的文件。它们**不进入库里**，但必须被报出来，否则用户会以为"身份库已经覆盖全部会话"。

## 8. 与现有 Stage 1 代码的差异（实施清单）

1. `internal/sessionidentity/store.go`：schema 改为第 5 节；pragma 顺序改为 6.2；
   `Import` 输入改为 `(sessionDir, []Candidate{ID, WorkspaceRoot, Title, Position, Source})`，
   路径在内部用 `sessionpath.TranscriptPath` 推导。
2. `internal/sessionidentity/catalog.go`：删除 `projects/<slug>/` 分支；保留顺序与
   工作区元数据；路径推导交给 store。
3. **新增** `internal/sessionidentity/scan.go`：6.6 节的只读扫描器，复用
   `store.IsSessionTranscriptName`；扫描器不得 import `internal/control`（分层规则）。
4. `internal/sessionidentity/store_test.go:167`：期望路径改为与 bridge 同源；
   新增第 4.3 节的漂移回归测试。
5. **新增测试**：扫描器的幂等性（同目录扫两次结果一致）、SHA-256 前后不变、
   往返校验拒绝（`TranscriptPath` 逆推不成立的文件）、未认领文件不得入库。
6. `cmd/reasonix-desktop-bridge/core_runtime.go`：`bridgeSessionPath` 改为调用共享函数。
7. 尚未接入启动流程这一点保持不变；本方案的阶段 2 才动 host。

## 9. 验证矩阵

### 9.1 数据样本（迁移前必须准备）

空历史、长历史（>200 条可见消息）、损坏尾行、缺失 sidecar、**相同内容不同 ID**、
同名复制、路径被移动、导入中断、带工作区、不带工作区。

### 9.2 SQLite

重复扫描、事务中途失败、WAL 崩溃恢复、`quick_check` 失败、数据库被替换成
future schema、备份恢复、同路径冲突、alias 冲突。**任何错误都不得改写 JSONL**。

### 9.3 Bridge / host

同一 ID 的打开、切换、重命名、删除；删除被拒（运行中）；原有客户端与新可选字段
双向兼容；host 与身份库对同一会话的路径判断一致。

### 9.4 记忆（阶段 3）

项目与全局隔离、手工编辑后重新索引、检索结果去重、`session-context` 注入预算与时间顺序。

## 10. 待评审决策（请评审者明确表态）

**编号约定**：下面 V1–V6 是**本方案新增**的问题，与 APPROVAL 稿的 Q1–Q8 不重叠；
APPROVAL 的 Q1–Q8 仍按其原编号评审，两者的对齐见第 13 节。请不要把 V 编号与 Q 编号混读。

| # | 问题 | 本方案倾向 |
| --- | --- | --- |
| **V1** | **`session_dir` 存进库，还是只存文件名、路径由调用方提供？** 存 dir 可校验漂移，但 Preview 换数据根就要整体 move | 存 dir + 显式 Move（第 5、6.3 节） |
| **V2** | **`position` 方向**：现在是"越小越新"（对齐 `workbench_catalog`）。是否改为"越大越新"以符合直觉？ | 保持现状，但必须在 store 注释里写明方向 |
| **V3** | **导入失败策略**：单条路径不符就整体失败，还是跳过该条并记录？ | 整体失败：宁可挡住，也不要静默丢身份 |
| **V4** | **阶段 2 的 host 回退窗口**：`workbench-sessions.json` 保留多久、以什么条件判定可停止读取？ | 要求"最近列表 diff 为零"持续一个完整发布周期后再停 |
| **V5** | **是否要求 sidecar 关闭时校验身份行与磁盘一致**（只报告、不自动修复）？ | 要求，作为阶段 2 的退出条件之一 |
| **V6** | **孤儿扫描的范围**：只扫 `sessions/` 一层（本方案），还是也递归子目录（如旧的 `projects/<slug>/sessions/`）？ | 只扫一层；若真实数据里存在子目录会话，则说明第 2.2 节的核对不完整，需先改基线 |

对照 APPROVAL 的对应关系：V1↔Q2（schema）、V2 无对应、V3 无对应（APPROVAL 未表态）、
V4↔B1 收尾条件、V5 无对应、V6↔Q3（孤儿处理）。

## 11. 风险

| 风险 | 控制 |
| --- | --- |
| 两个宿主同写一个 profile | 单实例锁（阶段 2）；身份库在 state home，不进 cache |
| 身份库损坏 | 备份策略针对实际 Preview 数据根；**不在 WAL 活跃时复制单个 .sqlite**，用 backup API 或停机后复制，并演练恢复 |
| 路径再次漂移 | 第 4.1 的唯一构造函数 + 第 4.3 的同源回归测试 |
| 把错误路径写进权威库 | 阶段 1 的只读核对清单；发现不符立即整体失败 |
| 误删用户数据 | 删除只走 `RemoveSessionArtifacts`；先文件后行；失败即回滚 |
| 静默丢身份 | 歧义一律不自动绑定，保留旧记录为 missing 并请用户选择 |

## 12. 复核指引（给评审者）

要证伪本方案，请依次检查：

1. `grep -rn "bridgeSessionPath\|\"tauri-\"" cmd internal` 是否确实只有第 2.1 节列出的两处构造点。
2. 在真实 Preview profile 下 `ls reasonix-core/`，确认没有 `projects/`。
3. 取 `workbench-sessions.json` 中任一带 `workspaceRoot` 的条目，确认其 transcript
   在 `reasonix-core/sessions/` 而不是 `projects/...`。
4. 读 `internal/config/paths.go:146-151,429-435,483-488` 与
   `internal/boot/boot.go:569-571`，确认 session dir 与身份库路径的推导。
5. 读 `internal/sessionidentity/catalog.go:47-60` 与 `store_test.go:167-208`，确认
   缺陷与"测试把缺陷写成期望"这一点。
6. 对第 10 节的 V1–V6 与 APPROVAL 稿的 Q1–Q8 分别给出明确选择，而不是"视情况而定"。
7. 对第 13 节的决议索引逐条核对：V2 与 APPROVAL 的差异是否都被显式声明。
8. 对第 13.5 节核对本方案被 V3 修正的 3 条错误是否已真正改掉（不是只在索引里认错）：
   `grep -n "internal/config" tools/repolint/layers.go`（应无 `leaves` 命中）、
   `grep -n "func BranchID" -A 9 internal/agent/branch.go`（确认 ID 恒等于文件名）、
   `sed -n '60,70p' cmd/reasonix-desktop-bridge/core_runtime.go`（确认 `ErrNotExist →
   SetFreshSessionPath` 的静默空会话路径）。
9. 对第 14 节核对 MiMo 引用：表内每个行号都可以直接 `sed -n` 打开。

## 13. 决议索引与对 APPROVAL 稿的合并


本方案与 [MIMO_STYLE_SESSION_STORAGE_APPROVAL.md](./MIMO_STYLE_SESSION_STORAGE_APPROVAL.md)
是并行产出的两份稿。为避免评审者读到两套结论，此处显式对齐：**编号沿用 APPROVAL 稿**，
本方案只声明"采纳 / 覆盖 / 补充"，不另起一套 A/B/C。

### 13.1 直接采纳（不再复述理由）

A1–A7、B3、B4、B5、C1–C5 **全部采纳**，内容与本方案第 3、7、9 节一致。特别注意：

- **A7 记忆层**：采用 MiMo 的 `memory/projects|sessions|global` 树与本方案阶段 3 不冲突；
  但"已有 `REASONIX.md` 不迁移、双轨写入需 RFC"这一限定**必须保留**（本方案阶段 3 未展开，
  以 APPROVAL 为准）。
- **C5**：与第 7 节最后一句同义——阶段 4 之前禁止对外宣称完成 SQLite 迁移。

### 13.2 本方案覆盖 APPROVAL 的地方

| 编号 | APPROVAL 稿 | 本方案 | 依据 |
| --- | --- | --- | --- |
| **B2** | "阶段 2 前冻结 alias schema"，倾向但未定死 | **阶段 1 就带 alias 表**（第 5 节） | 没有 alias，`ErrPathChanged` 是死路；且现在还没有任何已导入数据，改 schema 零成本 |
| Q2 | 二选一 | 选**独立 `path_alias` 表**（第 5 节） | 支持显式移动与审计 |
| Q3 | 倾向"给孤儿分配新 ID + 标记 `imported_from_scan`" | **改为确定性推导，拒绝分配新 ID**（6.6 第 2–3 条） | 随机/新分配 ID 会让"重扫幂等"与"ID 不变"互相矛盾；确定性推导同时满足两者 |
| Q7 | 主路径备份，次路径允许新 ID | 同意，但**必须把"可能不再拥有原 ID"写进用户可见提示**（3.2） | 避免把恢复说成无损 |

### 13.3 本方案补充 APPROVAL 的地方

| 缺口 | 补充位置 |
| --- | --- |
| APPROVAL 的分歧表把"JSONL 扫描"列为待办，但没有规则 | 6.6 节：扫描范围、ID 推导、往返校验、只读 SHA-256、未认领清单 |
| APPROVAL 的边界图画的是 `current_path UNIQUE` | 第 5 节：唯一性落在推导键 `(session_dir, transcript_name)`；**APPROVAL 第 5 节的图需要按此更新** |
| APPROVAL 未定义"什么证据才算同一会话" | 6.4 节 |
| APPROVAL 未规定导入清单的内容 | 7.1 节（含"未认领文件"专节） |
| MiMo 源码对照只在 APPROVAL 第 2.1 节出现 | 本方案第 1 节非目标直接引用 `MIMO_SOURCE_COMPARISON.md` 作为依据，避免评审只读 V2 时误以为"我们要把消息搬进 SQLite" |
| 上游 1.38.3→1.38.10 的规模数据 | 本方案不重复；引用 APPROVAL §1.2 与已有评审第 75 行的**同一限定**：数字只说明变更规模与修复密度，不能单独证明某架构是崩溃原因 |

### 13.4 仍需第三方裁定的合并冲突

1. ~~**B2 时机**：本方案主张阶段 1 带 alias~~ → **§13.5 已按 V3 闭合**：撤回该主张，
   alias 与 Move 一起在阶段 2 设计，阶段 1 保留 `ErrPathChanged` 作为防线。
2. **Q3**：本方案主张确定性推导 + 拒绝未认领文件；V3 主张只列**候选清单**、不自动认领。
   两者都不自动绑定 ID，差异只在措辞与实现细节；**建议按 V3 的"候选清单"表述**，
   避免"推导 ID"被误读成已认领。请评审确认这一合并表述。
3. ~~**schema 等价性**：`(session_dir, transcript_name) UNIQUE`~~ → **已由 §13.5 取代**：
   schema 改为 `relative_path TEXT NOT NULL UNIQUE`（V3 §3.2），不再需要这条等价性论证。

### 13.5 与 V3（`SESSION_STORAGE_IMPLEMENTATION_V3.md`）的对齐

V3 是并行产出的实施稿。它的**事实核查部分有 3 条直接命中本方案的错误**，我逐条验证后
按下表修正；它的**设计部分有 3 处比本方案更好**，本方案采纳。

#### 被 V3 修正的本方案错误（我已核实）

| # | 本方案原文 | 核实结果 | 处理 |
| --- | --- | --- | --- |
| 1 | "`internal/store` 与 `internal/config` **都**在 `leaves` 里" | **错**。`leaves` 不含 `internal/config`（V3 §2.5 正确） | §4.1 已更正，结论不变（新包仍需零内核依赖），但换掉了论据 |
| 2 | 把导入器的 `projects/<slug>/sessions/` 说成"凭空推导" | **措辞过头**。`config.ProjectSessionDir()`（`paths.go:451-462`）真实存在且在 CLI/desktop/bot 使用 | §4.2 已重写：缺陷是"按 workspaceRoot 自行推导"而非"目录不存在"；修正原则改为**导入器接收调用方传入的实际 session dir** |
| 3 | §6.4 第 1 条把 `.jsonl.meta` 的 ID 当独立证据 | **不成立**。`BranchMeta.ID` 恒等于文件名（`branch.go:186-195`、`223/321/409/596/602`），是循环证据 | §6.4 已重写为"阶段 1 不做自动重连" |

#### V3 顺带发现、本方案遗漏的一条真实缺陷（已核实）

`resumeBridgeSession`（`cmd/reasonix-desktop-bridge/core_runtime.go:65-68`）在
`agent.LoadSession` 返回 `os.ErrNotExist` 时**无条件**调用
`controller.SetFreshSessionPath(path)`。后果：**已登记且曾经落盘的会话，其 transcript
被删/移走后，再打开会静默变成一个同 ID 的空会话**，用户看到的是"历史凭空消失"。
这与 V3 §3.2 主张的生命周期状态机（`reserved/ready/missing/deleting/deleted`）是同一个
问题；身份库一旦记录 `missing`，就必须在这里拦住"新建空会话"分支。
本方案 §6.5 的 `missing` 语义因此**升级为阻断条件**，不是展示标记。

#### 采纳 V3 的 3 处改进

1. **`relative_path TEXT NOT NULL UNIQUE` 取代 `session_dir + transcript_name`**（V3 §3.2）。
   相对 state root 存路径，整份 profile 搬到别的绝对位置时**不需要批量改库**；
   而本方案的绝对 `session_dir` 必须整体 move。V3 的约束强度不变（仍 UNIQUE），
   可移植性更好。**本方案接受这一取代**，相应地把"相对路径的逃逸校验"
   （拒绝 `..`、绝对路径、符号链接逃逸）列入 §4.1 的构造函数职责。
2. **生命周期状态机 + `deleted` tombstone**（V3 §3.2、§5 S2.3）。文件系统与 SQLite
   无法组成一个原子事务，V3 的"持久状态机 + 重启重试未完成清理 + 禁止 ID 重用"
   比本方案"先删文件再删行"更强。本方案接受：删除 = `BeginDelete`（阻断新写）→
   调用 `control.RemoveSessionArtifacts` → `FinishDelete` 留 tombstone。
3. **alias 延后到 Move 语义确定**（V3 §3.3）。V3 指出要先定义 Move 是"认领已搬好的
   bundle"还是"由应用搬整个 bundle"，两者失败恢复不同；在此之前 alias 无用。
   **本方案撤回 §13.2 中"阶段 1 就带 alias"的主张**，改为：阶段 1 保留
   `ErrPathChanged` 作为防线，alias 与 Move 一起在阶段 2 设计（对应待裁定项
   13.4 第 1 条就此闭合）。

#### 两份稿仍未闭合的分歧

| 点 | V2（本方案） | V3 | 需裁定 |
| --- | --- | --- | --- |
| 未登记文件的处理 | 确定性推导 ID（文件名逆运算）+ 拒绝无法推导者 | 只列**候选清单**，不自动分配、也不自动认领 | 实质相同（都不自动绑定），措辞需统一；建议按 V3 的"候选清单"表述，因为"推导 ID"容易被误读成已认领 |
| 导入失败策略 | 路径冲突整体失败；单条坏记录跳过并报错 | 任何路径冲突或越界都使该批次失败 | 一致，合并表述 |
| `busy_timeout` 时机 | V2 修订后与 V3 一致（先装 busy handler 再取锁） | 相同 | 已收敛 |
| 状态机列名 | `missing INTEGER` | `state TEXT CHECK(...)` | 采纳 V3，见上 |


## 14. MiMo 源码复核（本轮新增，逐条对应代码）

本节是**直接读源码**得到的结论，不是对既有文档摘要的转述。凡与 `APPROVAL` §2.1
或 `MIMO_SOURCE_COMPARISON.md` 不一致处，以本节为准并已在上文合并。

### 14.1 核实到的事实与出处

| 事实 | 出处（`packages/opencode/src/`） |
| --- | --- |
| `session` 表无 transcript 路径列；`directory` 是工作区；`project_id` 外键 + `ON DELETE CASCADE` | `session/session.sql.ts:19-63` |
| 级联链 `session → message → part`，另有 `todo`、`session_prefix_snapshot` 同样级联 | 同上 |
| 会话 ID = `ses_` + 26 字符，**可排序**，分 ascending/descending 两个方向 | `id/id.ts:1-60`，`session/schema.ts:8-12` |
| ID **不是** UUID：v2 = 方向标记 + 16 hex 时间 + 9 base62；v1（遗留）= 12 hex 时间 + 14 base62 | `id/id.ts` 注释与 `generateID` |
| 格式换版时排序仍正确：`g` > `f`（升序 v2 排在所有 v1 之后）、`-` < `0`（降序反之） | `id/id.ts:24-29` |
| 迁移时 ID **取自文件路径 basename**，不读 JSON 内容字段；注释明确写了"早期迁移搬过目录但没更新 JSON" | `storage/json-migration.ts:169`（项目，`path.basename(projectFiles[...])`）、`203`（会话） |
| 迁移前**预扫描**全部文件，再在**一个事务**内批量写入 | `json-migration.ts:117-125`（预扫描）、`156`（BEGIN） |
| 每批用 `SAVEPOINT` + `onConflictDoNothing`，批失败只回滚该批并把错误计入 `errors[]`，整体仍 COMMIT | `json-migration.ts:102-112`（批）、`413`（COMMIT） |
| 父实体不存在的记录**跳过并计数**（`orphans.sessions/todos/permissions/shares` 打 warn） | `json-migration.ts` 各处 `orphans` 分支 |
| 迁移期临时把 `synchronous` 降到 OFF 提速 | `json-migration.ts:49-52` |
| 库路径按**安装通道**分文件（`mimocode-<channel>.db`，通道名做字符清洗），可用 flag 覆盖 | `storage/db.ts:31-44` |
| pragma 顺序：`busy_timeout` → `journal_mode=WAL` → `synchronous=NORMAL` → `cache_size` → `foreign_keys` → `wal_checkpoint(PASSIVE)`；注释写明"WAL 自身要拿锁，先装 busy handler" | `storage/db.ts:92-97` |
| schema 迁移是**带时间戳的目录 + `migration.sql`**，按时间排序后由 drizzle migrator 应用 | `storage/db.ts:62-71`（读迁移目录）、`100-117`（应用） |
| 标题可溯源：`title_source ∈ {fallback, generated, user}` + `title_revision`，并有 CAS 与"user 不可被生成覆盖""generated 不可退回 fallback"守卫 | `session/session.sql.ts:30-31`，`session/projectors.ts:95-99` |
| 会话归档是软删（`time_archived`），不删行 | `session/session.sql.ts:47` |
| 记忆是 Markdown 真源：`memory/{projects/<pid>/MEMORY.md, sessions/<sid>/checkpoint.md, global/MEMORY.md}` | `session/checkpoint-paths.ts:15-40` |
| 旧 `memory.md` → `MEMORY.md` 是一次性 rename，**容忍并发竞争者**（ENOENT 视为成功，其他错误抛出） | `session/checkpoint-paths.ts:48-60` |
| 记忆索引 `memory_fts(path UNIQUE, scope, scope_id, type, body, fingerprint, last_indexed_at)` | `memory/fts.sql.ts` |
| `fingerprint = size-mtime` **只用于跳过未变文件**（命中即 `hit`，不重写） | `memory/reconcile.ts:57-58` |
| 记忆的写入开关只在一个访问器里解释，业务代码不得直接读负向字段 | `memory/write-gate.ts` |
| 历史 FTS 有 versioned 迁移状态（clean/repair/done + cursor） | `history/fts.sql.ts`、`history/migration.ts` |

### 14.2 由此在本方案中修正或强化的条目

| # | 影响 | 落在哪 |
| --- | --- | --- |
| 1 | **推翻了我此前的一个判断**：APPROVAL §2.1 把 MiMo 会话 ID 写成"`ses_` + 26 字符可排序编码"，容易被读成与 UUID 无关；实际上它**同样是"时间前缀 + 随机后缀"**，只是长度和方向不同。因此"Reasonix 保留 `tauri-<uuid>`"不是"与 MiMo 背道而驰"，而是**同一族方案的不同参数**。这条现在是 Q1/A3 的直接依据 | 14.1 第 3–5 行 |
| 2 | **补 `title_source` / `title_revision`**（此前 schema 里没有，是真实缺口）：否则将来接入自动标题时，Agent 生成结果会覆盖用户手改的标题，且没有 CAS 可依 | §5、6.5 |
| 3 | **导入的事务边界照抄**：文件检查与候选收集必须在事务**之外**完成，事务内只做写入；否则一次读文件失败会把已经开始的写入留在中途 | §6.1、9.2 |
| 4 | **"跳过型容错"与"整体失败"必须分开写**：MiMo 对"父实体不存在的孤儿"是跳过 + 计数 + warn，对"批内某行写失败"是回滚该批 + 记 `errors[]` 后继续。本方案保留"**已登记 ID 的路径变了** → 整体失败"（数据完整性），但对"单条候选不可读/校验不过"改为**收集错误并按条报告，绝不静默丢弃** | §6.1、7.1 |
| 5 | **`Move` 的并发容错**：`MEMORY.md` 那次 rename 的写法（并发竞争者 ENOENT 视为成功）直接适用于本方案的 alias 写入与移动 | §6.3 |
| 6 | **schema 迁移用带时间戳的有序迁移目录**，而不是"`user_version` + Go 里写 DDL"。MiMo 的 `migration.sql` 目录 + 有序应用 + drizzle 版本表更可审计；本方案阶段 0 只有 v1，但**目录结构现在就按可演进设计**，避免把 DDL 写死在代码里 | §6.2 |
| 7 | **库文件按环境分文件名**（MiMo 用安装通道，清洗非法字符）。Reasonix 的 Preview 已是独立 state home，**不需要通道**，但需要一条明确规则防止 dev 与正式包互写：库名固定 `session-state-v1.sqlite`，路径来自 `DesktopSessionIdentityPath()`，不额外拼环境后缀 | §2.3、6.2 |
| 8 | **`fingerprint` 的定位再次确认**：MiMo 也只用它跳过未变文件，不用它判定身份。与本方案 §6.4 的禁止项一致，可作为评审时的外部佐证 | §6.4 |
| 9 | **`synchronous` 的取舍**：MiMo 用 `NORMAL`（他们的库是唯一权威，宁可承担回滚风险换取吞吐）。本方案的写入频率是"每次开/改/删会话一次"，**保留 `FULL`**；若评审要求对齐 MiMo，则必须同时接受"崩溃可能丢掉最近若干条身份行" | §6.2 |
| 10 | **软删 vs 硬删**：MiMo 有 `time_archived` 软删；本方案的删除已经走 `RemoveSessionArtifacts` 删文件，删行是硬删。若将来要"回收站"，需要独立的 `archived_at` 列与文件保留策略，**不在本次范围** | §6.5、非目标 |

