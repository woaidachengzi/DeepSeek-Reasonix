# 当前候选生成备份实际应用至旧版配置目录

区别于profile restore（仅修改后重启），本轮实际停止Preview及其自有sidecar后，将 d1c9fd5a 导入生成的真实config.toml备份复制到另一个独立私有旧版HOME/.reasonix/config.toml，0600。先核验备份与导入前原件完全相同，再核验恢复文件与备份SHA相等、default_model为原先native-import/alpha。

官方1.38.3 CLI内嵌binary SHA5b1ab314在运行前核验，使用新的私有HOME/core/state/cache、本机拒绝代理，实际config currency退出0，输出CNY。恢复后的整个配置树（文件字节hash/mode/mtime）前后不变，Preview配置/备份树也不变；原导入/修改beta/sidecar重启/宿主退出/explicit拒绝与原目录旧CLI回读仍通过。原件和备份不被回退覆盖。当前app post-run SHA与strict签名保持。

这是由当前候选生成的单份配置备份实际复制应用、旧CLI回读和保护证明，不是Preview GUI一键回退、旧Wails GUI启动、历史会话/附件/所有检查点恢复、Wails共享目录互斥、完整凭据迁移或完整数据快照。之前官方旧GUI历史/附件/检查点证据仍按旧候选范围保留，不自动升级为本轮。wrapper实际执行时临时夹具正常清理；本目录保存源码、回执、日志和摘要，不保存原生用户密码。D其余验收、A/B/C/E门禁仍存在，未发布。
