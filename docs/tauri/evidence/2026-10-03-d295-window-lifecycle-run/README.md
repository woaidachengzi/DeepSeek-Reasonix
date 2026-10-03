# d295 当前候选完整窗口失败（2026-10-03）

同一实际安装包 host `d29503c849e5303eca92054bc8a6d4010e2bee1eaea90deb07d6ccc54a9c0822`，原 verify-installed-d 顺序执行 identity/lifetime/startup/window。前三区组 exit 0，window exit 1；包 hash、严格签名和脚本依赖身份由 runner 核对并冻结。

身份损坏/冲突/链接拒绝及恢复通过；两种档案 SIGTERM/SIGKILL、空闲/流式任务退出与原件保护、同档案重启通过；启动前/令牌前/readiness 前中断与清理通过。

完整窗口 managed 首个 exercise 阶段最小化超时，completed stages: 0。nativeMiniaturized=false、didMiniaturize=0、willMiniaturize=0，窗口仍 visible。流程包含调整至 1000×700 逻辑尺寸、保存、隐藏、重新显示后调用原 Tauri minimize。原固定 trace 与完整失败日志保留；没有放宽超时或重试失败样本。后续窗口阶段及 explicit 完整流程未执行。

私有现场 `/private/tmp/reasonix-native-window-smoke-8ayef1cy` 保留。独立只读进程核对无运行中的 Preview 或该现场进程，见 cleanup.json。此前八阶段独立最小化切片成功不能覆盖这次失败，也不能据此认定几何或系统动画为根因。D 窗口稳定性未验收，E 仍未开启；当前 GUI/其余门禁仍待完成。
