# D：真实 Minimize 菜单 sender 的只读诊断

源 HEAD 01822bac3，构建时仅 native_menu_smoke.rs 与 native_window_smoke.rs 两项修改；冻结源码与 install.json 对应。新增诊断只在既有 opt-in 原生窗口探针内读取固定菜单角色，没有增加 renderer 命令、权限、菜单 action 或恢复行为。

## Review 与验证

遍历有深度/项目数上限，在 AppKit 主线程读取 performMiniaturize: 项的 enabled、hidden、固定 Cmd+M 配置及真实 sender 的目标解析；不调用 menu.update、validation 或发送 action，不保存目标对象、输入字符、URL 或用户内容。未发现本批需修复的代码问题。此前严格 cargo clippy --all-targets -- -D warnings exit0；pnpm tauri:build exit0，包含前端门禁、类型与 bundle 预算，app/DMG 构建签名通过。使用新安装 sidecar 的 cargo test：231 passed、0 failed、5 ignored；ignored 不算通过。

新 DMG 只读挂载并复制至独立临时安装目录，严格 deep 签名通过。host 1754bc34d6b89d263ac1173f613cf3fe92a00555dea231b4261b89ec3591ef7f、sidecar 5c8dad34056254e6ff965d41fc1b3748bbf29aaf4c68321964baac1755c04a21；DMG 哈希见 install.json。本目录是该包的证据，不继承 e677 的其他验收。

## 实际结果与失败记录

managed/explicit 的 menu-settings-native-minimized 原有安装包 gate 2/2 通过（session22816 terminal0），实际 shown/minimize-ready/minimized 几何匹配左屏 x=-3200/y142/2560×1640/scale2。ready 阶段真实 sender 目标为 main，菜单启用且 Cmd+M 配置正确；native 最小化与 Settings 恢复通过，原 host/open 正常退出0、sidecar/readiness清理。

私有交互 host5551/sidecar5558，初始实际窗口几何同上（session82829 terminal0）。CUA Raise、点击空白 composer 后发送 Cmd+M，主窗口 local monitor 记录两次 cmd-m，两次 inputMainWasKey/actionTargetIsMain 均 true。随后实际 Window→Minimize 点击，仍无 will/did-miniaturize、nativeMiniaturized=false，restoreRequests/Completions 保持1/1。真实输入路径未通过，不能用程序 gate 代替。

动作后读取 key.json 时 nativeKeyWindow=false；menu.json 时 applicationActive=false。此时 senderTargetPresent/IsMain=false 不能证明按键发生时 sender 解析失败，更不能作为根因；下一步应在现有按键回调中同步记录固定 sender 诊断。未采取重映射 native role、主动验证菜单或重复恢复等绕过措施。

实际 Cmd+Q 后原 host/open exit0，两个程序 gate 与交互共六个原 PID 均不存在，包签名/哈希保持。未再读取或重绑定已退出的 app；未归档私有 HOME/core/ready、令牌、原剪贴板或原生二进制。

## 仍待验收

最小化、窗口稳定与其他 D/E/A/B/C 缺口保持。此包只覆盖上述程序 gate 和失败诊断，未补验全屏/物理选区/托盘/通知/keychain/完整官方 GUI 回退。仅沿用 Windows/Linux 用户延期；没有 push、正式发布或默认下载切换授权。用户要求提交后重新打包，重打包产物须另有提交 HEAD 与哈希回执，不能与本目录此前构建混同。
