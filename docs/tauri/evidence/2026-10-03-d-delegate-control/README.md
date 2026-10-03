# 应用 delegate 对照：激活前提失败

本批只扩展独立 window-baseline example：通过显式私有环境参数，在 Ready 后第一份
主线程采样中对 NSApplication 调用 setDelegate(None)。Tao 仍持有原 delegate，事件循环
不替换；每个快照核对 delegatePresent。不是生产初始化/修复策略。

首次尝试读取协议对象 class 编译失败；改为读取 AppKit 返回的 delegate 是否存在，
release build 与 example Clippy -D warnings 通过，首错日志保留。

同二进制 `41e5bbe8fc968abee54b56b4f043e690ef61a6a50df34d62f6c68ba7c1d0069f`，
三个新私有档案，经 LaunchServices 启动，不访问默认配置、剪贴板或系统设置。
实际应用类均 TaoApp，普通 Cocoa 窗口，所有状态在主线程读取。

| 样本 | PID | 实际 delegatePresent | 前提 | kernel/open | 判定 |
| --- | --- | --- | --- | --- | --- |
| keep 首轮 | 5699 | true，每个快照 | active/key=false、visible=true | 2/0 | 前提失败 |
| keep 新档案复验 | 5780 | true，每个快照 | 同上 | 2/0 | 前提失败 |
| none 单独样本 | 5831 | false，每个快照 | 同上 | 2/0 | 前提失败 |

keep 失败时 runner 停止，未运行后续 none；随后 none 用独立 runner 执行。
三个 runner 均 exit 1，未放宽 active/key/visible/page 原断言。样本中的最小化 false
不能解释为满足前提后的 delegate 结论，也不能与上一批有效前提样本拼接成控制实验。
全部确切 PID 已确认退出，无 sidecar。

这批只能说明：移除 delegate 本身没有使该样本取得 active/key。尚未判断 delegate
是否影响最小化；应用激活、时机及宿主环境需要继续检查。保留失败记录，不继续以相同
无效前提重复采样、不宣称窗口根因已定位。生产 host 15fcd43b 未改，D 未验收、E 未推进。
