# cbb590ba 完整窗口门禁失败

固定同一实际候选，原完整 window --focus --edit --dialogs 门禁未改。managed exercise 尺寸/位置、隐藏/显示后第一次最小化超时，completed stages=0，后续阶段与 explicit 未运行。native minimized=false，无 will/didMiniaturize，restoreRequests/completions=2，最终页面完成但 active/key/occlusion 均 false。失败夹具 /private/tmp/reasonix-native-window-smoke-if8a3pq9 保留，退出/侧进程清理由原 runner 执行。

Tauri available_monitors 返回空，独立 [AppKit/CoreGraphics 只读对照](../2026-10-03-d-cbb-display-availability/README.md)发现 NSScreen 保留两块屏幕，而 CGGetActiveDisplayList 返回 status0/count0，正常与私有 HOME 相同。不能据此证明最小化根因、无物理显示器或仅由私有环境引起。CUA getState 本轮30秒超时、kernel reset，未获得有效桌面快照，未重启此失败门禁。

D 窗口稳定性未通过，旧包局部通过不能覆盖本次失败。
