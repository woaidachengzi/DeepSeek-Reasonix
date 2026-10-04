# 验收窗口固定左侧屏幕 2，以及不读 AX 的最小化观察

用户要求弹窗不要干扰屏幕 1，并确认屏幕 2 是左侧显示器。后续交互验收先验证父窗口完整位于左屏，再打开弹窗；新私有档案不能沿用默认主屏位置。需要跨屏/主屏专门测试时另行安排，不能无提示干扰用户。

当前候选仍为46c2c9da host/31418d61 sidecar（完整身份见上一份目录多文件证据），源码未改。第一轮explicit私有root `/private/tmp/reasonix-launch-services-jjma6bft`，host92379/sidecar92386。

先在composer焦点后发一次Cmd+M，未随后读取CUA AX，直接读既有原生观察文件：两条localCmdM事件的key和查询target均main，Will/DidMin/DeMin均0。然后刷新AX，Window菜单实际暴露Minimize `performMiniaturize:`，点击后同样不读AX直接保存native快照，仍Will/DidMin/DeMin0。排除本次“后续AX读取立即恢复窗口”的解释，不证明selector调用内部是否发生，不称最小化通过。第一次菜单操作被CUA状态变化保护拒绝，刷新后才操作；截图没有清楚黄色按钮，未猜坐标点击。

用户提出屏幕位置后，用可见标题空白区域拖向左侧未达成目标（geometry x640仍在右屏）；不称拖动成功。正常Cmd+Q、原handle terminal0后，备份该私有档案window-state，仅写入已有恢复格式：x=-3200/y142/width2560/height1640/scale2/maximized=false。没有改系统显示设置或用户日常档案，没有运行脚本注入UI事件。

普通复用重启host92724/sidecar92731，原生快照确实恢复x=-3200/y142，宽2560完全处于左屏[-3840,0]。实际打开目录面板，AX为sheet/open-panel，nativeAttachedSheet=true且父窗口仍是上述左屏几何。Cancel后正常Quit，原handle terminal0、host/openexit0、sidecar/ready无残留，最终window-state保持左屏位置。这证明本夹具的父窗口位置恢复与目录sheet附着，不扩大为所有系统通知/钥匙串/第三方弹窗均能强制指定显示器。

后续当前包交互验收可复用这个已正常完成的私有root；新夹具先保存相同有效左屏位置再启动，并验证实际native bounds。程序窗口矩阵若自行移动到主屏，不得直接在用户学习期间运行。D最小化/首次全屏键与其他缺口仍未完成，E待D稳定；没有发布、push或下载切换。
