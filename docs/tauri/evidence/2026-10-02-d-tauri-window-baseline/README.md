# 同依赖最小 Tauri 窗口对照（2026-10-02）

新增独立 cargo example `desktop/tauri/examples/window-baseline.rs`，不进入生产 main。
使用当前 Cargo.lock 的 Tauri 2.11.6/Tao 0.35.3/Wry、唯一 baseline bundle ID、
固定本地 HTML 和独立私有 HOME。没有 sidecar、产品页面、单实例插件、托盘、
偏好/窗口状态读取或业务恢复逻辑，不修改系统设置。生成 app 严格 ad-hoc 签名核对通过。
本轮没有重新构建生产 Preview，生产包仍是上一批 `9d58aae0`。

两种标题栏（默认普通栏、Overlay+hiddenTitle）均在主线程调用实际
`NSWindow.miniaturize`，随后读取 Tauri/native 状态，每 250ms 一次，约六秒。
调用前明确要求页面 Finished、applicationActive、native key 和 native visible
全部为 true。两个最终样本满足前提，但请求后 key 状态退出，原生和 Tauri
minimized 均一直为 false，未发生可验的最小化/恢复。这是诊断失败，不是验收通过。
普通栏结果排除 Overlay 为该样本的必要条件；业务页面/sidecar/恢复路径也不是
该样本复现的必要条件。不能据此认定某个框架缺陷或系统设置为根因。

最终 baseline binary SHA256 `088a8e79fffdc8cd2ff0bafc308e263a1a2b36acf312dc5416397b3b64876f95`。
最终私有 app `/private/tmp/reasonix-tauri-window-baseline-v84e53si/Reasonix Window Baseline.app`。
两个样本 PID 79676/79695 均有 kernel 正常退出码 **2**；open 返回 0。
诊断结果失败与正常宿主 Quit=0 是不同验收范围，未替代上一批实际 Cmd+Q 结果。

首轮和追加回执样本也复现状态失败；夹具最初调用 `AppHandle.exit(2)` 却取得
kernel 0，runner 因回执不符停止，没有运行 Overlay。保留日志，未改成把 0
计为成功。检查当前本地 tauri-runtime-wry 的 RequestExit 分支设置
`ControlFlow::Exit`，Tao run 直接退出，不把请求的 2 自动传播到 kernel。
因此独立夹具改用 `run_return`，事件循环完成清理后显式返回诊断结果的进程码，
最终两样本 receipt=2，runner=0 表示两份诊断结果已完整核对，不表示窗口通过。
没有修改生产退出策略；已有生产故障门禁须继续以错误 marker 与 kernel 回执共同判断。

`cargo build --locked --release --example window-baseline` 和定向 Clippy `-D warnings`
通过；诊断源和固定启动脚本、原始结果/日志及退出回执保存于本目录。此前 Cocoa/WKWebView
对照曾通过最小化，但它不使用 Tao 事件循环/窗口 delegate；下一步可在相同 Tauri
上下文核对普通 Cocoa 窗口，继续区分窗口子类/delegate 与应用/事件循环。D 未验收，
E 未启动，A/B/C 与其余发布缺口继续保留，没有发布或切换下载项。
