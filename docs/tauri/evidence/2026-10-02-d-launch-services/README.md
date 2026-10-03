# 2026-10-02 正常 LaunchServices 启动与实际文件确认

使用同一真实安装包 host `317d96bb0df1806bf23a09ac83439084a36604fd8fb35fa072e23c62ec5f39fb`，源码/构建和只读安装归属 `../2026-10-02-d-tray-caller/`。本轮没有重建或修改产品源码，签名复核通过。

## 正常 .app 启动

新 probe 通过 `/usr/bin/open -n -W` 的 LaunchServices 路径及明确 private HOME/TMPDIR/核心目录启动，原 executable runner 保留。只向命令行传入非秘密目录/模式；实际 sidecar ready-file 和父 PID 绑定 host，检查实际档案继承、app data、凭据身份和 401。kernel host exit 回执要求正常码 0，同时要求 open 返回 0、sidecar 和 ready 文件消失。两种档案通过且身份不同；不是只看 open 返回值或把它的 PID 当 native host。

首轮托管成功，但显式夹具把空 REASONIX_STATE_HOME 和 event override 放入 env，违反显式档案继承检查而失败。首次日志保留；纠正夹具不传入这些无意覆盖，用新根重新执行原检查后两档案均通过，没有修改产品语义或放宽断言。

## 实际产品 UI

LaunchServices 以新的显式私有档案正常启动，CUA 绑定成功（此前 -3811 记录保留，不能从这一次恢复认定启动上下文是根因）。实际产品附件按钮→原生文件面板→Go to Folder→私有 workspace→选中唯一的 40 字节 canary.txt→Open，工作区显示“canary.txt 待发送”和移除按钮。再开面板并 Cancel 后仍保留同一个附件和空输入；移除草稿后 Send 禁用。固定源文件字节不变，没有发送模型请求。

实际 Cmd+Q 后，原私有 host/sidecar/open 三个已确认 PID 均消失，ready 文件为零；没有向这三个 PID 发 shell 信号，故这项是产品退出验收。仅为这一个显式档案的空闲用户退出，不代替托盘退出、关闭后运行任务或其他档案。

## 退出后的观察隔离问题

Cmd+Q 后再读取 CUA 窗口，观察转到了同一复制测试包的新默认档案实例，origin 与原私有 origin 不同。立即停止 UI 操作；没有操作该实例的设置、会话或模型。以精确私有复制包的 executable 路径及单个子进程核对后 SIGTERM 清理该新实例。没有证明后台启动完全不写默认档案，因此不声称用户数据原件已核对不变；也不把这次清理计入原私有 Cmd+Q 验收。后续仅对仍存活的私有 PID/对应 origin 操作，退出后使用 PID/kernel 回执，不再向 dead binding 读取窗口。

原始 CUA 观察在任务工具记录；本目录只保存脱敏总结及固定私有夹具，不收录原生默认 Documents 列表或退出后的默认档案 UI 树。发生过 -3811 与本次 retarget 的事实均保留。此验收仅证明新对话文件选择/取消/移除路径，不证明模型送入、持久附件拷贝、多个文件、用户托盘弹出或所有对话框均通过。

D 未结案，E 未推进；实际窗口/托盘、通知、钥匙串、不同缩放/物理拔插、历史数据回退与旧 Wails 生命周期互斥、A/B/C 及正式签名/公证/发布授权缺口保留。没有改系统设置或默认下载项。
