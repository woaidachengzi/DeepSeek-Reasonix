# 2026-10-02 托盘语言与 Wails 基线对齐

## 缺口与实现

冻结 Wails 1.38.3 的 `desktop/tray_common.go`、`settings_preferences.go::SetTrayLocale` 将已解析 UI 语言同步为托盘“打开/退出”或“Open/Quit”，不持久化独立托盘语言；启动语言来自配置，Auto 由前端解析后同步。Preview 原托盘固定 Show Reasonix/Quit Reasonix，工作区没有这条同步。

现托盘保留实际 Tauri MenuItem handle，新增 runtime-only `set_tray_locale`；工作区挂载与 locale 改变均传入解析后的 en/zh/zh-TW，Auto 不被改写成显式语言。标题沿用 Wails（繁体 UI 同样使用基线中文托盘标题）；未知语言拒绝且不改菜单，第二项更新失败会尝试恢复第一项。失败在当前工作区显示重启/重试提示，不泄漏原生诊断，卸载后的旧错误不显示提示。未修改菜单动作、快捷键、关闭/退出或窗口恢复策略。

新命令沿用整个 custom handler 的主窗口 caller 限制，不接受 renderer 伪造窗口来源，没有新增通用插件权限、文件系统权限或持久化项。新 `tray-language` opt-in probe 经主线程调用同一入口，核对 Tauri menu item 状态和配置不变；完整窗口 runner 此后也包含该项（两档案共 48 个阶段），独立切片可单独选择。

## 回归与实际包

- TypeScript/Rust 编译、严格 all-targets clippy、完整前端 Tauri 回归通过。最初测试因 stub 尚未导出新 API 失败，补齐后通过；不是产品失败已被测试覆盖的证明。
- 新组件检查证明 Auto（navigator English）在挂载时同步托盘、不调用保存语言；native 拒绝时工作区给出解决提示、不显示底层错误。全量测试类型检查通过。
- 首次生产构建成功编译及签名 app，但 DMG bundle 脚本失败，保留 build-first.log。相同正式构建流程在允许本地磁盘镜像操作后完整 app/DMG 构建通过；没有进一步将失败归因为特定 OS 设置。
- DMG 校验通过，只读挂载、私有目录实际安装及严格 ad-hoc 签名通过，镜像已卸载。host SHA-256 `4d2a6865a75341b60c2635600675442267fb78566b84143e29d8c90216983e91`，sidecar `29d985c7af09baefec4308339429eabfe921896c47db6f4c30584b742b7efd39`；DMG 摘要和路径见 install.json。
- 两种私有档案各菜单、clipboard-native、tray-language 三阶段，共 6/6，通过 en/zh/zh-TW 标题状态、无效输入拒绝、bridge 的已保存 language 前后相同，档案重启身份、鉴权拒绝、剪贴板原件恢复和退出清理。后续正常 package 两档案通过，验收后包摘要/严格签名保持。

## 证明范围与剩余门禁

菜单标题读取是 Tauri MenuItem 状态；已核对锁定 Muda 0.19.3 的 setter 会更新 AppKit items，但 getter 返回缓存。不能把这项程序化证据当成用户实际弹出的 NSMenu 文本/托盘点击通过。真实 UI 同步与托盘显示/点击仍须 CUA 通道恢复后验收。

当前新包未复跑完整 48 阶段窗口、异常退出、双屏、通知/钥匙串和数据回退；前包 b95b38c0 完整 31/46 后显式最小化失败及其独立 8/8 异常退出各自归属保留，不转记为新包成功。此前 CUA -3811 及原生最小化失败根因未确定。D 未验收，E 未推进，A/B/C、全量旧数据回退、正式签名/公证与发布授权缺口均保留。未改变默认下载项或发布。
