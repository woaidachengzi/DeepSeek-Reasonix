# 当前真实包目录/文件选择与保存覆盖

当前host249dfd15、sidecar29d985c7，私有DMG安装副本使用explicit隔离档案，PID20690/sidecar20698，启动前strict签名与档案检查通过。全程通过CUA实际原生面板操作，未注入选中路径或伪造callback。

1. 目录面板真实Go To/Open选择本轮workspace，界面项目及默认工作区返回精确私有路径。
2. 文件面板Go To/Open选中本轮canary.txt（40字节），单个待发送附件显示精确路径。没有发送模型请求；取消再次打开的文件/目录面板保留原工作区和已有附件；移除本轮附件后Send禁用。
3. 录制最小前端诊断，实际Save面板保存到`保存 中文 & space/dialog-save.json`。文件可解析为diagnostic schema，权限0600。第二份导出选择同一目标，真实原生覆盖警告出现；先Cancel，原件hash/size/mode/mtime全部不变；再Save/Replace，写出不同有效JSON且0600，首份完整备份保存。
4. canary仍等于夹具生成字节，保存阶段前后的hash/size/mode/mtime不变。诊断开关已关闭，测试草稿已清空。实际Cmd+Q kernel0/open0，sidecar/readiness无残留，runner0。退出后未读取死绑定。

动作/面板目标/返回UI事实见actions.json，文件证明见两份JSON与manifest，进程证明见result/exit-receipt。初始面板可能显示系统记忆的Documents/旧GoTo路径，随后由真实用户界面明确导航到本轮私有目标；只归档私有结果，不复制无关目录列表。

源码参考：冻结Wails目录OpenDirectoryDialog与诊断SaveFileDialog；当前Tauri directory/single选择、file/multiple选择及Rust诊断save。renderer权限仅dialog:allow-open，保存仍由受限host命令发起，无新增fs权限。本轮没有源码修改，无需为相同候选重复构建。

范围：一份explicit档案，单文件/目录确认与取消、诊断JSON保存/覆盖取消/覆盖确认。尚未证明多文件/过滤器/保存失败/托管档案同矩阵、主题导入导出/会话文件Save等全部入口。Wails诊断ZIP与当前Preview JSON的完整格式等价性也不由本轮证明。D整体和最小化/窗口稳定性仍未完成，E未推进。
