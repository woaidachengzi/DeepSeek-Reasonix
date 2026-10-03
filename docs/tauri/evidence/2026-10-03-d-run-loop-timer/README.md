# AppKit run-loop timer 调度对照

仅扩展私有 window-baseline example，新增 `REASONIX_WINDOW_BASELINE_ACTION=run-loop-timer`。沿用同一采样步数、4/14 调用时点、原生 miniaturize/deminiaturize 方法及计数；改为 main run loop 默认 mode 的 50ms single-shot NSTimer 回调。保留原 direct/main-queue 分支，不改变生产菜单或窗口、依赖清单和权限。

最终默认 feature 离线 locked release build、同配置 Clippy -D warnings、git diff --check 通过。曾在错误 cwd 下两次尝试编辑，均 FileNotFoundError 且未写入；见 setup-errors.txt。特性显式构建与最终默认构建日志都保留。

诊断包二进制 SHA256 `650f2b60da3a2cfb04f089a6b18ebccf13c008177d4be019ec2bc3fe248536af`；strict ad-hoc 签名通过，有效 entitlements 与当前生产包相同。使用私有 HOME/TMP、固定本地页面，无 sidecar 或用户设置。

- 独立 Cocoa + WKWebView + loaded reorder，PID21959：激活/key/visible/page/WK加载前提成立，timer 开启、mini/demini各实际执行1次；原生 minimized始终false，无will-mini/did-mini/did-demini，kernel2/open0。
- Tauri overlay 窗口，PID22164：激活/key/visible/page前提成立，同样各执行1次，原生最小化/恢复均false、kernel2/open0。
- 紧随其后复验保存的 Swift Cocoa 正对照，原始二进制b88de85c，PID22123：真实will-mini/did-mini/did-demini，minimized=true后恢复，kernel0/open0。保留固定源码原件，未改变它的启动时序。

前两项 runner0仅表示成功保存有效失败及真实退出回执，不是最小化通过。比较证明当前桌面环境可完成最小化，但替换为本次定时器回调仍不足以修复问题。Swift对照的加载完成后即时激活/调用时序、窗口组成、应用身份等仍有差异，不能据此把根因认定为Tauri事件循环。下一步控制页面完成后的激活与实际调用时机。

本轮没有生产代码变更，因此249dfd15当前候选的既有验收不需重建；本例对照也不能作为生产候选最小化通过记录。D窗口稳定性仍阻碍D完成，E尚未开启。
