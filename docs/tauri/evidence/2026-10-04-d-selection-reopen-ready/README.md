# D：选区普通重开就绪前提修正与 Minimize 响应目标（2026-10-04）

上一轮有源码/真实新包七项前的六gate、输入原子观察及失败分类进展，本轮继续依据失败定位。生产配置持久化、窗口、菜单、权限、依赖、前端均未改；改动限opt-in探针：check错误带固定stage、首次storage访问前确认可信Finished页面与渲染入口，原生输入增加分发时key/响应目标固定布尔值。

## 两轮安装身份与来源

基于 `c7aae23ff` 加各自source.patch的dirty构建，首轮源码在 `first-source/`、最终在 `final-source/`，与相应gate冻结patch逐字一致。

| 候选 | host SHA256 | DMG SHA256 |
| --- | --- | --- |
| 首轮阶段定位 feb | feb6b7e16e3277fb89c0e3edde18c75bf9ffc2bd7e3717daf42d7a05bf1ff343 | 27add288f9663992206b048c7da123c9ed11cf934a4382ae52b8aa30ff9fc750 |
| 最终就绪修正 46 | 46c2c9da5f1d10a0c92a5d319b465b381a36256e9e770870e6b6933ec13c5256 | 72b472c6edcc6c3a409205ce0964a56af2e0169bdc68a998743a1a7780d22e85 |

共同sidecar `31418d61fb58823fabe12e0b448f9d005eae4566381e0ff3ca868ef2c11d1fd4`。两轮真实app/DMG构建、只读挂载ditto全新安装及strict ad-hoc签名通过，根目录安装JSON有确切路径。最初strict clippy因诊断JSON宏大小达到recursion limit而失败；将新增字段移至现有另一快照对象后通过，没有增加recursion_limit。初始失败日志保持，首轮实际构建只使用修正后源码。最终strict clippy通过，使用最终真实安装sidecar的Rust **231 passed/0 failed/5 ignored**；ignored不计通过。前端未改，构建仍执行lint/类型/契约/bundle等原门禁。

## 原失败定位及修正范围

`first-selection/result.json`保持selection-send **failed**。首轮managed真实配置录键、Add引用、旧键原子条件、去重、二次发送/两次provider引用边界及第一hostexit0通过；普通重开kernel2，新的固定stage指明 **reopened configured selection shortcut** 的第一个检查发生WKErrorDomain/code4（JavaScript异常）。`first-selection-raw/`记录这次原失败，explicit未运行。

动作前 `selection-reopen-before-storage`原生快照：trustedMainUrl=true、mainPageFinished=false、visible=false、key=false。可信native URL不等同于页面已经完成/当前JS可读origin storage；没有记录异常localizedDescription/具体JS异常正文，不能进一步断言是SecurityError、JSON解析错误或产品初始化异常。该证据显示原探针在WK导航完成前开始首次storage检查。

最终探针先记录 `selection-reopen-before-ready`，等待既有trusted主页面Finished与URL匹配，再按原15秒DOM检查机制确认`.tauri-shell`及composer，记录before-storage后执行**原有**配置与历史断言。没有catch后忽略异常、动作重试、写storage、修改业务持久化、扩大几何容差或延长现有各检查超时；新就绪条件是读取已加载应用的前提。所有check错误只增加固定stage；原异常仍fatal。修正的是验收探针开始读取存储的时序，不宣称已修复业务数据丢失或证明具体JS异常机制。

## 最终46当前包实际验收

`final-native/result.json`七个gate **全部通过**：selection-send/reload/package/failure-exit/lifetime/fullscreen/window。

- selection-send：managed和explicit各全新私有档案，实际Settings录Cmd+Shift+L、旧键未接管且保留选区、Add引用/草稿保持/去重/OS剪贴板保持、实际二次发送与provider引用边界均通过，各恰两次provider请求。随后普通重开，两档案均kernel0，localStorage配置值保留、实际会话点击/引用历史回读/内部协议隐藏/无草稿回放通过。日志保存两份reopen/configuredShortcutPersisted等完整回执；成功根按原工具清理，不宣称另有成功native trace文件归档。
- reload：两档案各正常/隐藏，用真实NSMenu生产handler验证新realm/rendered/storage/origin/原生窗口及同sidecar；只覆盖健康加载页面，不证明never-Finished/坏JS恢复。
- package/failure-exit/lifetime：原鉴权/命名空间/权限/正常和异常退出、两档案idle/streaming及TERM/KILL退出清理/同档案重启保护通过。
- fullscreen：双档案原exercise/restore前置、真实原生菜单进入退出完成通知各1，存档字节/精确几何与重启回读保持。
- window：**managed24/24+explicit24/24=48/48**，原最小化/隐藏/显示/最大化/精确恢复/重启、外观失败回滚、后台关闭/任务运行时菜单Quit、菜单/剪贴板/编辑/面板取消/托盘语言/单实例/关闭退出原门禁通过。阶段数不算独立gate；一次矩阵通过不是间歇问题根因已修复或全D已稳定。

