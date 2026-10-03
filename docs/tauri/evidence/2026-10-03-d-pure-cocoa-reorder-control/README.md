# 不含 Tauri/Tao 的 Cocoa 呈现对照（2026-10-03）

从已保存 Cocoa 源码建立独立 Swift/AppKit app，1280×820、非持久化 WKWebView、固定本机 HTML；没有 Tauri/Tao 事件循环、产品配置、sidecar 或网络。arm64 macOS minos11.0 编译通过，strict ad-hoc 签名及有效 entitlements 与当前生产包一致。每轮私有 HOME/TMP，LaunchServices 启动，精确 PID 的 kqueue 内核退出记录保存。

同一二进制固定四轮 loaded/none/loaded/none：加载后 orderOut/makeKeyAndOrderFront/activate 两轮均收到 will/did-mini、原生 minimized=true 和 did-demini，kernel0；禁止该重排的两轮均在已 active/key 的请求后未最小化，kernel1、failed-mini，open0。每轮只发一次请求，失败不重试；matrix0是收集有效回执，不是四轮通过。

本轮不含 Tauri/Tao 仍有失败，因此不能把全部失败归为它们的事件循环或方法覆盖。重排组在之前 Tauri 诊断也有间歇失败，不能作为稳定生产修复。该顺序对照仍有时间/系统环境未控变量；不能声称重排为唯一原因，亦没有权威证据把失败归因于屏幕捕获。后续须继续核对呈现生命周期和真实生产路径，并推进其余 D 验收。D 尚未通过，E 尚未开启。
