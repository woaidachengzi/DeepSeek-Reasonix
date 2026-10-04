# D：左屏关闭行为与普通重启

当前46安装包，产品未改。用户确认屏幕2为左侧；两份explicit隔离档案均以受限窗口模板启动，x=-3200/y142/w2560/h1640/scale2，实际native观察确认。没有操作正式档案。

第一份：实际关闭按钮在默认“保持后台运行”下隐藏应用，host95073/sidecar95080仍存活；实际Window → Show Reasonix后visible=true、hidden=false，restore2/2，左侧geometry不变。随后设置退出单选项成功，但最后Close被CUA状态保护拒绝，监控300秒超时/exit1并清理，因此这次不算关闭即退出通过。

第二份：设置中实际选择“退出 Reasonix”，点击原生关闭按钮；kernel退出0、open退出0、sidecar与ready无残留。普通重启同一私有档案与origin，左侧geometry恢复，设置单选项仍为退出；再次实际关闭，退出0且清理通过。偏好固定字段closeBehavior=quit，当前文件mode0644（不宣称0600）。六个自有host/sidecar PID最终均不存在；安装包deep/strict签名验证exit0。

## 基线与范围

Wails beforeClose对应background分支先保存窗口/快照tabs并校验恢复路径，再隐藏；quit返回false交给退出。Tauri对应CloseRequested由keep_running隐藏、quit app.exit(0)。本轮核对实际空闲主窗口的两种用户意图和配置持久化，未覆盖Wails运行中会话快照、remote窗口、托盘点击、managed档案或其他平台，也没有证明流式运行关闭与回退等价。

最小化和间歇窗口稳定缺口保持；D尚未全验收，E完整验收仍待，A/B/C、官方GUI全资料回退、签名公证等发布阻碍仍按迁移清单追踪。没有新增延期、push、正式发布或默认下载切换。仅归档固定观察、退出回执和基线片段，不归档私有HOME/core或完整偏好。
