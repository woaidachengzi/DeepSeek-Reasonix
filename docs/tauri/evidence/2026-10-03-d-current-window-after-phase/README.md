# 当前候选完整窗口门禁复验

当前只读DMG安装候选host249dfd15、sidecar29d985c7（完整hash见result.json），严格签名、全部程序/Info摘要及签名identity在门禁前后相同。沿用未改的smoke-native-window.py 886ca1c2，完整 --focus --edit --dialogs，各档案24阶段。未改变等待、激活/原生最小化条件或清理语义。

- managed 24/24通过，包含实际原生最小化/恢复、最大化保存/恢复、hide/后台close、settings三种隐藏状态唤起、运行中任务菜单退出、主题切换/失败回退、原生剪贴板、对话框取消、托盘语言、菜单编辑、第二实例、close退出与重启策略。每阶段退出0、sidecar/readiness清理；整套credential身份一致。
- explicit完成8阶段：exercise、restore-maximized、restore-normal、appearance-rollback-unconfigured、application-hide、background-close、menu-shortcuts、menu-settings-hidden。第9项menu-settings-minimized失败，原生窗口未进入最小化。后续阶段未运行，不能沿用managed通过覆盖。

失败fixture `/private/tmp/reasonix-native-window-smoke-e9zqkmss`由runner保留；本目录复制仅自身native结果、geometry与trace，不归档任何用户配置或凭据。失败程序由原runner finally清理；终止后独立查询确认own sidecar=0、readiness=0、Preview未运行，见post-terminal-cleanup.json。总runner exit1，result.json window failed；保留全部日志和原生失败状态，不能把前8项或诊断正样本算作整套通过。

同候选确实能在这次managed和explicit exercise中最小化，但后续重复启动仍失败。状态阶段诊断亦出现同二进制前后差异，见[诊断证据](../2026-10-03-d-state-phase-window/README.md)。当前说明窗口稳定性尚未满足，不宣称根因或生产修复。

本门禁不覆盖真实用户菜单/黄色按钮/CmdM/fullscreen交互与多显示器物理变更，亦不替代配置快捷键/IME、通知、钥匙串等剩余D验收。D未完成、E尚未开启；仍需按完整目标继续。
