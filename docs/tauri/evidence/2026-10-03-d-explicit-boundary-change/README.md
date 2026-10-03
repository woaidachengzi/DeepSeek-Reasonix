# 显式 Preview 存储拒绝默认正式版目录

对照 Wails 1.38.3 internal/config/paths.go：默认 home/state 为 ~/.reasonix，macOS旧 support 为 ~/Library/Application Support/reasonix，默认缓存为 ~/Library/Caches/reasonix。旧版不遵守Preview新协议，原先显式 REASONIX_HOME 无边界检查。

新启动检查在创建主窗口和 sidecar 之前，对 home/state/cache 拒绝这些默认树的重叠（相同、祖先、子目录及现存符号链接别名）。仅规范化路径用于比较，不重写调用者环境值，不创建目标。未展开的 ~/ 或 ${VAR} 和外围空白拒绝并提示使用解析后的路径，避免 Rust 与 Go 选址不一致；这是明确的 Preview 输入限制。托管档案仍清除继承的 state/cache。18项源回归及strict Clippy通过。

首版 d904651a 十种危险配置拒绝通过，但package managed通过后explicit的环境路径严格检查失败；首版源码patch、日志和not-run门禁保存在../2026-10-03-d-explicit-boundary-package-run，不删除。移除环境路径改写后重建65560c94，完整app/DMG、只读安装strict签名通过。最终verify-installed-d的explicit-boundary/package/boundary/profile四组全部通过，原件字节/权限/mtime/inode/别名保持、拒绝无sidecar/readiness；正常托管/显式目录、配置导入后修改重启与备份保护通过。profile restore不是应用备份。

这只保护已知默认正式版根；任意自定义Wails目录、旧版后来主动选到Preview目录、外部目录并发重定向等不能据此认定互斥通过。窗口、生命周期、实际钥匙串/复制/托盘/通知等仍须新655候选复验，不继承d1证据。单份配置实际应用旧CLI回读的旧证据也需按包区分。D未完成/E未开启，仍为ad-hoc候选，未正式发布。