Rust忽略GUI测试不与真实门禁争用；原生门禁串行，原handle终态后才启动下一项。`final-backup/`另验本包实际配置备份应用到0700私有HOME/0600配置，由固定SHA官方1.38.3内嵌CLI回读exit0，摘要一致/原树保护。**本46完整官方GUI历史/附件/检查点回退未运行**，不继承历史包。

## 原生响应目标：实际物理差异仍保持

新增查询仅在既有opt-in快照/interactive本地主窗口固定M/F事件中执行。使用标准AppKit selector、nil target和nil sender搜索响应者，保留target存在/是否本NSWindow、当时key布尔值；对象只在主线程查询期间retained，未保存pointer/class/内容，也不sendAction或修改角色。输入仍返回原event，token按原退出路径移除、32条记录边界保持。**nil-sender查询结果不证明实际菜单sender的动作调用已发生。** [Apple target查询说明](https://developer.apple.com/documentation/appkit/nsapplication/target%28foraction%3Ato%3Afrom%3A%29)

最终私有显式idle root `/private/tmp/reasonix-launch-services-6pwq3bv5`，PID89869/sidecar89876，exact live CUA路径与私有origin绑定。操作与各动作后原生快照在 `physical-actions.json` 和 `reasonix-ready-physical-*.json`，没有截图导出。

- composer点击后key=true、minimizeTargetPresent/IsMain=true；一次Cmd+M观察到两条匹配事件，**每条inputMainWasKey/targetPresent/targetIsMain均true**。仍nativeMiniaturized=false、Will/DidMin=0；事后key=false/target不存在。由此排除本次输入观察时查询不到主窗口目标的解释，不能用事后失焦倒推分发前无目标，也不能断言menu实际调用或AppKit已拒绝。重复事件来源未证实，不能认定用户按两次或监听产生重复。
- 首次Ctrl+Cmd+F进入：计数0/进入事件0，未验收；local monitor覆盖有边界，0不直接证明未送达。随后实际View→Toggle Full Screen：native进入/DidEnter1；Ctrl+Cmd+F退出观察到两条key/targetMain为true的事件，DidExit1/fullscreen=false，原几何2560×1640/x988/y130/scale2精确恢复。仅菜单进与快捷键退出通过，不扩大为键盘双向通过。
- 实际App→Quit后原session84134正常结束，kernel hostexit0/openexit0、sidecar/ready清理通过；未读/重绑死亡CUA对象，未以信号终止计成功。

物理最小化、首次全屏键进入与间歇根因仍未解决。下一步沿实际角色调用与窗口转移核对，而非增加自动focus/minimize重试或替换原生角色来获得绿色结果；其他D可继续推进，但D稳定之前不推进E逐项验收。

## Review、证据清理与完整目标

两轮Frozen source.patch/helper SHA与构建候选一致；最终源码/精确stage条件、main-thread target借用、原样event/退出remove、固定输出边界已review。postcheck前后两安装包strict signature/SHA保持；首轮失败/最终物理夹具无Preview、sidecar/ready残留，exact物理host不存活。没有复制私有HOME/core、原始剪贴板、keychain、observer/安装二进制。SHA256SUMS覆盖本目录其他全部文件。

当前包选区配置/二次发送/普通重开程序门禁已经通过，物理IME/选择/快捷键场景不能自动视为通过。D仍待物理最小化/首次全屏进入与窗口稳定性、真实托盘、混合scale/拔屏/零活跃后恢复、文件目录确认保存、通知点击/拒绝重授权冷启动、钥匙串人工授权取消/旧来源边界、mailto/OAuth实际来源、旧共享自定义根互斥、never-Finished可见恢复、全资料官方GUI回退等。浏览器canary用户已确认，未重复。

A协议/事件/错误、B旧Global/图像/大历史、C terminal/browser/worktree/MCP/plugin真实工具进程生命周期仍有缺口；E remote host/bot/updater/复杂页已有部分实现但尚未按D稳定前提逐项完整验收。Developer ID正式签名/公证等仍阻碍正式发布。仅Windows/Linux沿用用户确认延期；没有新增延期、push、正式发布或默认下载项切换。目标保持active。
