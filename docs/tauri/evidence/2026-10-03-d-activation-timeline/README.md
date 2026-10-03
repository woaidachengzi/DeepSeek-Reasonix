# 74fefe10 单实例激活时序观测

同一已安装候选74fefe10/sidecar16e3761d，未重建/修改生产代码，不重复浏览器验收。只读观察器记录Bundle/PID、active/hidden及控制台字段；没有窗口标题、屏幕像素或用户输入，没有激活/隐藏/控制其他应用。原生四阶段exercise→restore-maximized→restore-normal→second-instance保持原焦点、几何、持久化与实际kernel退出断言。无重试。

首版 observer-v1：managed4/4原生通过，explicit前三项通过、second-instance失败/kernel2；原生数据是有效失败证据，原窗口正确x920/y344/2000×1400，恢复1/1，主窗口elapsed693ms became-key，708ms resigned-key，709ms application-resigned-active。独立明确复现同包间歇失焦，不证明修复。来源目录和阶段/observer时间在各分目录。

首版只Thread.sleep轮询，没有服务主run loop；getter前台保持另一应用而Preview active变化，前台数据可能陈旧，不能用于判断谁夺焦点。根据Apple [NSRunningApplication](https://developer.apple.com/documentation/appkit/nsrunningapplication?language=objc) 与 [runningApplications](https://developer.apple.com/documentation/appkit/nsworkspace/runningapplications?language=objc) 对run loop更新时间属性的说明，观测器改用主run loop并记录NSWorkspace激活/失活/退出通知；时限采用单调uptime 120秒、50ms采样、仅状态变化输出。首版源码/原数据与失败保留，未洗成通过。

v2-explicit4/4通过：真实宿主93485、第二实例93498，系统事件在unixMs1791019595572激活第二实例；1791019595641收到第二实例退出及主实例激活，1791019595707前台getter与主实例active一致。主实例保持至正常退出，4份kernel exit0、同profile identity/精确普通状态保持。系统通知和getter属于非原子异步观测，允许过渡片段状态不同，不以getter瞬态取代原生窗口断言。

此成功样本提供第二实例退出后的激活序列，尚未在修正后的观察器下捕获失败序列。结合v1原生短暂key后失焦，下一项是检验按第二实例退出事件协调恢复，不能直接宣布因果或扩大为完整窗口48阶段通过。没有应用新的生产窗口策略，也没有放宽2px持久化断言；该问题维持此前失败证据。

控制台loginDone/onConsole=true；锁定字段缺失/null，不能推断屏幕未锁。getter与事件中另一应用Bundle仅作本地定位元数据，不据此认定用户操作或该应用抢焦点。CUA仍无可靠可用界面会话，本轮未调用CUA。D未完成/E未开启，真实授权/物理UI和其他清单缺口保持，未发布/切换下载/推送。
