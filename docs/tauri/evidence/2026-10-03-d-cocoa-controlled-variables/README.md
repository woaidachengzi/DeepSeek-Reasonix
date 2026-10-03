# Cocoa 变量控制

保留全部通过/失败，均为隔离 HOME/TMPDIR 私有应用、固定本机内容、ad-hoc strict 签名，构建验证有效 entitlements 与当前生产包一致。以确切 PID 的 kernel exit 为准；LaunchServices open exit 0 不代表功能成功。见 summary.json 与各样本源码/build/runner/原生日志。

| 样本 | 内容/调度差异 | 前提与结果 |
| --- | --- | --- |
| empty | 无 WKWebView，1280×820；每 250ms 由 worker 调度 main，step4/14 mini/demini | 激活/key/visible 前提成立；kernel2，最小化/恢复失败 |
| web | 仅增加 WKWebView 固定 HTML，其他与 empty 相同；未等待 didFinish | 同上；kernel2，失败 |
| timer | 仅将 empty 调度改为 run-loop Timer，其他不变 | 同上；kernel2，失败 |
| reorder | 仅在 empty step0 增加 orderOut 再 show | 激活前提失败，kernel2；不能作有效最小化因果样本 |
| size | 仅将 empty 改为1000×700 | 前提成立；kernel2，失败 |
| phase | 从原通过 Swift 源码移除 WKWebView，合成 loaded；保留50ms阶段式Timer/orderOut-show | active/key 后执行 mini，未见 will/did-mini；kernel1 failed-mini |
| positive-repeat | 先前已通过的 b88de85c 二进制保持不变 | kernel0/open0；will-mini/did-mini/did-demini 全部收到，通过 |

结论：普通 NSApplication 的空窗口与 Tauri 诊断可复现相同失败，不能将此前对照直接归因于 Tauri 事件循环。单独加 WKWebView、改变 Timer 调度或尺寸不足以解除失败。通过样本仍包含 WKWebView didFinish 后 orderOut/show 与阶段式调度的组合；下一步控制页面完成后显示的条件。尚无生产修复，不视为 D 验收通过。
