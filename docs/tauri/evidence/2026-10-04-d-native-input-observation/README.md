# D：原生输入与旧选区键原子诊断（2026-10-04）

上一轮通过实际 f4 的窗口48/48、全屏及物理菜单进/快捷键退，推进了真实包验收；物理最小化、首次全屏快捷键进入仍失败。本轮保留那些记录，增加诊断以区分输入观察与窗口完成，并在新包重新验收。生产窗口、菜单/快捷键行为、权限、依赖和前端未修改。

## 源码与当前安装身份

基于 `c18e4ceb2` 加 `source.patch` 的 dirty 构建；两份改动源码与未修改的 WK evaluation helper 在 `source/`。真实 app/DMG 构建、只读挂载复制安装及 strict ad-hoc 签名通过，安装路径 `/private/tmp/reasonix-d-input-observation-installed-rh0um20b/Reasonix Tauri Preview.app`。

- host：`041949c6917d9b77d10fc1b837cc0f0ea4c0cc1dd66b44ccf63ffff6bde14d85`
- sidecar：`1adfc780af2305c149bc5795ee9f66809979642f1044a5e867448c50a48ea493`
- DMG：`5c48aaf1d9498e1273eb1687f522377be7c4a11b115ed6c57d6d1303e3d4aff1`

不是 f4 包，摘要、原始构建/安装日志已保存；不继承旧包的窗口或回退结论。本轮 final strict clippy通过；使用当前安装sidecar的Rust **231 passed / 0 failed / 5 ignored**，ignored不计通过。构建含前端类型/lint/契约/bundle门禁，前端未改，未重复用户已确认的浏览器canary。

## 新诊断及权限边界

`native_window_smoke` 只在 opt-in `interactive-observe` 注册 AppKit **local** monitor；main-thread注册，过滤本主窗口number及 KeyDown/LeftMouseDown。仅固定 macOS M/F虚拟键位与Command/Control/Shift/Option组合分类计数，以及主窗口左键计数。没有读取字符、保留任意键、坐标、事件对象或页面内容；callback立即返回原指针，不消费/替换/发送事件，不做focus/minimize/重试。退出时按原 `stop_window_observers` 路径先remove准确token，通知observer也照常清理；注册失败明确exit2。普通/程序化门禁不注册输入monitor。计数与最近32项时间线写入既有只读快照，属于观测而非产品行为修复。

