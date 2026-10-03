# 最小化方法实现只读对照（2026-10-03）

前一轮同一签名诊断二进制 520c853b：Cocoa+WK 两次通过，Tauri overlay 首次失败、第二次通过。完整原始记录在 prior-pairs；每轮 loaded/key 前提成立，原生状态和通知决定结果。失败退出 2 是功能失败，收集器退出 0 不代表全部通过。

私有 example 新增只读 Objective-C 方法实现比较：miniaturize:、performMiniaturize:、deminiaturize: 与 NSWindow 的 implementation 地址以 fn_addr_eq 比较，只记录布尔值。未调用实现指针、未修改 runtime、未移除代理、未改生产入口或放宽验收。

离线 locked release build 与 clippy -D warnings 均退出 0。新签名诊断二进制 4f1a6ac3 四次 Cocoa/Tauri 交替通过最小化/恢复，每次 kernel/open 退出 0；两种动态 KVO 类三个方法均与 NSWindow 相同。记录在 method-pairs。它说明本轮成功路径未覆盖这三个方法，不能证明失败时同样状态，也不能认定新增只读采样解决了间歇性失败。后续应在失败现场采样；当前真实候选 60076bd5 的完整窗口门禁失败继续有效。

所有窗口使用私有 HOME/TMP、固定本机 HTML、无 sidecar 和产品配置；有效 entitlements 与当前生产包一致。截图浏览器人工确认已经单独保存，不扩大为窗口或完整 D 验收。D 尚未完成，E 尚未开启。
