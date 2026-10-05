# API Key 保存后的会话应用修复

Review 覆盖 7 个原未提交文件，发现普通 reopen 丢失会话授权、先关闭旧会话使失败回退不可靠，以及 settings fingerprint 读取失败仍发送旧凭据。改为 RuntimeSettingsFactory 接入现有 boot.Rebuild；成功迁移后发布替代运行时，并通过 ReleaseResources 释放旧运行时，不做旧会话 shutdown snapshot 或触发 SessionEnd。设置检查、构建和提交由同一 ownership mutex 保护。读取/构建失败返回安全的 settings_apply_failed 409，保留旧运行时；运行中/暂停的旧请求不会被配置更新强制重建。

与 Wails desktop/model_settings.go、desktop/settings_app.go 及 internal/boot/reload.go 基线核对：复用历史、审批模式、session grants、Plan 只读信任、Goal/recovery、lifecycle 和会话私有临时目录迁移。仅复用已有引擎，不引入新的配置/凭据体系。API Key 仍直接写 Preview 独立 .env，不要求钥匙串授权。

验证：go test -race ./internal/desktopbridge ./cmd/reasonix-desktop-bridge ./internal/boot -count=1 通过；最终测试变更的目标 race 回归见 race-regression.log。真实 Controller 回归通过保存路径验证请求实际使用 first-key/second-key，模拟 provider 构建失败后保留原 controller，恢复原 key 后仍能继续对话，后续热重建保留历史、授权、审批模式和临时目录。其他测试覆盖 settings read 错误的拒绝与 HTTP 脱敏、缺失/非法候选释放、重试、取消/活跃状态、并发退出和删除保护。

真实 MiMo 网络和用户原生 GUI 尚未验证；未访问用户凭据、钥匙串或现有 Applications，不代表 D/E/A/B/C 全部验收通过。本包仅构建可直接运行的 app；禁止正式发布或切换默认下载项。

干净源码 6fef0d95d 构建 app-only 成功，生产前端门禁、Go sidecar、Rust release 和 strict ad-hoc signature 检查通过，见 package-build.log、package-receipt.json。上一份 .app 已在临时目录保留为 .app.rollback（避免重复应用索引），路径见 receipt；可停止新包后将此目录复制回原 .app 路径回退。

包内真实 sidecar smoke 通过，见 package-smoke.json 和可重复夹具 package-smoke.py。一次性 HOME/REASONIX_HOME + 本机 HTTP mock 完成 5 个 provider 请求：同一 chat 会话首次发送、保存轮换 key 后第二次发送、错误配置恢复后第三次发送、选择 flash 后发送、进程重启后的 flash probe。保留旧 user history 和 auto 审批模式；不合法 TOML 使 submit 返回 409 并保持 history 可读，未发出上游请求。未授权保存拒绝、本地 .env 是唯一含模拟新 key 的文件且权限 0600、清除后未就绪、两次 shutdown exit0/readiness 清理通过。夹具档案/进程已清理，未调用真实 MiMo、启动用户 GUI 或访问系统钥匙串。
