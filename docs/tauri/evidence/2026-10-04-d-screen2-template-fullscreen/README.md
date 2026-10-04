# D：全新左屏档案启动与首次全屏快捷键往返

基于 `4ee186ff4`，仅验收启动器/其边界测试改变，生产源码、权限、依赖均未改。继续使用当前真实46包，host `46c2c9da5f1d10a0c92a5d319b465b381a36256e9e770870e6b6933ec13c5256`、sidecar `31418d61fb58823fabe12e0b448f9d005eae4566381e0ff3ca868ef2c11d1fd4`。没有重新构建或继承其他包结果。

## 启动器改动与边界

新增 `--window-state-template`，仅允许全新 `--interactive --observe` 档案，禁止与 reuse-root/native-phase/failure-exit 混用。模板只读取六个既有normal geometry字段，限制普通文件/大小、坐标i32、尺寸/scale、非maximized，拒绝其他档案字段与符号链接；模板不复制原目录、凭据或配置。

在新建0700私有root的app_data中，exclusive create保存0600window-state；不存在才创建目录，不覆盖已有状态。启动后在15秒有界观察中确认实际native geometry精确等于请求，才输出initialWindowGeometry并进入交互验收；写入文件本身不算恢复通过。失败仍fatal/原清理路径，不扩大产品权限，不移动用户窗口，不放宽已有窗口验收断言。此选项只配置初始窗口，不能保证后续测试动作不会主动移屏。

2项回归通过：负坐标恢复格式/目录文件权限/原模板字节保持/拒绝覆盖；拒绝额外token字段、布尔宽度、越界坐标、NaN/Inf scale、最大化/非法高度和symlink。结果在reasonix-window-template-tests.log中。

## 当前安装包物理验收

命令为 `probe-launch-services-profile.py <current46-app> --interactive --observe --window-state-template /private/tmp/reasonix-screen2-window-state.json --wait-seconds 300`。全新explicit root `/private/tmp/reasonix-launch-services-5ddhnqhu`，host93180/sidecar93187；先证明native x=-3200/y142/2560×1640/scale2，完整处于用户确认的左屏2。CUA绑定当前存活exact app与新private origin `reasonix-preview://67faf3380658c0e6783ad058a5e5ee60.localhost/`。

- 初次原生minimize按钮AX点击未见Will/DidMin，nativeMiniaturized=false；不判定按钮selector已被内部执行。
- composer点击/AX确认焦点后，一次Ctrl+Cmd+F：local匹配事件2，每条key/查询target均main；**WillEnter=1、DidEnter=1、nativeFullscreen=true**，全屏geometry为左屏x=-3840/y0/3840×2160/scale2。
- 刷新全屏AX后一次同快捷键退出：**WillExit=1、DidExit=1、nativeFullscreen=false**，精确恢复原x=-3200/y142/2560×1640/scale2。这次第一次键入即成功，没有重发或用菜单替代进入。仍不根据重复local事件猜测输入来源。
- 恢复后composer焦点，再一次Cmd+M：local匹配2，Will/DidMin仍0、nativeMiniaturized=false。动作后保存原生快照前不读AX，继续保持失败。
- 实际Cmd+Q，原session69055 terminal0，kernel/openexit0、sidecar/ready清理。postcheck strict signature0、两SHA保持、exact host不存活、无Preview与私有sidecar/ready残留。

固定native快照、退出/位置回执及冻结工具归档，没有私有HOME/core、剪贴板、keychain、二进制或其他用户窗口。此记录仅证明一份explicit左屏档案的首次全屏键往返，**不将此前同包其他现场失败改为通过，不证明间歇根因已解决或全D稳定**。mixed-scale/拔屏/零显示器等仍待；最小化保持未完成，E完整验收仍待D稳定，A/B/C及正式签名/公证/完整官方GUI回退缺口不变。无新增延期、push、正式发布或默认下载切换。
