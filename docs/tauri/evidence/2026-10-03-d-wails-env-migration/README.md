# Wails 全局 .env 显式凭据迁移

对照 Wails 的 `UserCredentialsPath` / `StageModelCredentialLocked`，新增设置页“迁移 Wails 全局凭据”。renderer 只提交 provider 和固定来源 `wails-env`，不接收密钥或提交文件路径、钥匙串账号；鉴权 sidecar 原生专用入口只读固定原 HOME/.reasonix 配置与全局 .env，按同名 provider、APIKeyEnv、网络与认证目标匹配来源。

已有目标钥匙串或文件凭据时拒绝覆盖。无来源、格式/范围错误、同步失败沿用固定错误及原生事务回滚；草稿和原件保持。旧配置使用同一无凭据配置规范化规则解析已限大小的内存快照，不再次读取来源路径、不加载 shell/project env、不写迁移。支持 Wails 的 BOM/UTF-16 文件编码。

源码回归：Go provider 测试与完整 internal/config 测试通过；真实 sidecar 的 Rust keychain 事务 25 项通过、3 项默认忽略，其中新增全局来源导入/拒绝目标文件覆盖/同档案重启/删除/来源字节与元数据保护。该测试使用内存凭据后端，不能代替 OS 钥匙串验收。前端 API key 组件、typecheck、完整 test:tauri 与严格 clippy 通过。隐藏 WebView 检查新增来源 wails-env 的普通/伪造 main 请求，已在当前包门禁实际通过。

保留中间 provider 默认字段污染失败、后续修复，以及普通 sandbox 的 loopback/Go 缓存拒绝；在已授权本机环境执行后通过。界面自动化 getState 再次 30 秒超时并重置，不能标记真实设置页或系统授权通过。

规范化后的生产 app/DMG 构建通过，DMG 只读挂载复制安装与严格签名验证通过。安装宿主 `c71ab66c727b1dc71854a993ba232f6515975328f684e365d3fc9bef801f2707`、sidecar `94207d451469c99cd25439e8107244f4a19a5c6da349668a111db98fa911a9fd`、DMG `7b6a7a5f6f93e568af8a5cbc7306e9e72c107d9b88ddac40245eaf5cb7bfe154`；见 [安装记录](install.json)。当前包14个不同程序门禁通过，包括双屏8项；完整窗口在托管8阶段后Settings活动key窗口前提失败，显式未运行，零活动显示器条件门禁不适用。见[首轮](../2026-10-03-d-wails-env-package-run/README.md)、[续跑](../2026-10-03-d-wails-env-package-continuation/README.md)与[双屏](../2026-10-03-d-wails-env-displays/README.md)。[官方Wails资料回退](../2026-10-03-d-wails-env-legacy-rollback/README.md)和[配置备份实际恢复](../2026-10-03-d-wails-env-backup-apply-legacy-control/README.md)均在当前包通过。测试后两包严格签名通过、无匹配运行实例。历史 a62 的通过不继承到新候选。D 未完成、E 未开启；未正式发布或切换默认下载项。

后续同包[启动方式对照](../2026-10-03-d-c71-activation-context-control/README.md)及[LaunchServices完整窗口](../2026-10-03-d-c71-launch-services-window-fixed/README.md)通过：双档案48/48、全部实际kernel exit0，累计15个不同程序门禁。上面的裸宿主失败与[适配器空环境参数失败](../2026-10-03-d-c71-launch-services-window/README.md)保留为历史现场。生产代码与包身份未改，未重复浏览器验收；真实OS全局来源迁移、物理UI等待办保持，D未完成/E未开启。
