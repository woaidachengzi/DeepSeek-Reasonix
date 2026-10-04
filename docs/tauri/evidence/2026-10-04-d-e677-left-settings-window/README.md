# D：e677 当前包左屏 Settings 门禁和实际按键

产品未改；仍为2297f7981对应e677安装候选。host e6778b8fc3a5bcd7095d75afccedae753a521559fb6dd4896ea72d4100ec0706、sidecar6189151139f5c820c15e4da18419f7ce4bc582d7b8885d3f7227fde8b828cb1b。没有继承46旧包通过记录。

## 固定左屏门禁

LaunchServices启动器的受限六字段normal模板现在也允许三个既有Settings native phase：hidden、Tauri API minimized、native menu minimized。拒绝reuse/failure/interactive混用和其他主动移屏phase；原窗口尺寸、身份/sidecar认证、主窗口就绪、active/key、最小化、原生事件、Settings恰一次派发、恢复、kernel退出及清理条件保持。

native phase不使用不存在的interactive-observation文件作为就绪依据，而在正常退出后严格检查该phase的native trace：settings-shown必须精确匹配模板，两个minimized phase还必须具有settings-minimize-ready/settings-minimized同一精确geometry；缺stage、位置错或geometry损坏则失败，不只相信seed文件。仅这三种不主动移动/缩放窗口的phase可用。交互模式仍保留启动后实际native geometry一致才允许验收的15秒检查。

4项边界回归通过，覆盖原模板格式/私有权限/禁止覆盖、合法及被禁止组合、native轨迹stage缺失/错屏拒绝。新增断言不削弱原门禁，模板没有复制私有配置或凭据。

当前e677安装包三个phase串行各跑managed/explicit，共6/6 exit0。原生菜单最小化含Will/DidMin及DidDeminiaturize完成断言；Tauri API路径也实际minimized后Settings恢复；hidden路径窗口hide后Settings恢复。所有显示/最小化观察geometry均为左屏x=-3200/y142/2560×1640/scale2。各自kernel/open退出0、sidecar/ready清理。原门禁不记录恢复后最终native geometry，此处不额外宣称该最终几何实际验收。

## 当前候选实际输入

另一全新explicit隔离档案，host98483/sidecar98491，左屏初始geometry精确确认。CUA实际Window AX Raise，composer click→CmdM，两条local事件均main key/target main，但Will/DidMin仍0、nativeMiniaturized=false、restore1/1；快照前没有读AX。

首次Ctrl+Cmd+F实际进入Will/DidEnter1/1，全屏左屏x=-3840/y0/3840×2160；退出同键Will/DidExit1/1，恢复x=-3200/y142/2560×1640/scale2。实际Window→Minimize点击仍无原生Min事件且restore1/1未增；不根据AX点击返回成功宣称AppKit内部动作完成。普通CmdQ kernel/open0且cleanup。未用截图、猜测黄色按钮坐标或更改系统共享状态。

因此6项程序门禁通过仅证明本包这些受控调用路径，**实际CmdM/菜单点击仍未验收**。输入已到达时key/main target正确、读取AX前未自动restore，这两种解释不能支持本次失败；并没有证明系统共享控件或某个框架就是根因。一份实际全屏往返通过也不能覆盖间歇稳定、mixed-scale/拔屏/零显示器等。

其他D、当前候选clipboard/文件/通知/keychain/单实例/互斥/全资料回退仍按清单待验；E完整验收仍待D稳定，A/B/C/正式签名公证缺口保留。仅Windows/Linux用户延期，不自行延期其他。固定快照/回执/工具归档，不归档HOME/core或凭据。未push/发布/切换默认下载。
