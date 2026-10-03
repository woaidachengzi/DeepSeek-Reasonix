# f259ddc9 当前安装候选窗口矩阵与安全前置中断

2026-10-03。未改窗口/菜单生产代码；使用消息选区改造后实际安装包：host `f259ddc90191a09d7f5418b30579c6c9ca89e754e9ad456ccf5001d241ee7ca3`、sidecar `156f1d447b179408fd2a803ee496d1be96d31a8c9b870f0deb637d02b3335311`、DMG `f1bdf9e05c4ab05f0192f7e803ef0ffafa1e86b1744e084204f3bcac402a600e`，详情见 install.json。包来源/源码 patch 已在[上轮复制验收](../2026-10-03-d-message-selection-copy/README.md)保存，不把当前 HEAD 当作该包的干净构建来源。

## 完整矩阵的准确结果

原 `trace-native-window.py` 调用未修改的完整 `smoke-native-window.py`（focus/edit/dialogs/LaunchServices），每阶段附只读 NSWorkspace 激活与原生窗口边界时序。托管 **24/24** 通过；显式 **20 项通过/计划24**，在 menu-editing 启动前的剪贴板 capture 前置检查拒绝停止。44个已运行阶段均 kernel exit0；显式 menu-editing、second-instance、close-quit、restore-close-quit 当时未执行。不是完整48/48，不是产品最小化或焦点失败。各阶段 receipt、native trace/result 与原始完整日志保留。

显式失败私有 fixture `/private/tmp/reasonix-native-window-smoke-56bucily` 保留。原始 __enter__ 仅输出泛化前置错误，没有保留该次 helper 的具体返回码，不事后补造原因。未写入系统剪贴板，没有自动替换用户内容。随后独立只读 capture 当前剪贴板返回1，AppKit 固定诊断指出不可读取的 invalid UTI（content），快照 unavailable/bounds；该当前状态符合拒绝保护，但不能证明此前一次 capture 的精确失败原因。未归档任何原剪贴板字节、私有 HOME/core、凭据或编译后的观察器二进制。

## 独立补验与诊断改进

- 在严格核对所有者、目录0700、同一固定签名包、原生 geometry 种子和无运行实例后，复用上述停止的显式私有 fixture，独立执行 second-instance/close-quit/restore-close-quit，**3/3**、kernel exit0，精确几何与凭据身份保持。结果在 remaining，控制脚本在 remaining-runner.py。这些阶段不触碰剪贴板；不补记为未中断完整矩阵通过。
- `NativeClipboardFixture` 现在将 capture 返回1/2/3/其他映射为固定分类：不可读/超限、快照期间变化、原格式不能精确round-trip、helper异常。只增加诊断，不改保护边界、恢复代次或真实复制操作，不回传 helper stderr/内容。
- 5项 Python 回归通过（新增一项含4个返回码子场景），确保拒绝不会执行系统写入、发布测试控制文件或泄漏 private stderr；已有中断/代次变化/恢复副本保护仍通过。该工具改动无需重建产品包；前后源码与 diagnostic.patch 保留。
- CUA getState 本轮再次30秒超时并重置kernel；没有完成本轮物理菜单/按键操作。用户被请求在方便时复制普通文字再确认，未收到响应，未把等待当成许可或自行更改剪贴板。
- 测试前、独立补验前后及最终严格 codesign 通过，remaining脚本还核对固定二进制SHA。当前包原四组 message-copy/package/startup/lifetime 通过范围仍有效；本轮不是其余全部D或官方回退验收。

## 下一项与发布边界

当前原48阶段矩阵不含全屏进入/退出，因此即使将来48项通过，也不能替代全屏验收。优先补齐实际NSWindow fullscreen位与进入/退出完成通知、退出后精确原尺寸/位置和持久化状态检查；物理全屏菜单/按键与最终最小化状态仍需独立现场证据。显式menu-editing待当前剪贴板能完整保存后验收，不绕过保护。

历史失焦/2px差异仍未确认根因，不用本次单实例成功关闭历史记录。多显示器拔插/混合缩放/零活动显示器恢复、授权取消及旧Wails服务、真实选择/保存对话框、通知点击/冷启动、任意旧自定义目录互斥，以及当前候选官方备份实际回退继续保留。D未完成/E未开启；A协议事件错误、B旧Global图片/大历史、C真实进程工具和正式签名公证缺口仍阻碍正式发布。Windows/Linux为唯一已有延期；没有新增延期、发布、push或默认下载切换。
