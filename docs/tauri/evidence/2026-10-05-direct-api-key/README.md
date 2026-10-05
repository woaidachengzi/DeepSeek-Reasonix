# API Key 直接保存并使用

用户明确要求普通添加模型不依赖系统钥匙串。现采用 Reasonix/Wails 已有全局 .env 凭据机制，在 Preview 独立 profile 中持久化 API Key；保存后立即由 core 读取，重启可恢复。文件为本地明文 .env，原子写入、0600 权限、现有凭据锁和目录边界保护。用户 API Key 未读取、未迁移、未打印或发送；仅使用隔离模拟密钥验收。新本地保存入口为受 token 鉴权/idempotency 的 POST /v1/settings/provider-api-key，原生 save/clear_provider_api_key 限 main 窗口，WebView 不得知桥接 token；服务名/访问表/凭据来源、长度/换行注入边界检查，返回脱敏摘要。配置里只保留 APIKeyEnv 引用，密钥写 .env。清除后移除本地值和旧 runtime override，仍保留用户原钥匙串记录；新保存值优先于此前 runtime keychain overlay。

添加模型编辑器和现有服务卡片改调用本地 API Key 保存，保存失败保留草稿/可重试；普通路径不调用 keychain_save/delete。Startup 与 restart_bridge 删除自动钥匙串恢复，避免仅打开应用就触发密码弹窗。旧版凭据迁移收进可选折叠入口，仅用户主动触发时读原来源，随后写入本地凭据以供重启使用。遗留 native keychain commands 仍可显式使用，启动时不读取；旧系统钥匙串仅有的 Key 需要用户重新填写或主动迁移。本次未替换 Applications 或访问用户系统钥匙串。

Go bridge/internal/config 回归通过：无授权写入拒绝、idempotency、0600、本地保存后清空进程环境再读仍就绪、选择另一模型、旧内存覆盖移除、无效来源/空值/换行注入拒绝、清除后的未就绪与密钥不回传。组件测试覆盖正常添加、已有卡片保存、重试和模型选择器；完整前端生产 build、Rust keychain 24 个隔离单测和 clippy --all-targets -D warnings 通过。CUA 隔离真实组件 + mock host，5197/direct-key-qa.html：选择 MiMo、填写模拟 API Key、保存、模型就绪、选择 mimo-v2.6-flash 成功，console error/warn 空；截图 browser-direct-key.png。页面、server、tab 已清理。真实 MiMo 付费连接未调用。

本轮按用户要求调整 D 凭据默认方案；钥匙串保留显式迁移路径，其他 D/E/A/B/C 验收缺口保持，不能据此宣布迁移完成或正式发布。原候选包可回退；本轮继续 app-only，不生成 ZIP/DMG。
