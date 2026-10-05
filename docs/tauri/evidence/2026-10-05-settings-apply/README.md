# API Key 保存后的会话应用修复

Review 覆盖 7 个原未提交文件，发现普通 reopen 丢失会话授权、先关闭旧会话使失败回退不可靠，以及 settings fingerprint 读取失败仍发送旧凭据。改为 RuntimeSettingsFactory 接入现有 boot.Rebuild；成功迁移后发布替代运行时，并通过 ReleaseResources 释放旧运行时，不做旧会话 shutdown snapshot 或触发 SessionEnd。设置检查、构建和提交由同一 ownership mutex 保护。读取/构建失败返回安全的 settings_apply_failed 409，保留旧运行时；运行中/暂停的旧请求不会被配置更新强制重建。

与 Wails desktop/model_settings.go、desktop/settings_app.go 及 internal/boot/reload.go 基线核对：复用历史、审批模式、session grants、Plan 只读信任、Goal/recovery、lifecycle 和会话私有临时目录迁移。仅复用已有引擎，不引入新的配置/凭据体系。API Key 仍直接写 Preview 独立 .env，不要求钥匙串授权。

验证：go test -race ./internal/desktopbridge ./cmd/reasonix-desktop-bridge ./internal/boot -count=1 通过；最终测试变更的目标 race 回归见 race-regression.log。真实 Controller 回归通过保存路径验证请求实际使用 first-key/second-key，模拟 provider 构建失败后保留原 controller，恢复原 key 后仍能继续对话，后续热重建保留历史、授权、审批模式和临时目录。其他测试覆盖 settings read 错误的拒绝与 HTTP 脱敏、缺失/非法候选释放、重试、取消/活跃状态、并发退出和删除保护。

真实 MiMo 网络和用户原生 GUI 尚未验证；未访问用户凭据、钥匙串或现有 Applications，不代表 D/E/A/B/C 全部验收通过。后续按用户既有要求构建 app-only，包内 smoke 证据另补；禁止正式发布或切换默认下载项。
