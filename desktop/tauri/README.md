# Reasonix Tauri host

这是逐步替换 Wails shell 的 Tauri 2 host；它暂不替换稳定的 Go Agent、会话格式或
现有前端 API。开发启动时 host 从 `REASONIX_DESKTOP_BRIDGE_BIN` 读取由构建流程提供的
`reasonix-desktop-bridge` 可执行文件路径；打包启动时则由 Tauri 的 `externalBin` 从应用
包内定位同一个 bridge。两种路径都通过每次启动独有的 token、ready 文件和 launch nonce
监管它。

目前暴露给 WebView 的命令为 `bridge_status`、`restart_bridge`、`bridge_open_session`、
`bridge_session_snapshot`、`bridge_submit` 和 `bridge_cancel`。token、loopback 端口和
bridge 原始请求不暴露给 JavaScript。`bridge_start_events` 在 Rust 内部订阅 SSE，并以
Tauri `bridge:event` 和故障时的 `bridge:connection-error` 事件转发给 WebView。

`desktop/frontend/src/lib/tauriBridge.ts` 已为上述小范围命令提供类型化适配器，只会在
Tauri WebView 中激活；现有 `desktop/frontend/src/lib/bridge.ts` 仍是 Wails 默认实现，直到
对应功能面完成迁移。Tauri WebView 现会渲染 `TauriSessionPreview`：可开/读会话、发送、
取消，并显示 bridge 事件和连接故障；它是有意隔离的最小垂直切片，不会冒充完整 Wails UI。
事件订阅可从 snapshot 的 sequence 开始，避免切换期间漏掉事件。sidecar 异常退出时，预览
提供受控重启：重新启动 bridge、重新打开当前 session、获取新 snapshot 后才恢复事件订阅和发送。

开发环境还需要满足前端锁定的 Node 24 与 pnpm 10。使用前端目录中的本地 Tauri CLI
启动，脚本会把 Go sidecar 构建到被忽略的 `desktop/tauri/target/sidecar-dev/`，并仅向
Tauri host 注入其路径：

```bash
cd desktop/frontend
pnpm tauri:dev
```

这不依赖全局 `cargo tauri`，也不会将 sidecar 路径、token 或 loopback 地址暴露给 WebView。

## 构建 macOS 测试包

```bash
cd desktop/frontend
pnpm tauri:build
```

该命令先使用当前 Rust host target triple 构建 Go bridge，再由 Tauri 将它作为受管 sidecar
嵌入应用包。当前仅支持与构建机器相同的 target，不接受跨 target 打包；这是为了避免在
尚未建立 macOS 双架构签名与回归流程前制造未经验证的安装包。

本机构建没有 `APPLE_SIGNING_IDENTITY` 时会使用 ad-hoc 签名，适合本机测试但未公证；不要
将此类包作为正式下载发布。配置 Developer ID 与公证凭据后，正式发布流程必须保留 Tauri
的签名并完成 Apple notarization。

## 测试

依赖 Go bridge 的受监督生命周期测试在本地默认跳过。要让它们真正运行，先构建 bridge
并把路径传进去：

```bash
bridge_target="$(rustc --print host-tuple)"
mkdir -p desktop/tauri/binaries
go build -trimpath -o bin/reasonix-desktop-bridge ./cmd/reasonix-desktop-bridge
cp bin/reasonix-desktop-bridge "desktop/tauri/binaries/reasonix-desktop-bridge-${bridge_target}"
cd desktop/tauri && REASONIX_TAURI_BRIDGE_TEST_BIN="$PWD/../../bin/reasonix-desktop-bridge" cargo test
```

`CI` 环境变量存在时该路径是必需的：缺失会让这两个测试失败，而不是静默跳过。
