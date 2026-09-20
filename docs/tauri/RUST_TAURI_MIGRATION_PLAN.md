# Reasonix Stable 1.38.3：Rust + Tauri 迁移计划

> 状态：设计计划，尚未开始实现。
>
> 基线：`desktop-v1.38.3` / `v1.38.3`，提交
> `fa018e4109268c912063c8cc619302fccdb57d74`。
>
> 目标：以 Tauri 替换 Wails 桌面壳，并在不破坏已有 Provider、会话、
> Agent、MCP、工具和用户数据的前提下，逐步引入 Rust。此计划**不**把
> “Tauri 化”误当作“立刻重写全部 Go core”。

## 1. 决策与非目标

### 已决定的方向

```text
React frontend（尽量复用）
            │
            ▼
Tauri 2 host（Rust：窗口、菜单、托盘、系统对话框、sidecar 生命周期）
            │
            ▼
Reasonix bridge protocol（版本化、本地私有）
            │
            ▼
Go core（先保持 1.38.3 的 Agent / Provider / Session / MCP / Tool 行为）
```

Tauri 的第一职责是替代 Wails host；Go core 保持运行。只有当 bridge 已稳定、
回归测试可证明行为一致后，才从边界清晰且无状态的模块开始用 Rust 替换。

### 本阶段不做

- 不删除 `desktop/` 或更改 `stable/1.38.3`。
- 不迁移、重写或静默转换已有会话、注册表、Provider 凭据。
- 不将现有 Wails 的所有导出方法逐个翻译成 Tauri command。
- 不实现自动更新、插件市场、浏览器 UI、远程主机管理的完整 Tauri 功能。
- 不在 Tauri PoC 期间同时升级 Go、Node、pnpm、React 或核心依赖。

## 2. 当前基线的架构事实

| 区域 | 1.38.3 现状 | 迁移含义 |
| --- | --- | --- |
| 桌面 host | Wails 2.13（`desktop/wails.json`） | 用 Tauri 2 替换，不直接复用 Wails 生命周期。 |
| 前端 | `desktop/frontend`，React/Vite/pnpm | 首轮继续复用，先在单一 API 适配层替换 Wails bindings。 |
| 后端 API | `desktop.App` 的大量 Wails-bound Go 方法 | 必须收敛为版本化 bridge 合同，不能复制为同样大的 Rust API。 |
| 事件 | Wails 事件通道 `agent:event`，并携带 tab/session 语义 | bridge 需要有序、可背压、可重连的流事件协议。 |
| 平台能力 | Wails runtime 调用窗口、对话框、外部 URL、托盘、更新器 | 各能力分批换为 Tauri plugin 或少量 Rust command。 |
| 核心状态 | Go 内部的 Agent、Session、Provider、MCP、Tool 与持久化实现 | 第一阶段继续由 Go 唯一拥有，禁止在 Rust 再造第二份状态。 |

## 3. 目标目录与分支

迁移在独立工作区、独立分支上进行：

```text
stable/1.38.3                 # 不改：可用回退基线
work/stable-hardening         # 只接受低风险稳定修复
experiment/tauri              # 仅 Tauri 迁移和 PoC
```

计划中的新增目录：

```text
desktop-tauri/
├── frontend/                 # 初期由 desktop/frontend 迁入或共享；不双份长期维护
├── src-tauri/
│   ├── src/
│   │   ├── main.rs
│   │   ├── bridge.rs
│   │   ├── sidecar.rs
│   │   └── platform/
│   ├── capabilities/
│   └── tauri.conf.json
└── package.json

cmd/reasonix-desktop-bridge/  # Go：唯一的稳定桌面 bridge 入口
docs/tauri/                   # 协议、兼容性和验收证据
```

`desktop-tauri/frontend` 可以在探索期引用现有前端源码，但在第一条可运行
路径确定后必须确定唯一前端目录，避免两个 React 应用长期漂移。

### 已落地的偏差（决策记录）

实现没有新建 `desktop-tauri/`，而是把 host 放在 `desktop/tauri/`，前端保持唯一的
`desktop/frontend/`，没有复制第二个 React 应用：

```text
desktop/frontend/                      # 唯一前端；Wails 与 Tauri 共用
desktop/tauri/                         # Tauri 2 host（Rust）
├── src/{main.rs,bridge.rs,protocol_generated.rs}
├── capabilities/main-window.json
└── tauri.conf.json
cmd/reasonix-desktop-bridge/           # Go：唯一的稳定桌面 bridge 入口
internal/desktopbridge/                # Go：runtime、ledger、protocolgen
docs/tauri/                            # 协议、兼容性和验收证据
```

这样"唯一前端目录"的要求无需额外收敛步骤即成立。代价是 host 位于即将被替换的
Wails Go module 内部：将来摘除 Wails 时，需要把 `desktop/tauri/` 与
`desktop/frontend/` 一起移出，而不能直接删除 `desktop/`。在计划第一次需要决定
"`desktop/` 是否整体退场"时重新评估这个取舍。

## 4. Bridge 合同：先设计，后编码

