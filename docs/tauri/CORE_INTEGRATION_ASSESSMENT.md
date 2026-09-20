# Go Core 集成评估

## 结论

1.38.3 的 Go core 已有可复用的前端无关边界：

```text
boot.Build(options) → control.Controller
```

`boot.Build` 负责按现有 config、Provider、工具、MCP、skills 与 session 规则组装
Controller；Controller 已拥有 `Submit`、`Cancel`、`History`、`Snapshot`、
`SessionPath` 和 `Close` 等生命周期操作。因此 Tauri bridge 不应重写 Agent loop
或自行加载 Provider 配置。

## 可复用但不可直接暴露的模块

`internal/serve` 已证明 Controller 可通过 HTTP/SSE 被驱动，并提供
`serve.Broadcaster` 作为 event.Sink。但它是面向浏览器 Web UI 的完整服务器：

- 它注册了远超 PoC 所需的大量路由；
- 它的认证、CSRF、host guard 与桌面 token 生命周期不同；
- Broadcaster 对慢订阅者会丢中间帧，依赖 `/history` 恢复，未提供 bridge 所需的
  `sequence` / `resync_required` 合同；
- 它的前台/后台多会话和 lease 语义面向 Serve，而不是 Desktop tab owner。

所以 bridge **可以复用** Controller 构建、event 类型和部分广播实现，但不得把
`serve.Server.Handler()` 挂到 `/v1` 下，也不得让 Tauri 直接依赖 Serve 的 Web
路由。

## 下一代码边界

新增 `internal/desktopbridge`，由 `cmd/reasonix-desktop-bridge` 调用。包内应有：

```text
RuntimeManager        # session path → 唯一 Controller owner
EventLedger           # 单调 sequence、小型可重连缓存、snapshot fallback
Bridge API handlers   # 只实现协议 v1 的 open/snapshot/submit/cancel/events
```

生产 `RuntimeFactory` 实现位于 host 侧（`cmd/reasonix-desktop-bridge/core_runtime.go`），
不在 `internal/desktopbridge` 内：`repolint` 的 layering 规则只允许 `cli`、`serve`、
`acp`、`bot`、`botruntime`、`boot` 这几个 frontend 与 `cmd/`、`desktop/` 这两个 host
import `internal/control`，而 `internal/desktopbridge` 只保留与 core 无关的生命周期
原语。host 通过既有的 `RuntimeFactory` 接口注入自己的实现，测试继续注入
`RuntimeFactoryFunc` fake。

初版必须限制为一个本地 workspace、一个活跃 session owner；同一 session path 的
第二次 open 应返回已存在快照，而不是创建第二个 Controller。Controller 在 manager
关闭时先 `SnapshotForShutdown`，再 `Close`。跨实例锁、桌面 tabs、恢复旧会话和
多 workspace 留到这条受限路径经过回归验证之后。

## 验收先决条件

- `RuntimeManager` 的全部生命周期可用 fake factory 在无 Provider/网络条件下测试；
- 生产 `boot.Build` 路径只在明确的 `open_session` 后执行，bridge 启动/health 不读
  或迁移用户配置；
- 任何构建失败都以已脱敏错误返回，不能让 token、`.env`、prompt 或 provider key
  出现在 response 或日志；
- Agent event 先写入有界 ledger，再投递 SSE；客户端断线后必须能判断继续还是
  `resync_required`。
