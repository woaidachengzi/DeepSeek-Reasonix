# macOS ARM64 并行会话候选包 0.1.4

- 接管 App／DMG 从干净提交 `b07d8fe7f` 构建，应用身份 `io.reasonix.desktop.takeover`。
- `takeover-package.json` 记录 DMG、宿主和 sidecar 的 SHA256。App 和只读挂载 DMG 内的 App 均通过 `codesign --verify --deep --strict`，两个二进制完全一致。
- `preview-native-parallel.json` 来自最新实现提交 `34eaf0261` 的隔离 Preview 包：两个真实 SSE 请求并行，后台快照、独立停止、单会话设置刷新、普通退出断开请求和 sidecar 清理通过。只使用私有 loopback 夹具，没有真实密钥／历史。
- 完整 Go race、Rust 320 项（6 项原有忽略）、Tauri 前端和 transcript 回归通过；Chrome 组件深浅主题渲染没有 overflow／console／框架错误。

安装包为本地 ad hoc 签名候选包，未 Apple 公证。已安装版本仍为 0.1.3；原资料及备份未改动。锁屏期间没有执行 0.1.4 接管 App 的人工原生界面验收，不将 Preview 自动化结果等同于原资料人工验收。

构建和验收步骤见[多会话改造记录](../../PARALLEL_SESSIONS.md)。
