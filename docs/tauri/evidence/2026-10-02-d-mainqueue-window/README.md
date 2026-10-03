# 原生主队列窗口对照（2026-10-02）

只扩展独立 example，在同一签名二进制、私有档案、相同有效 entitlement（与生产
逐项相等）下比较 Tauri 回调中直接执行 NSWindow 操作与 AppKit main operation queue。
主队列 block 断言主线程，保留窗口 wrapper，执行完递增计数；不跨线程传原生指针。
release build 与定向 Clippy 通过。本批不是生产窗口补丁。

binary SHA256 `27911b6e5078d5ec997cf1c2f76b399337288f66cf8943b4e283e0ddddd68a52`，
app `/private/tmp/reasonix-tauri-window-baseline-_g1ocvl1/Reasonix Window Baseline.app`。

| 样本 | 前提 | 执行次数 mini/restore | 结果 |
| --- | --- | --- | --- |
| Cocoa 直接执行，PID 80916 | active/key/visible/隐藏页 Finished 全满足 | 1/1 | native minimized 一直 false，诊断退出 2 |
| Cocoa 主队列，PID 80932 | 同上 | 1/1 | 同上，诊断退出 2 |
| Tauri 普通标题栏主队列，PID 80946 | active/key 前提未满足 | 1/1 | 前提失败，不解释最小化结果，退出 2 |
| Tauri Overlay 主队列 | 未运行 | — | runner 在前一项前提失败后停止 |

主队列请求确实执行，因此“动作没有执行”及“仅 Tauri 代理回调内调用导致该 Cocoa
样本失败”不足以解释差异。没有证明应用 delegate/NSApplication 子类/其他宿主
上下文的具体根因；前一批当前独立 Cocoa 正对照仍有自己的通过范围。
保留所有失败/前提失败，未重试覆盖或记成通过，未改系统设置。D 未验收、E 未推进。
