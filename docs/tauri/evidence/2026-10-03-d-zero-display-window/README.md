# b6f7003f：完整窗口失败与真实零活跃显示器保护

2026-10-03，当前候选补验；生产及探针源码未改。执行源 HEAD `13da58eca`，实际包为上批由 fc9c96828 加冻结探针构建的 b6f7003f；新 HEAD 仅提交了该批源码/证据，不表示重新构建。

## 完整矩阵保持失败

`window/result.json` 与 `window/window.log`：完整 window 门禁在 managed exercise 后读取窗口存档失败，0 个阶段被外层接受，explicit 未启动；排队 tray-language/startup 均 not-run。原生 exercise 自身 ok=true、kernel exit0；minimized=true 且 Will/Did=1，但没有 window-state.json。不能记完整48/48，也不把原生自身成功覆盖外层存档失败。

`failure/` 保留首轮四份原始数据，未复用失败夹具。四个时序点 monitors=[]、初始/后续窗口几何及页面 Finished 标志可核对；`PreviewWindowState::capture` 在 available_monitors 为空时拒绝采集，新档案没有既存缓存，save 不写空状态。此记录与无显示器存档保护一致，不证明历史最小化/失焦/2px根因，更不能据此改断言、补默认存档或把整组改成通过。Mac 锁定与零活跃显示器不是同一条件，本次以实际 CoreGraphics 查询/原生时序为依据。

## 独立三门禁通过

`conditional/result.json`：inactive-display-state、tray-language、startup 各 exit0、passed，不改写原队列的 failed/not-run。

- 零显示器：门禁在每次前后实际查询 CGGetActiveDisplayList，必须成功且 count=0。两个独立私有档案种子为 width2000/height1400/x-3400/y100/scale2，现有菜单 phase 不移动/缩放窗口。正常退出后解析的旧存档与种子相同、无 sidecar/readiness 残留。原始档案结果见 managed/explicit-inactive-result.json。此项证明真实零显示器期间旧几何保护，**未证明显示器重新活跃后的恢复、混合缩放或物理拔插**；断言比较结构而非声称原文件字节/mtime相同。
- 托盘语言：两个档案各 menu-shortcuts/clipboard-native/tray-language，共6阶段；菜单及运行时语言/偏好保护、完整剪贴板备份恢复通过。程序调用不代表物理菜单或键盘验收。
- 启动：两档案 × SIGTERM/SIGKILL × parent-check/token/ready 三个边界，共12项；真实 kernel退出0、lease/readiness/sidecar清理、受保护原件保持与重启身份通过。不是物理 Quit 或冷启动通知点击验收。

同 b6 候选不同程序门禁累计17项通过（上一批14 + 本批3），仍不是 D 整组完成。完整窗口失败记录保留，需在实际显示器环境满足时继续完整矩阵；物理UI仍待解锁。

## 包身份与清理

host SHA-256 `b6f7003fb08ab9a6cf8229eafefebb0764ffac3b12fadcfe30367209b96df613`；sidecar `28a5c67da9208fb809fe601f35444436bd2fd835ed342b46b5f3b3bd51c3cf18`。两个队列逐项复核摘要/严格签名，末次签名及摘要保持。通过真实 plist ID 查询 Preview/Wails 无运行实例，失败夹具无自身sidecar/readiness。

末次核对初版 Python plistlib 无法解析官方省略 XML 声明的 plist；改用系统 plutil 获取真实 ID 后完成核对，不改官方包、不猜测ID。当前回退/构建/源码回归范围见上一批 [引用发送与回退](../2026-10-03-d-selection-send-reopen/README.md)，本批未重新跑这些门禁。

## 状态与后续

D仍进行中、E未开启；没有自行延期待验項，未授权发布/默认下载切换。物理菜单/快捷键及配置映射、IME、文件/目录确认保存、通知权限/点击、钥匙串授权及旧服务、重新活跃/混合缩放/拔屏、未Finished页可见失败恢复、任意旧共享目录互斥及历史窗口问题仍待验。A/B/C协议/历史图片/复杂进程生命周期与正式签名公证仍阻碍发布。

归档只含工具源码、执行日志和非秘密窗口/包身份JSON；不含私有HOME/core、剪贴板原件或二进制。在本目录执行 `shasum -a 256 -c SHA256SUMS` 验证所有冻结文件。
