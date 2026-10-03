# Cocoa 源码与启动身份复核

三组均在隔离 HOME/TMPDIR 下顺序运行，strict ad-hoc 签名、有效 entitlements 与当前生产包相等。源码/构建/runner/完整事件/确切 PID kernel exit 全部保存。

| 样本 | 条件 | 结果 |
| --- | --- | --- |
| identical | 完全相同的已通过 Swift/WKWebView 源码，新名称与 bundle ID | failed-key，kernel1/open0；未满足激活/key 前提，不能记为有效最小化失败 |
| identity | 相同已通过源码，新构建，采用原通过样本名称和 bundle ID | kernel0/open0；激活/key、will-mini/did-mini/did-demini 均成立，通过 |
| empty-identity | 先前空窗口固定节奏源码，采用通过样本名称和 bundle ID | precondition=false，kernel2/open0；不能记为有效最小化因果样本 |

部署 minos11.0/SDK15.2 与签名 flags=adhoc 经核对与原通过样本一致。新编译二进制也可通过，原二进制本身并非唯一通过条件。然而本组激活前提不稳定，不能认定 bundle ID 是根因，亦不回填先前失败或将诊断身份用于生产修复。

下一步将 Cocoa/WKWebView 的页面完成后 orderOut/show 条件直接加入 Tauri 私有 example 对照。D 最小化/全屏与实际 UI 验收仍未通过；生产行为未改。
