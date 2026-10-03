# 实际激活尝试：UI 绑定超时，未执行点击

独立 example 增加显式手动激活模式：Ready 写入私有 ready.json，最多等待 60 秒的
start.json；此前不执行最小化样本。主线程状态增加 activation policy 与 manual 标识。
这只是诊断流程，不修改生产应用或系统设置。

样本 binary `57955cbd13ed18d3c67b62f47f7a3cc594bad8d426431721c8f42029a83afb50`，
私有 root `/private/tmp/reasonix-tauri-window-baseline-lsh8zpf9/keep-tao-cocoa-standard-direct`，
PID 6133。release build 通过。

CUA 绑定该存活 app 返回 -10005 timeoutReached，工具报告耗时 7560.9098 秒。
未完成窗口点击，也未写入 start.json。应用自己的 60 秒期限已结束，result.json
记录 manualActivationTimeout=true；随后进程检查确认原 PID 不存在，未重绑失效 app。

runner 在解析该超时结果时 KeyError(restored)，退出 1；kqueue 数据没有保存为回执，
**本样本不宣称 kernel 退出码通过**。原结果、日志及 UI 错误元数据保留。
已修复源码超时结果的统一字段与 runner 缺字段读取，example Clippy 通过；修复后的
完整实际超时回归尚未运行，不将代码检查代替运行结果。

手动激活与 delegate/最小化比较均未验收。生产 host 未改；后续若恢复 UI 验收，应对
工具调用设置更短的显式 timeout，先确认确切 PID 活着，再绑定，不读取失效 app。
本次没有系统剪贴板或用户默认档案操作。D 未结案、E 未推进。
