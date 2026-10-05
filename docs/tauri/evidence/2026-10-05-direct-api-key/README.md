# API Key 直接保存并使用

用户明确要求普通添加模型不依赖系统钥匙串。现采用 Reasonix/Wails 已有全局 .env 凭据机制，在 Preview 独立 profile 中持久化 API Key；保存后立即由 core 读取，重启可恢复。文件为本地明文 .env，原子写入、0600 权限、现有凭据锁和目录边界保护。用户 API Key 未读取、未迁移、未打印或发送；仅使用隔离模拟密钥验收。新本地保存入口为受 token 鉴权/idempotency 的 POST /v1/settings/provider-api-key，原生 save/clear_provider_api_key 限 main 窗口，WebView 不得知桥接 token；服务名/访问表/凭据来源、长度/换行注入边界检查，返回脱敏摘要。配置里只保留 APIKeyEnv 引用，密钥写 .env。清除后移除本地值和旧 runtime override，仍保留用户原钥匙串记录；新保存值优先于此前 runtime keychain overlay。

添加模型编辑器和现有服务卡片改调用本地 API Key 保存，保存失败保留草稿/可重试；普通路径不调用 keychain_save/delete。Startup 与 restart_bridge 删除自动钥匙串恢复，避免仅打开应用就触发密码弹窗。旧版凭据迁移收进可选折叠入口，仅用户主动触发时读原来源，随后写入本地凭据以供重启使用。遗留 native keychain commands 仍可显式使用，启动时不读取；旧系统钥匙串仅有的 Key 需要用户重新填写或主动迁移。本次未替换 Applications 或访问用户系统钥匙串。

Go bridge/internal/config 回归通过：无授权写入拒绝、idempotency、0600、本地保存后清空进程环境再读仍就绪、选择另一模型、旧内存覆盖移除、无效来源/空值/换行注入拒绝、清除后的未就绪与密钥不回传。组件测试覆盖正常添加、已有卡片保存、重试和模型选择器；完整前端生产 build、Rust keychain 24 个隔离单测和 clippy --all-targets -D warnings 通过。CUA 隔离真实组件 + mock host，5197/direct-key-qa.html：选择 MiMo、填写模拟 API Key、保存、模型就绪、选择 mimo-v2.6-flash 成功，console error/warn 空；截图 browser-direct-key.png。页面、server、tab 已清理。真实 MiMo 付费连接未调用。

本轮按用户要求调整 D 凭据默认方案；钥匙串保留显式迁移路径，其他 D/E/A/B/C 验收缺口保持，不能据此宣布迁移完成或正式发布。原候选包可回退；本轮继续 app-only，不生成 ZIP/DMG。

包内 smoke 夹具校正：local BaseURL 核心默认允许无密钥，不能用 configured=false 作为其断言；改用 MiMo 官方 BaseURL + 明确的本机 mock request_url，且 mock 按实际 MiMo api-key 请求头校验。初次夹具不匹配已清理，未调用真实 MiMo。另补显式 local APIKeyEnv 可保存边界，不以 RequiresAPIKey() 阻止用户提供本地代理密钥；新自定义连接的用户输入密钥不再因 local configured=true 被跳过，重试保持该行为。新增 Go 回归及连接组件回归通过。

最终干净源码 016f1fb1d 执行 app-only 构建，所有生产前端门禁/sidecar/Rust/signature strict 核验通过，只生成 arm64 app。本包实际 sidecar smoke 使用独立 HOME/REASONIX_HOME 和本机 mock：未授权 POST 拒绝、保存就绪、仅 .env 保存且 0600、MiMo api-key 认证头实际请求、退出重启后第二模型请求成功、清除后未就绪，两次 shutdown 202/exit0/readiness 删除，夹具清理。未启动用户原生 GUI 或请求真实 MiMo，未访问系统钥匙串。老包保留作回退，Applications 未替换，不正式发布。
