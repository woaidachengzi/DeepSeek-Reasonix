# D：About / Updates 依附主窗口、非阻塞显示

新候选 host `e6778b8fc3a5bcd7095d75afccedae753a521559fb6dd4896ea72d4100ec0706`，sidecar `6189151139f5c820c15e4da18419f7ce4bc582d7b8885d3f7227fde8b828cb1b`，DMG `82d37d058284416c17c73469e034271126713cae65d7f93c280bc8bdcee45072`。源为2d3744875加冻结product.patch；仅main.rs两个菜单分支改变，签名和bundle为本地ad-hoc Preview，未公证。

## 问题和修改

原菜单About/Updates未指定parent且blocking_show等待结果。当前插件源码明确该方法不能运行在main thread context；其show实现把原生创建排队到主线程并异步等待。改为先取得main窗口、parent(&window)、show空回调，保留原信息文案；无main窗口时不另开无父窗口对话框。不把信息Updates菜单算成自动更新功能完成。Wails1.38.3菜单采用AppMenu系统About与WindowMenu标准角色；其基线没有同名Updates自定义入口。

## 新安装包验收

pnpm tauri:build exit0：既有前端lint、WAAPI/单scroll-writer/层次/绑定、CSS/z-index/theme、类型与bundle预算门禁，以及app/DMG签名构建通过。新DMG只读挂载、ditto到新临时安装目录，deep/strict签名exit0。严格clippy all-targets -D warnings exit0；指定新安装sidecar的cargo test结果231通过、0失败、5 ignored，ignored不算通过。

实际LaunchServices隔离explicit启动主窗口在用户指定左屏2：x=-3200/y142/2560×1640/scale2。实际菜单About、Updates均由CUA观察为sheet，nativeAttachedSheet=true，父窗口精确geometry保持左侧；About sequence146、Updates260，观察持续更新。实际OK关闭两sheet后，设置页可打开；Cmd+Q正常kernel/open退出0、sidecar及ready清理。没有操作正式档案、外链或更新下载。

## 最小化排查与范围

修改前46候选另一左屏档案中，截图紫色系统共享控件遮挡交通灯，未猜坐标点击；实际composer点击+CmdM的两条local事件记录均inputMainWasKey=true、actionTargetIsMain=true，但Will/DidMin=0。实际Window/Minimize点击仍无事件，restoreRequests/Completions1/1未增加，nativeMiniaturized=false。快照前不读AX；正常CmdQ清理。这排除了本次“输入时无main target”和“读取AX前已经自动restore”的解释，**未证明系统共享控件就是原因**，也未解决最小化。Tao源码的set_minimized调用NSWindow::miniaturize；当前焦点回调仅在restore_pending时恢复，未找到持续自动反最小化路径，不宣称已查完全部系统/框架因素。

新候选只完成上述对应验收。旧46的文件/保存/剪贴板/显示器/单实例/回退等通过属于历史包，未直接继承。后续仍需当前包相应回归，最小化/窗口间歇稳定、D其余真实验收、官方GUI全资料回退以及A/B/C缺口保持；D未稳定，E完整验收仍待。Windows/Linux沿用用户延期，其他未自行延期。没有push、正式发布或默认下载切换。原失败与超时记录保留。
