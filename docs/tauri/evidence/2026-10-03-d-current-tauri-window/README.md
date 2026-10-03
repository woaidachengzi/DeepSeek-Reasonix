# 当前环境 Tauri 事件循环窗口复验

当前锁定依赖的独立 example，固定本机页面，无产品设置和 sidecar。离线 release 构建通过；私有包 ad-hoc strict 验证，有效 entitlements 与生产包一致。保留 TaoApp 与应用 delegate，独立 Cocoa NSWindow 在 Tauri 事件循环内执行原生调用。

二进制 `3a04b1f43b9143e2db4a0ee3c0f3fdfa5840161cdbde28eb58b56e7a7b8638b9`；确切 PID 14664。24 次采样，激活/key/visible/pageFinished 前提成立，miniaturize/deminiaturize 各执行一次；nativeMiniaturized 始终 false，minimized/restored 均 false。kernel exit 2、open exit 0。Runner exit 0 只表示成功采集并校验预期失败回执，不能记为功能通过。

同轮[独立 Cocoa 对照](../2026-10-03-d-current-cocoa-activation/README.md)通过。当前有效失败继续将调查收敛到 Tauri 事件循环与原生 AppKit 交互；尚未证明具体根因，也未修改生产窗口代码。全屏/实际快捷键/多显示器验收仍未完成。
