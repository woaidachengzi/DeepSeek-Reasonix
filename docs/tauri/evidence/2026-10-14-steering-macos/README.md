# 运行中引导候选包 0.1.6

macOS ARM64 接管 App／DMG 从干净提交 `c8598c7b5` 构建。App 和只读挂载 DMG 内的 App 均通过严格签名核验；宿主和 sidecar 二进制一致，摘要见 `package.json`。构建的 hooks lint、类型检查、滚动写入和 bundle 大小门禁通过。

真实 Go 控制器验证工具边界应用引导、精确目标归属和重复请求仅应用一次；完整 bridge race、Rust 320 项（6 项原有忽略）和 Tauri 前端回归通过。新增闭合 intake 回归证明 UI 状态仍为运行时，拒绝的引导不会转为新的 Submit。组件补验失败保留草稿、附件保留，以及等待审批不被引导绕过。

现有 Playwright／Chrome 的真实 React 组件检查深浅主题、独立引导／停止布局、原回答保留及会话切换，无 console／框架错误。0.1.6 没有新增原生 App 人工点击验收记录；不把 0.1.4 旧包的记录改写为新版通过。当前包未替换 Applications 中的副本。

[功能与上游源码参考](../../SESSION_STEERING.md)。
