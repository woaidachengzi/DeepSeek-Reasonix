# 失败现场方法与窗口重排对照（2026-10-03）

同一签名私有诊断二进制 ebcf416b，固定两组各八次 Cocoa+WK/Tauri 交替运行，私有 HOME/TMP，本机 HTML，无产品配置或 sidecar。每次只调用一次 miniaturize，只有原生最小化状态及 will/did-mini 通知成立才请求恢复；失败不重试。收集器退出 0 仅表示收集完毕，失败个案 kernel exit 2/open exit 0。

loaded-reorder：Cocoa 4/4 通过，Tauri 3/4 通过。Tauri 第一次失败，loaded 时尚未 active/key，随后 minimize-ready 时已 active/key/visible，样式 32783、miniaturizable、屏幕存在，occlusionState 8194；请求后无 will/did-mini。三个方法在请求前后及最终采样均与 NSWindow implementation 相同。其余 Tauri 成功，不能归为方法覆盖或据此关闭间歇性失败。

no-reorder：唯一改变是禁止诊断的 loaded 后 orderOut/makeKeyAndOrderFront/activate 重排，Cocoa 4 次、Tauri 4 次均失败；每个 loadedReorderExecuted 为 false，active/key/visible 前提成立，三个方法比较仍相同。说明本轮失败也出现在独立 Cocoa NSWindow/WKWebView 中，不能限定为 TaoWindow 方法覆盖。两组运行按时间先后，环境变化仍是未控变量；不能声称重排是唯一原因或稳定修复，第一组也有失败。生产源码、真实包及验收断言未变。

新增采样只读取公开状态和方法 implementation 并比较布尔值，不调用指针或修改 runtime。初次编译使用不存在的 getter 失败，记录保留；改为公开 styleMask 的 Miniaturizable 位判断后，离线 locked release build/clippy -D warnings 均通过。完整源码、原始日志、内核退出回执及有效 entitlements 已保存。当前生产候选 60076bd5 的窗口完整门禁失败仍有效，D 未通过，E 未开启。后续需继续检查窗口呈现/激活和原生事件生命周期，不能用重试或诊断正样本替代修复。
