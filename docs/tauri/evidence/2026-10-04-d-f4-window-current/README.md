# 当前 f4：完整窗口与全屏通过，物理最小化仍未验收

2026-10-04。上一轮是生产原生 Reload 改造与验收，确有提交/源码/安装包证据进展；本轮继续优先核对最小化及全屏，不把历史成功迁移到当前候选。没有修改生产代码、probe 或门禁，不增加重试、sleep 或容差。验收时工作树干净，源码 HEAD `ee1d34698`；其代码对应上一轮实际构建源，原 dirty 构建映射仍见[Reload 证据](../2026-10-04-d-native-reload/README.md)。

## 固定当前候选

安装 `/private/tmp/reasonix-d-native-reload-final-installed-6ade4s85/Reasonix Tauri Preview.app`：host `f4c2a60db15259131e2b7b4fd334d1a6746f443f6b21ae092707fa46adba1e98`，sidecar `7352e79707cdcb83d7f55348afe251e5065c335c27a3b9c1e6506d6d3f403649`；DMG 摘要/挂载复制安装、构建、Rust231/5ignored、strict clippy 和当前包配置备份应用/官方 CLI 回读在上述上一轮证据，本轮未重建或重复这些测试。两个新增 gate 记录当前摘要、strict signature、干净源码来源和冻结 helper（含 `.c`）；前后摘要保持。

Wails 1.38.3 `createAppMenu` 使用原生 App/Edit/Window 角色、Settings/Show/Quit 事件，基线 `wails-menu.go` 已保存。本轮继续测试 Tauri 原生最小化及全屏角色与窗口恢复，未改成 JS 模拟，也没有更换安装副本。

## 程序化验收

1. `minimize-controls/result.json`：两种档案各四例，全新 0700 根，各动作路径只执行一次。Tauri API、直接 NSWindow::miniaturize、原生 Minimize 角色共 **6/6 通过**；收到实际 Will/DidMiniaturize=1，并通过既有 Settings 恢复/事件断言。要求 AppKit 和应用 occlusion visible 的额外两例 **前提失败**，没有发出最小化动作，CLI exit1；两份原始失败/时序保留，不能称8/8，也不能称这两例最小化调用失败。成功样本的 occlusion 也可为 false，不能把等待呈现当作修复。
2. `fullscreen-current/result.json`：**fullscreen gate passed**。两种档案各原 exercise、restore-maximized、restore-normal、menu-fullscreen、restore-normal 共10个进程阶段通过；菜单进入/退出各完成通知一次，普通几何/存档字节保持，重启回读及凭据身份不变。原切片含完整几何/最小化前置，没有绕过失败断言。六条全屏 before/entered/exited 原生回执在日志中。
3. `full-window-current/result.json`：**window gate passed，managed24/24 + explicit24/24，共48/48**。原有尺寸/隐藏/显示/最小化/恢复/最大化、重启回读、外观成功与失败回滚、后台关闭、菜单/Settings恢复、真实私有任务运行时后台菜单Quit、剪贴板、取消面板、托盘语言、编辑菜单、单实例和关闭退出/重启回读断言均通过；所有 host 有 kernel exit0，真实 sidecar/ready清理、凭据身份保持。一次完整矩阵通过不证明间歇根因已修复，也不代表物理托盘点击、面板确认/保存、完整多显示器切换、通知/钥匙串、D/E整组完成。

上述原生检查串行运行，原 session 17801 exit1、91059 exit0、52136 exit0，终态确认后才进入独立物理夹具，没有超时重启或复用失败根。

## 实际 CUA：进展与失败并存

私有显式 idle 夹具 `/private/tmp/reasonix-launch-services-dw7m1pez`，exact PID79726/sidecar79733，`--interactive --observe --wait-seconds300`；所有操作仅绑定该已确认 live 的安装路径/私有 main URL。原生只读状态在各动作后复制；完整动作转录在 `physical-actions.json`。没有截图导出，不能宣称已保存像素。

- Window→Minimize 后没有 native mini/Will/Did；随后 Window→Show Reasonix 使 active/key=true。已确认焦点后黄色按钮仍未收到事件，nativeKey 转 false。两种物理路径未通过，不能从 AX 树仍在直接判定视觉状态或归因某个产品调用。
- 聚焦 composer 后首次 Ctrl+Cmd+F 进入未收到全屏事件，因此 **快捷键进入未验收**。随后实际 View→Toggle Full Screen 收到 Will/DidEnter各1、nativeFullscreen=true；实际 Ctrl+Cmd+F退出收到 Will/DidExit各1，原几何2560×1640、x=-3200/y176/scale2精确恢复。**菜单进入与快捷键退出已验收，未扩大为键盘双向或全部窗口状态通过。**
- 全屏退出后 Cmd+M仍无mini/Will/Did，即使当前 nativeOcclusionVisible=true，不能据此把 occlusion作为失败的充分解释。输入分发/焦点或AppKit转移根因未确认，没有加窗口重试或替换原生角色。
- 实际 Reasonix→Quit：原 monitor session57055正常exit0，exact kernel hostexit0/openexit0，sidecarRemaining=false/readyRemaining=false；未使用信号终止计作成功，未读取/重绑死亡CUA对象。`physical/`含launch/result/exit回执。

`postcheck.json`严格签名、host/sidecar SHA保持；Preview与physicalHost均不存活，物理夹具与两个失败controls的自有sidecar/ready无残留。私有HOME/core、原始剪贴板、keychain和observer二进制没有归档。

## 剩余目标与下一步

本轮推进的是当前包窗口/fullscreen证据；上一轮 dda 完整矩阵失败与 f4 selection-send联合断言失败保持原记录，没有改写。新的48/48不证明那些失败由Reload修复。下一步继续区分物理最小化输入分发与AppKit状态转移，并将选区旧键联合断言的两个子条件分开记录，按真实新候选复验；不能让泛化错误说明替代原因证据。

D仍未验收完成，Preview稳定性仍有物理/间歇缺口。原生快捷键/IME/选区及配置重开，物理最小化/托盘，多屏混合scale/拔屏/零活跃后恢复，文件目录确认保存，通知横幅点击/重授权冷启动，钥匙串人工授权取消/旧服务边界，mailto/OAuth实际来源点击、旧自定义共享根互斥、never-Finished可见恢复、当前包全资料官方GUI回退仍待验。用户已确认浏览器canary，不重复该人工验收。

D稳定后再推进E；remote host/bot/updater/复杂管理页仍需配置/进程/连接/异常/权限逐项验收。A协议事件错误、B旧Global图像/大历史、C真实工具进程生命周期以及Developer ID签名/公证等仍阻碍正式发布。只有Windows/Linux沿用用户已确认延期，没有自行延期其他项目；目标保持active。无push、正式发布或默认下载切换。

所有gate helper的冻结摘要已复核，源码patch为空且native检查期间未改工具。SHA256SUMS覆盖本目录其他全部文件；所有结果按对应范围解释。