### 4.1 传输选择

首选：Go sidecar 仅监听 `127.0.0.1` 的随机端口；Tauri 启动时生成一次性的
高熵 token，通过继承环境变量或受限启动参数传给 sidecar。所有请求与流连接都
必须携带该 token。端口、token、提示词、API key 和会话内容不得写入日志。

原因：HTTP/streaming 对大量 Agent 事件、重连、诊断和跨平台调试都比自定义
stdin/stdout 多路复用更直观。若实现阶段证明 loopback transport 有安全或部署
限制，可改为 Unix domain socket / Windows named pipe，但必须保留同一协议语义。

### 4.2 合同原则

- 协议从 `v1` 开始，每个 request、response 和 event 都带 `protocolVersion`。
- 命令按用户意图分组，而不是按现有 Go 方法一对一镜像。
- 每个可变操作携带请求 ID；重试必须幂等或明确返回冲突。
- 每个事件携带单调 `sequence`、`sessionId`/`tabId`、事件种类和时间戳。
- sidecar 重启后前端通过 `GetSnapshot` 获取权威快照，再从最新 sequence 订阅；
  不靠重放任意内存事件恢复 UI。
- Go 是会话、Agent、Provider、MCP 和工具状态的唯一 owner；Rust 只保存 host
  自己的窗口、菜单与 sidecar 健康状态。
- JSON Schema 作为源文件，生成 TypeScript 与 Rust 数据类型；禁止手写三份
  可漂移的 DTO。

### 4.3 第一批能力

PoC 只定义以下能力：

```text
Health / protocol handshake
Create or open session
Get session snapshot and history page
Submit prompt
Cancel active turn
Subscribe to typed agent events
Graceful shutdown
```

Provider 设置、文件工具、MCP、终端、工作区、远程主机、更新器都在这些能力
通过端到端验收后再进入合同。

## 5. 分阶段执行

### Phase 0 — 冻结与可观测基线

1. 在 `stable/1.38.3` 记录版本、commit、Go/Node/pnpm/Wails、构建命令和产物。
2. 运行可在当前机器运行的 Go、前端、Desktop 测试，并记录受环境阻塞的项目，
   不以升级依赖“修复”它们。
3. 建立人工 smoke test：启动、Provider、单会话聊天、流式回复、中断、恢复、
   重启、工作区读写、MCP、设置保存。
4. 列出配置、会话、注册表、凭据、缓存、日志和窗口状态的数据位置与格式。
5. 为每个迁移候选动作先拍摄行为证据（输入、事件序列、持久化结果），而非只看
   UI 截图。

退出条件：原 Wails 程序的可运行范围、已知失败和数据契约已经写入文档；能够
随时从 `stable/1.38.3` 构建或检查基线。

### Phase 1 — API 面盘点与前端解耦

1. 从 Go bindings 与前端 import 生成 API 清单，按以下类别分组：
   session/agent、Provider/settings、workspace/git、terminal/shell、MCP、
   platform UI、updater、remote、diagnostics。
2. 在前端引入唯一的 `desktopApi` 接口和事件订阅接口；Wails adapter 是第一种
   实现。此次只做无行为变化的机械替换，并由类型检查和现有测试保护。
3. 将 `agent:event` 的 payload 类型、顺序语义、重连语义、错误语义写成
   `docs/tauri/BRIDGE_PROTOCOL.md`。
4. 标出每一个直接 Wails runtime 调用，并划分为 Rust host、Go bridge 或暂不支持。

退出条件：前端不再直接依赖生成的 Wails binding；不改变任何可见功能。

#### 已落地的偏差（决策记录）

没有新增 `desktopApi` 接口。`API_SURFACE_AUDIT.md` 盘点后确认
`desktop/frontend/src/lib/bridge.ts` 已经是唯一的 React-to-Go adapter —— 545 个
host 入口集中在 `app` Proxy 与事件订阅 helper 中。在它之上再包一层只增加转发
代码，不增加边界，因此不引入该层。

Tauri adapter 落在 `desktop/frontend/src/lib/tauriBridge.ts`，只覆盖已迁移的最小
命令面。代价是退出条件目前**未达成**：`bridge.ts` 仍 `import type` 生成的 Wails
类型，运行时仍经 `window.go.main.App`。该条件随功能面逐个迁移收敛，不是一次性
切换。

### Phase 2 — Go desktop bridge（不引入 Tauri UI）

1. 新增 `cmd/reasonix-desktop-bridge`，只包装最小会话/Agent 路径。
2. 复用既有 Go core；不得把 `desktop.App` 整体嵌入新服务，也不得复制 session
   生命周期实现。
3. 用协议契约测试覆盖 handshake、submit、流事件、cancel、错误、sidecar 退出、
   token 拒绝和协议版本不兼容。
4. 提供独立诊断命令，可启动 bridge、调用 health、打印已脱敏的版本和能力清单。

退出条件：不运行 Wails 时，桥接进程可稳定完成“建会话 → 发送 → 流式回复 →
中断 → 正常退出”。

### Phase 3 — Tauri 最小 PoC

