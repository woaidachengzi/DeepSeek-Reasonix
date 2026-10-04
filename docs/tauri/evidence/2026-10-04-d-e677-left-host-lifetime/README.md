# D：e677 当前包宿主死亡、流式任务与重启

源HEAD3c8657989，产品未改；仅增加测试窗口模板和逐例固定回执。安装包host e6778b8fc3a5bcd7095d75afccedae753a521559fb6dd4896ea72d4100ec0706、sidecar6189151139f5c820c15e4da18419f7ce4bc582d7b8885d3f7227fde8b828cb1b，artifact.json绑定exact安装路径。本轮使用当前真实安装包，没有继承46旧包结果。

## 基线与实现

Wails1.38.3 shutdown先冻结publication、取消tab builds/catalog，再进入shutdownBody；后者还停止bot/remote/tray/terminal等，属于正常应用退出（源码冻结）。Tauri新增独立sidecar，需要额外保护宿主突然死亡的边界；Go native lifetime依据host PID/PPID和私有lease观察取消，进入core清理。本轮验证宿主突然退出时sidecar与正在执行的provider流自行收尾，**不把它当成Wails所有normal quit、bot/remote/terminal等生命周期等价**。

## 当前包八例

managed/explicit × SIGTERM/SIGKILL × idle/actually streaming，固定八例串行，runner session24664最终exit0。每次验证自己的same-UID host/child PID与process identity、实际private ready/身份目录、未认证health拒绝，之后才向唯一自有host发送信号；sidecar没有被runner驱动成功退出。

八例host返回码分别-15/-9，kqueue记录自有sidecar NOTE_EXIT/EXITSTATUS，全部原始wait status0/正常exit0；sidecar、ready及readiness目录清理通过。流式四例先核对host观察实际text与taskRunning=true、恰一次本机provider请求，再杀host；provider.verify_host_death断言通过。各case的canary、credential profile/backup以及已有config的bytes/mode/mtimeNs保持；同档案实际重启bridge+菜单快捷键合同成功，durable credential identity保持，原件再次保持。重启证明这些测试目录锁可重获，不证明任意旧不合作Wails目录互斥。

postcheck无匹配Preview或此安装sidecar，strict/deep签名exit0，安装双SHA前后保持。exact八例矩阵/四真实流/八正常sidecar退出/八重启独立核对通过，verification.json仅汇总这些已保存回执，不代替运行日志/源码。

## 左屏与工具边界

新增可选--window-state-template/--output，默认参数调用保留。每个全新0700fixture用已有受限模板exclusive seed0600normal window-state，输出目录必须全新；仅固定JSON回执，不归档HOME/core、credential文件、模型请求或输出内容。通用模板4项边界回归与Python语法检查通过；本轮八真实case是新工具实际正向验证。

idle四例使用现有interactive-observe只读native采样，杀host前要求x=-3200/y142/2560×1640/scale2实际一致。streaming四例保留已有task-host-death探针，不替换为闲置观察；窗口仅预置同一左屏normal state，nativeIdleGeometry=null，**未采样其实际GUI位置或最后重启窗口位置，不宣称这些视觉位置已验收**。本轮没有文件/钥匙串/通知弹窗操作或主屏移位矩阵，也没有CUA输入。无权限扩大、原门禁放宽或产品修改。

最小化/实际菜单输入及间歇窗口稳定仍未解决；其余D真实验收、当前包官方GUI全历史/附件/检查点回退、A/B/C与DeveloperID/公证缺口保持，E完整验收仍待D稳定。仅沿用Windows/Linux延期，不自行延期其他；未push/正式发布/默认下载切换。
