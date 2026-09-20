# Reasonix Tauri host

这是逐步替换 Wails shell 的 Tauri 2 host；它暂不替换稳定的 Go Agent、会话格式或
现有前端 API。启动时 host 从 `REASONIX_DESKTOP_BRIDGE_BIN` 读取由构建流程提供的
`reasonix-desktop-bridge` 可执行文件路径，并通过每次启动独有的 token、ready 文件和
launch nonce 监管它。

目前暴露给 WebView 的命令仅有 `bridge_status` 与 `restart_bridge`。token、loopback
端口和 bridge 原始请求不暴露给 JavaScript；下一阶段会由 Rust 以显式、受限的命令包装
bridge 协议，再让 `desktop/frontend/src/lib/bridge.ts` 接入该适配器。

开发环境还需要满足前端锁定的 Node 24 与 pnpm 10；当前机器的 Node 16 不能启动 Vite。
在准备好 bridge 二进制及正确 Node 环境后，从此目录运行 `cargo tauri dev`。
