# Wails API 与事件面盘点

## 结论

当前前端不是直接遍历调用 Wails binding：
`desktop/frontend/src/lib/bridge.ts` 已是主 React-to-Go adapter。

- `AppBindings` 本体声明 **466** 个方法；其组合的子 binding 接口另有 **79** 个
  方法，即至少 **545** 个 host API 入口。
- `bridge.ts` 通过 `app` Proxy 执行调用，并集中实现 `agent:event`、terminal、
  updater、runtime、tab、session 和 remote 等主要订阅。
- `bridge.ts` 是 Tauri adapter 的正确替换点；不应把 545 个方法逐一变成 Tauri
  command，也不应让 React 组件直接调用 Tauri `invoke`。

## 现有分层

```text
React components/hooks
       │
       ▼
lib/bridge.ts: AppBindings + app Proxy + event subscription helpers
       │                         │
       │                         └── browser mock
       ▼
window.go.main.App + window.runtime (Wails generated/runtime API)
       ▼
desktop.App (Go) + Go core
```

迁移后的形态应为：

```text
React components/hooks
       │
       ▼
desktop API contract (保留 app/event helper 的调用形状)
       ├── Wails adapter（稳定版与回归对照）
       └── Tauri adapter（invoke + bridge stream）
                 │
                 ├── Rust host：窗口、菜单、系统能力、sidecar 生命周期
                 └── Go bridge：会话、Agent、Provider、MCP、工具
```

## 按迁移顺序分组

| 组别 | 首轮处理 | 示例 |
| --- | --- | --- |
| A：PoC 必需 | 建立精简 bridge | health、session snapshot、open/create、submit、cancel、event subscription、shutdown。 |
| B：核心稳定性 | PoC 通过后 | settings/provider、会话历史、workspace、文件与 diff。当前 Tauri 已有 provider/历史/附件、逐层 workspace 文件引用、受限文件预览，以及 Git 与本轮 session checkpoint 变更/diff；会话回滚动作仍待后续切片。 |
| C：进程与工具 | 需单独生命周期测试 | shell/terminal、MCP、Browser、plugins、worktree。 |
| D：host 平台能力 | 由 Rust 实现，不进 Go core | 窗口、文件对话框、外部链接、菜单、托盘、通知、钥匙串。 |
| E：后置 | Preview 稳定后 | remote host、bot、updater、复杂管理页。 |

## 当前事件面

主 Agent 流为 `agent:event`。此外，前端消费了下列 Wails runtime 事件：

```text
InboxChanged                     agent:ready
app:open-settings                config:load-warnings
desktop:shell-status             history-index:changed-v1
project-tree:changed             project-tree:changed-v2
project-tree:runtime-changed     remote-tab:<tabId>:event
remote-tab:<tabId>:state         remote-tab:opened
remote-tab:updated               remote:forwards
remote:server                    remote:status
runtime-state:changed            runtime:rebuilt
session:active-version-changed   session:recovered
session:recovery-failed          tab:meta
terminal:exit                    terminal:output
topic:activation                 updater:progress
agent:event
```

Phase 2 bridge 只承诺 `agent:event` 的语义等价版本。其余事件按上述分组进入
版本化协议；没有对应协议前，Tauri UI 不显示或不启用相关功能，不能伪造本地状态。

## 尚存的直接 runtime 依赖

除 `bridge.ts` 外，少数前端模块仍直接使用 `window.runtime`（clipboard、主题、
文件拖拽、原生菜单、窗口状态、crash telemetry 和若干小型事件订阅）。这些应在
Phase 1 逐项归并到 desktop API 的 platform/event 子接口。此处不做机械替换，
以免在没有 Tauri host 实现时破坏 Wails 基线。

## 迁移守卫

1. 每一组必须先有 Wails adapter 测试，再增加 Tauri adapter 测试。
2. `app` 的调用点不直接感知 transport，不出现散落的 Tauri import。
3. 前端 reducer 按 `(session/tab, sequence)` 去重；不能假定 WebView 事件绝不重复。
4. 任何订阅断线都先获取权威 snapshot，不能凭内存事件重建会话。
