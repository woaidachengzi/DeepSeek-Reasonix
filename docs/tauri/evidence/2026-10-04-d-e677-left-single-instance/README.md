# D：e677 当前候选单实例恢复（左屏）

源HEAD e6583af14；产品未改，新工具仅独立启动既有second-instance门禁。安装双SHA在artifact.json：host e6778b8fc3a5bcd7095d75afccedae753a521559fb6dd4896ea72d4100ec0706、sidecar6189151139f5c820c15e4da18419f7ce4bc582d7b8885d3f7227fde8b828cb1b。当前包真实验收，不继承历史46等通过。

## 基线和工具

Wails1.38.3固定singleInstanceID，OnSecondInstanceLaunch→secondInstanceLaunch→showMainWindowFrom("second_instance")；REASONIX_DEV有开发跳过锁例外。Tauri plugin-single-instance回调→tray::show_main_window→实际main focus/unhide。正常打包启动的恢复意图对应，本轮不证明开发例外、跨档案参数路由或remote child singleton策略。

新smoke-single-instance.py要求受限六字段normal模板和全新输出目录；每个managed/explicit新0700fixture exclusive seed0600window-state。另写0600reasonix-native-window-normal.json作为**期望值参考**，没有执行exercise，不把写参考文件算几何通过或完整窗口门禁通过。既有native second-instance本身先要求实际visible/nonminimized/nonmaximized和geometry匹配，再background close并等待applicationHidden；真实第二次启动后要求visible/普通状态、focused、精确geometry及applicationUnhide，最后save。原断言未改。4项已有模板边界回归与Python AST语法检查通过；两种真实包case覆盖新工具正向路径，没有新增只镜像序列化字段的mock测试。

## 当前两例

managed/explicit串行2/2，runner session16868 terminal exit0。原宿主由LaunchServices启动；native窗口background close后，通过真实/usr/bin/open -n -W同包第二次启动。第二次open退出0，原runner核对自有sidecar仍是原单个PID且未替换，并重新验证未认证health拒绝。宿主原生门禁恢复focus、unhide和x=-3200/y142/2560×1640/scale2左屏几何，window-state再次与template完全相同。native phase result均second-instance/ok=true；原宿主kernel wait status0/exit0、未signaled，sidecar/ready清理。两档案durable credential identity不同。

postcheck无匹配Preview/此安装sidecar，strict/deep签名0，双SHA保持。固定退出回执、源码/工具冻结；无HOME/core、ready token、credential identity内容或用户资料归档。测试只操作新建私有fixture。

仅通过上述同档案正常包第二次启动恢复，未证明物理Dock/托盘点击、不同运行档案参数处理、与旧Wails并行目录策略/任意不合作旧writer互斥。最小化/实际菜单Min与间歇窗口稳定仍待，完整显示器场景/其他D、官方GUI全历史附件检查点回退、A/B/C与DeveloperID公证缺口继续列明；D尚未稳定，E完整验收仍待。仅Windows/Linux用户延期，未新增延期/push/正式发布/默认下载切换。
