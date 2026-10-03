# 单实例退出协调：review 拒绝，生产改动已撤回

本轮尝试在单实例监听器之前注册主线程 NSWorkspace 退出观察器；收到重复启动通知后保留同 Bundle ID、同 executable 文件名的 NSRunningApplication，待其全部退出再恢复主窗口。无定时重试、超时放宽或几何容差变更。实验源码与完整 patch 在 single_instance_restore.rs、gates/source.patch；HEAD 4ba805d8a 加未提交源码构建，不能当作该提交的干净二进制。

## Review 发现

[P1] request() 只按 Bundle ID 与 executable 文件名收集 peers，没有确认哪个进程发出了本次单实例通知，也没有绑定数据档案身份。其他档案合法常驻的 Preview 实例也会进入等待集合，reconcile() 只有集合全部退出才能调用 show_main_window。观察器没有独立完成边界，可能导致重复启动长期不恢复。主线程限定与退出清理虽正确，但不能消除这一行为缺陷。当前完整回归 restoreRequests/Completions=0 与等待未完成相符；未记录失败时 peers 清单，不能断言此缺陷就是该次故障根因。

## 验证范围

最终源码 strict clippy 通过，Rust 227 passed / 5 ignored；真实 bridge 使用此前安装包的同版 Go 源码产物，不是新 sidecar 的专项验收。app/DMG 构建、只读复制安装与严格签名通过。安装 host f120f29d7e2f55b891de69d9688ee04d4384842b9205779c16e11fef78e50f44、sidecar 0a64f86b4fe8e235a224079c07a85ce14802cff5433f90d457b4f09488e54bb2、DMG 631c3c4ad5b0b4b21c3a5a3061e196a88c965b917ee344d97284f82154a2e97a。完整安装位置与摘要在 install.json。

先行 managed/explicit 私有档案各4阶段（exercise/restore-maximized/restore-normal/second-instance），共8/8通过；主线程只读激活事件与内核回执存于两个 control 目录。这些对照没有证明稳定性。随后固定同包运行 package、startup、failure-exit 均通过；完整 window managed 已完成21阶段，second-instance 超时，主宿主98796 kernel exit2，active/key/visible=false，restoreRequests=0、restoreCompletions=0，几何x920/y344/2000×1400正确。explicit 完整矩阵与 lifetime 未运行，程序在首个失败停止。现场 /private/tmp/reasonix-native-window-smoke-6oc569mj 留存；原生结果及所有22个窗口内核回执已归档。未重复相同门禁以覆盖失败。

## 处理与后续

失败方案不进入生产提交：恢复 Cargo.lock/Cargo.toml/main.rs 至 HEAD，删除新增生产模块；源码及编译初期失败、最终回归、构建安装、原生成功/失败和运行脚本均保存。归档排除 HOME、私有钥匙串、用户剪贴板快照与二进制。manifest.json 校验归档原始证据（不含此说明）。撤回后生产源码与此前提交一致，文档检查无需重跑原生矩阵；刚构建的 f120 包仍是失败实验，不能称作恢复后源码产物或稳定候选。

迁移候选仍为此前74fefe10，其验收范围与失败不变。单实例恢复和2px精确持久化仍未解决；D未完成，E未开启。A/B/C、物理UI与授权、多显示器恢复、钥匙串等阻碍保持迁移清单记录。未发布、切换下载、推送；此次提交仅保存 review 和失败证据。