[Apple local-monitor文档](https://developer.apple.com/documentation/appkit/nsevent/addlocalmonitorforevents%28matching%3Ahandler%3A%29)说明它覆盖本应用sendEvent分发，菜单等内部tracking loop可能绕过。因此计数为0不能直接证明系统没有送达，也不能用记录到按键替代动作完成。

选区探针在同一次旧Cmd+L dispatch后冻结 `defaultPrevented` 与 `selectionCollapsed`，仍要求两者均false；保存到独立 `reasonix-native-old-shortcut-observation.json`，原Add action观察文件不覆盖。失败文字改为联合条件失败，避免误称旧键仍接管。没有吞异常、重新select、重复action或放宽原断言。

## 当前真实程序门禁及回退

- `native/result.json`：reload/package/failure-exit/lifetime **4/4通过**。Reload两档案×正常/隐藏；healthy页面范围不扩大为never-Finished/坏JS恢复。生命周期包括idle/streaming及TERM/KILL、实际sidecar清理/重启保护。
- `window/result.json`：完整window **managed24/24 + explicit24/24 =48/48通过**，随后双档案fullscreen gate也通过。原几何/最小化/恢复/最大化/持久化、后台关闭/任务退出、外观回退、菜单/文本剪贴板/编辑/取消面板/托盘语言/单实例/退出断言保持；全屏进入退出完成通知各1、原存档和精确几何/重启保持。两个gate均通过，不把48个阶段误算48个独立gate。本候选共六个程序gate，不代表D或物理UI全部通过。
- `backup/`：本包真实配置备份应用到独立0700/配置0600目录，固定摘要的官方1.38.3内嵌CLI回读exit0，备份/恢复SHA一致且各原树保持。当前包**完整官方GUI历史/附件/检查点回退未运行**，不借用f4/b6结果。

上述源码/包门禁串行，Rust GUI ignored与原生门禁不并发；原session核对terminal后才进入下一组。日志、工具冻结、sourcePatchSha与当前源码一致。失败没有观察超时重启。

## 选区失败记录及其含义

`selection/result.json`原gate **failed**。managed设置录键、Add引用/去重/剪贴板保持、配置键二次发送、精确两次provider引用边界响应及首次hostexit0均通过。新原子观察显示 `oldShortcutDefaultPrevented=false`、`oldShortcutSelectionCollapsed=false`，动作open/enabled、页面visible/focused，证明**此次当前managed样本**旧键未接管且当时选区保留；不能回推f4的历史联合失败是哪一项。

随后普通重开kernel2：`WKWebView edit evaluation failed (WKErrorDomain, code 4)`，对应锁定WebKit错误枚举的JavaScriptExceptionOccurred。此前通用异常现已分类，但未记录具体check stage或异常文本，不能断言是哪条表达式/Storage访问/产品渲染错误。源码复核发现reopen首个检查直接读localStorage，而run首个检查先等渲染入口；这只是待验证的初始化前提差异，不将其写成已证实根因。没有安装包reopen通过，explicit档案未运行。失败根 `/private/tmp/reasonix-native-message-copy-b9jvcii7`保留，安全的原子/首轮/发送/退出回执复制到 `selection-raw/`；没有复制原始剪贴板或私有历史。

下一步先在check错误传播中附固定stage、核对读storage时真实主页面初始化，再修正已证实的前提或产品缺陷，保持配置持久化/无draft回放/引用历史/恰两provider请求原门禁。不能将nullable guard或JavaScriptException分类本身当作修复通过。

## 实际输入/窗口差异

私有显式idle root `/private/tmp/reasonix-launch-services-z2o9gi6x`，PID82365/sidecar82372；native观察与CUA绑定该确切live应用/私有URL。动作转录 `physical-actions.json`，各动作后的原始native快照在根目录。无截图文件，不能宣称有像素归档。

1. composer点击后nativeKey=true、localMainLeftDown=1。
2. 一次CUA `super+m` 后 `localMainCmdM=2`，nativeMiniaturized=false、Will/Did=0、keyfalse。可证有匹配本地事件；**不能推断用户按了两次、monitor制造重复、菜单role已调用、或min根因已解释**。
3. 再聚焦composer、首次Ctrl+Cmd+F后计数0/进入事件0，未验收；monitor覆盖有限，不能将0单独解释为输入未送达。
4. 实际View→Toggle Full Screen后nativeFullscreen=true/DidEnter1。随后Ctrl+Cmd+F退出：localMainCtrlCmdF=2、DidExit1、nativeFullscreen=false，原几何2560×1640/x640/y142/scale2精确恢复。菜单进入/快捷键退出通过，快捷键首次进入不通过，不扩大为双向完成。
5. 实际App→Quit后原monitor session67634 exit0，exact kernel hostexit0/openexit0，sidecar/ready无残留；没有靠信号结束计作成功，没有重读/重绑死亡CUA对象。`physical/`有launch/result/exit回执。

输入monitor证据排除“这次Cmd+M完全未进入本地主窗口事件观察”的解释，但尚不能确定事件重复来源、原生role实际target或AppKit拒绝转移原因。下一步沿响应者/角色target和原生转移条件核对；不以自动窗口重试或改变菜单角色掩盖失败。

## Review与目标状态

复核main-thread原生指针/事件borrow、原样pass-through、token所有权与remove顺序、固定输出/32条边界、非interactive分支不开monitor及原子观察文件分离。strict clippy、231项、构建/真实包六gate及配置回退通过；当前失败保持failed，无新增权限。`reasonix-input-observation-postcheck.json`前后签名/SHA保持、Preview及确切physicalHost不存活，选区/物理夹具无sidecar或ready残留。SHA256SUMS覆盖本目录其他全部文件，helper哈希已核对。

D仍未完成，不能把间歇窗口失败称已修复；E仍等D稳定后逐项推进。除上述物理最小化/首次全屏键/选区重开外，真实托盘、混合scale/拔屏/零活跃后恢复、文件目录确认保存、通知点击/拒绝重授权冷启动、钥匙串授权取消/旧来源边界、mailto/OAuth实际来源、自定义旧共享根互斥、never-Finished可见恢复和完整资料官方GUI回退仍有待验项。A协议事件错误、B旧Global/图像/大历史、C真实工具进程生命周期、Eremote/bot/updater/复杂页及正式Developer ID签名/公证仍有发布缺口。仅Windows/Linux沿用用户确认延期；目标active，无push/正式发布/默认下载切换。