1. 建立 Tauri 2 shell，Rust 负责启动、观察、停止和超时终止 Go sidecar。
2. Tauri 在崩溃、关闭和二次启动时清理子进程；绝不扫描并杀死名称相似的进程。
3. 前端改用 Tauri adapter，只接入 Phase 2 的最小 API。
4. 实现一个窗口、新建/打开会话、发送、流式显示、取消、历史恢复和明确错误页。
5. 默认不开启自动更新；About 页面显示 `Reasonix Stable`、基线 commit、Tauri
   版本、bridge 协议版本和 sidecar 版本。

退出条件：在连续使用 30 分钟、两次重启和一次非正常 sidecar 退出后，应用不会
留下孤儿进程，且会话不丢失。

### Phase 4 — 数据兼容与平台能力

1. 明确新应用标识与数据目录策略。默认禁止静默迁移；首次使用既有数据前必须
   创建时间戳备份并展示原目录、目标目录与回退方法。
2. 实现单实例锁，防止 Wails 与 Tauri 同时写入同一 Reasonix 数据目录。
3. 逐项替换：文件/目录选择、外部 URL、窗口状态、菜单、托盘、通知、钥匙串。
4. 这些能力进入 Rust command 时，使用最小 allowlist、能力权限和路径校验；
   前端不得获得任意 shell、文件系统或网络权限。
5. 再接入 Provider/settings、工作区文件与 diff、shell/terminal、MCP。

退出条件：核心 smoke test 在 Wails 与 Tauri 的同一份测试数据上均通过；任一
迁移失败都可以恢复到原始数据备份和 Wails 版本。

### Phase 5 — 发布候选与切换

1. 先发布标识明确的 `Reasonix Stable Tauri Preview`，与 Wails 稳定版并存。
2. 实现本地构建可复现性（lockfile、Rust toolchain、Go toolchain、Node/pnpm、
   sidecar checksum、commit、构建时间）。
3. CI 首轮只覆盖 macOS：Rust check/test、Go bridge test、frontend build、协议
   兼容测试、签名/打包 dry run。通过后再增加 Windows、Linux。
4. 收集崩溃、sidecar 不退出、会话恢复、Provider 凭据和 MCP 生命周期的脱敏诊断；
   没有明确同意不上传用户内容。
5. 连续通过回归与人工 smoke 后，才考虑把 Tauri 设为默认下载项；Wails 仍保留
   一个完整稳定周期作为回退。

## 6. Rust 化 Go core 的准入规则

Tauri host 完成不等于应重写 Go core。每个候选模块必须有：稳定边界、行为测试、
数据格式说明、可单独切换的 feature flag，以及可逆回退路径。

| 顺序 | 候选 | 前提 |
| --- | --- | --- |
| 1 | 纯配置校验、版本解析、路径与文件工具函数 | 无持久化 schema 变化，跨语言 golden tests 一致。 |
| 2 | host 专属能力：更新策略、平台集成、sidecar 监管 | 不改变 Go Agent 与 session 格式。 |
| 3 | 独立 HTTP client 或 provider 辅助层 | Provider 请求/错误/重试均有录制回放测试。 |
| 最后 | Agent runtime、Session engine、MCP、Shell、Browser、并发控制 | 仅当协议和持久化模型已稳定，且迁移可在模块级回退。 |

任何会话格式、注册表、历史回放、checkpoint 或凭据存储的 Rust 重写都需要单独
RFC 与真实用户数据的离线迁移演练，不属于本计划的默认范围。

## 7. 主要风险与控制

| 风险 | 控制方式 |
| --- | --- |
| Go/Wails API 数量大，前端迁移失控 | 先有单一 adapter；按能力 vertical slice 迁移。 |
| 流事件乱序、重复或丢失 | sequence、快照恢复、契约测试、前端幂等 reducer。 |
| 两个应用并写会话数据 | 单实例/目录锁；首次迁移备份；不静默复制或升级。 |
| sidecar 孤儿进程或关闭数据损坏 | 父子生命周期、graceful shutdown、超时后的 PID 精确清理、重启测试。 |
| Tauri 权限过宽 | capabilities 最小化，Rust 端校验所有敏感请求。 |
| Rust 重写改变 Agent 行为 | 延后 core 重写；对每个候选模块运行 golden / replay 测试。 |
| 自动更新覆盖 Stable | Preview 与 Stable 分离 channel；默认无自动下载/安装。 |

## 8. 首个可执行任务包

下一轮只做以下内容，禁止开始 Tauri UI 或 Rust core：

1. 创建 `experiment/tauri` 的独立 worktree。
2. 完成 Phase 0 的构建、测试、数据目录和 smoke-test 文档。
3. 盘点 Wails bindings、Wails runtime 调用和 `agent:event` 的实际消费者。
4. 新增前端 `desktopApi` 接口及 Wails adapter，确保行为和测试不变。
5. 编写 `BRIDGE_PROTOCOL.md` 和最小协议 JSON Schema，但不启动 sidecar。

交付物是架构和回归证据；不是一个半成品 Tauri 应用。
