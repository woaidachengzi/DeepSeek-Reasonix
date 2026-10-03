# 普通 Cocoa 窗口与 Tauri 应用上下文对照（2026-10-02）

只扩展独立 `window-baseline` example，不修改生产 Preview；最新生产 host 仍为
上一批 `9d58aae0`。本批所有程序通过普通 LaunchServices 启动，使用新的私有 HOME，
不改系统设置、不读取产品配置、无 sidecar/外部请求，没有 UI 自动化或用户默认档案操作。

## 结果

| 样本 | 前提 | 最小化/恢复 | kernel 退出码 |
| --- | --- | --- | --- |
| 普通 NSWindow + Tauri 应用/事件循环，首轮 | active/key 不满足 | 不解释此结果 | runner 拒绝前提，无已保存 kernel 回执 |
| 同上，Ready 后显式激活测试窗口 | active/key/visible 满足；隐藏 Tauri 页已 Finished | 失败；native minimized 始终 false | 2 |
| 原独立 Cocoa/WKWebView，当前复跑 | active/key/页面完成满足 | Will/Did Mini 与 Did Demini 齐全，通过 | 0 |
| 独立 Cocoa/WKWebView，minos 11.0/有效 entitlement 空 | active/key/页面完成满足 | 同上，通过 | 0 |
| 独立 Cocoa/WKWebView，minos 11.0/有效 entitlement 与生产相同 | active/key/页面完成满足 | 同上，通过 | 0 |

普通 Cocoa 的 NSWindow 没有 Tauri 窗口子类或窗口 delegate，也没有 WKWebView 内容；
另有隐藏 Tauri WebView维持相同事件循环，其 Finished 不代表 Cocoa 有页面。
所有状态在主线程读取。该样本仍复现失败，故复现不需要 Tauri 窗口子类/窗口 delegate；
当前独立 Cocoa 正对照通过，不能解释为当前系统完全无法最小化。
应用 delegate、NSApplication 子类、操作执行位置/时机、其他窗口和宿主上下文仍有差异，
尚未证明哪项是根因。没有提出或实施绕过正常最小化行为的生产补丁。

首轮 Cocoa 在 setup 中 makeKey，进入事件循环后 active/key 未满足；runner 严格拒绝。
修正夹具在 Ready 后主线程显式 makeKey/activate，再检查原前提，不放宽条件。
最终 Tauri-loop binary `6c967f456ad533186248d32641e44ececa3c12fbdbc9ac7d6ea4776edad1fc0c`，
私有 app `/private/tmp/reasonix-tauri-window-baseline-w45pq__8/Reasonix Window Baseline.app`，PID 80139。
example release build 与 Clippy `-D warnings` 通过，诊断结果及失败码 2 保留。

原独立 Cocoa app 的旧 CDHash 先严格核对，当前复制复跑二进制 SHA256
`121191074aa3093d282e5e774c077cfae264b306b0248d8abf046caeade5e569`，PID 80246，
当前真实最小化 892ms、恢复通过 1556ms，退出码 0。与旧签名身份一致。

Mach-O 核对发现原独立 Cocoa 的 minos=18.0，而 Tauri/生产 minos=11.0，SDK 均 15.2。
为控制此差异，从保存的同一 Swift 源以 `-target arm64-apple-macos11.0` 重编，
实际 Mach-O minos=11.0/SDK=15.2。首个 min11 样本曾对二进制使用生产
`Entitlements.plist` 签名，但外层 app 重签后有效 entitlement 为空；原始
`effective-entitlements.json` 保留此差异，不把传入签名命令当作有效签名证据。
新 SHA256 `fba4df08a5ba504d4504592c1238d34bcf8162c3e561c9132b367aeea010ed3f`，
私有 root `/private/tmp/reasonix-cocoa-current-control-rsg4nogp`，PID 80420，
真实最小化 917ms、恢复通过 1588ms、kernel/open 均 0。
随后对外层 app 使用生产 entitlement 重签，解析最终 plist 与生产逐项相等后再复制复跑，
见 `effective-entitlements-after.json` 与实际复制包的核对。最终 SHA256
`9a4eb82f1bb645a2ee5ab951232df944a72be626be656ea393c0f3adb40a666e`，
root `/private/tmp/reasonix-cocoa-current-control-qiifbu1x`，PID 80585，最小化 838ms、
恢复通过 1500ms、kernel/open 均 0。部署版本与最终有效 entitlement 对齐后仍通过，
它们无法单独解释本批差异。Tauri-loop 独立夹具的有效 entitlement 也为空，其失败
不依赖生产 entitlement；这些结果不能替代生产实际窗口验收。
首轮 Swift 编译的实际首错是默认 ModuleCache 写入被沙箱拒绝，
改成私有 `-module-cache-path` 后编译通过；没有安装/升级 SDK，也不将连带 SDK 报错当成根因。

本目录保留原始日志、状态、回执、脚本与源文件。下一步检查在 Tauri 应用中经 AppKit
原生调度执行动作的对照，区别代理回调位置与应用上下文。D 未验收，E 未推进；
实际最小化/恢复、完整全屏及其他 D/A/B/C 发布缺口继续保留，不发布、不切换默认下载。
