# NSApplication 子类对照（2026-10-03）

重新建立完整 macOS D→E 目标后，继续定位窗口失败。本批仅改独立 window-baseline example，
生产 host 仍为 15fcd43b；未修改默认档案、系统设置、剪贴板，未发布。

Tao 0.35.3 event_loop.rs 默认先创建 TaoApp sharedApplication，再安装 TaoAppDelegate。
example 增加显式 `REASONIX_WINDOW_BASELINE_APP=plain`，在 Builder 前创建普通
NSApplication 单例；Tao 的 delegate 和事件循环仍保留。这个初始化次序是诊断控制，
不是受支持的生产策略，不应直接移入 main.rs。

同签名二进制、有效 entitlement 与现存生产包逐项一致，两份新私有 HOME；通过
LaunchServices 启动，读取主线程原生状态与确切 PID 的 kqueue kernel 回执。
普通 Cocoa NSWindow 不使用 Tauri 窗口 delegate/子类，无 WebView 内容；隐藏固定 Tauri
页面的 Finished 仅作为相同宿主的就绪条件。

| 样本 | 实际应用类 | active/key/visible/page 前提 | mini/restore 请求执行 | 原生最小化/恢复 | kernel/open |
| --- | --- | --- | --- | --- | --- |
| 默认，PID 5287 | TaoApp（每个快照核对） | 全满足 | 1/1 | 失败/失败 | 2/0 |
| plain，PID 5307 | NSApplication（每个快照核对） | 全满足 | 1/1 | 失败/失败 | 2/0 |

release build 与 example Clippy -D warnings 通过。runner=0 表示对照完整执行并核对了
预期失败码，**不是最小化验收通过**；两份原生结果均 false，原始回执保留。
确切 PID 已确认不存在，没有子进程或 sidecar。

二进制 SHA256 `2bfd03e6aa094cbaa21b52243527eb3c7b464548ef89bb2adba9c71c29bd2847`，
临时 app 位于 `/private/tmp/reasonix-tauri-window-baseline-7iy3q1jq/Reasonix Window Baseline.app`。

本批证明：TaoApp 子类并非普通 Cocoa 窗口在 Tao 上下文中失败的必要条件。
尚未区分应用 delegate、事件循环/调度及其他宿主上下文，不能宣称根因已定位。
下一步可在独立样本中控制 delegate，继续保留前提与原生断言；生产最小化/全屏退出、
其余 D 验收和 A/B/C 发布缺口继续保留，E 等待 D 稳定。
