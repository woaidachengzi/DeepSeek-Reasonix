# macOS ARM64 并行会话候选包 0.1.4

- 接管 App／DMG 从干净提交 `b07d8fe7f` 构建，应用身份 `io.reasonix.desktop.takeover`。
- `takeover-package.json` 记录 DMG、宿主和 sidecar 的 SHA256。App 和只读挂载 DMG 内的 App 均通过 `codesign --verify --deep --strict`，两个二进制完全一致。
- `preview-native-parallel.json` 来自最新实现提交 `34eaf0261` 的隔离 Preview 包：两个真实 SSE 请求并行，后台快照、独立停止、单会话设置刷新、普通退出断开请求和 sidecar 清理通过。只使用私有 loopback 夹具，没有真实密钥／历史。
- 完整 Go race、Rust 320 项（6 项原有忽略）、Tauri 前端和 transcript 回归通过；Chrome 组件深浅主题渲染没有 overflow／console／框架错误。

安装包为本地 ad hoc 签名候选包，未 Apple 公证。解锁后原生点击验收已完成，见 `native-ui-acceptance.json`：0.1.4 接管候选包打开原历史、跨历史切换及草稿隔离／恢复通过；私有 Preview 中通过原生控件启动双流、切换、单独停止、应用当前设置及正常退出，两个连接均清理。没有向模型发送原历史。Applications 中的已安装副本仍为 0.1.3，接管候选包从构建目录运行；备份保留。

构建和验收步骤见[多会话改造记录](../../PARALLEL_SESSIONS.md)。
