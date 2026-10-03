# LaunchServices 完整窗口首轮：托管通过，显式参数拒绝

当前真实安装候选c71ab66c未改变。完整窗口通过LaunchServices启动，托管24/24阶段通过，含Settings实际焦点/最小化/恢复、外观持久化与回滚、后台任务/退出、原生剪贴板与菜单编辑、对话框取消、第二实例和close行为；每阶段实际宿主kernel exit0。

显式档案首阶段因适配器把未设置的REASONIX_STATE_HOME作为空环境变量传给open，现有`check_sidecar_profile`报“sidecar retained an inherited state override”，未开始验收；清理真实host kernel signal9，不能作为正常退出通过。失败现场`/private/tmp/reasonix-native-window-smoke-4fi_bak9`保留；首轮[source/脚本/结果](result.json)与[日志](window.log)不修改。

修复仅保留环境变量的“未设置”语义，未放宽状态覆盖拒绝、焦点、最小化、退出或私有档案断言。修改后两档案完整48阶段重新验收，见[独立复验](../2026-10-03-d-c71-launch-services-window-fixed/result.json)。本目录不标记完整窗口通过，不代替物理UI、不同缩放/拔插或钥匙串授权验收。浏览器没有重复测试。
