# Tauri 图片链路阶段验收

日期：2026-10-08。基线：`d564c5c3e`，本页对应其后的图片阶段改造。
这是目标的第一阶段记录，不是 A/B/C 或三平台正式发布完成标记。

## 当前实现

- 输入框显示已复制进工作区的图片缩略图；新对话的已粘贴图片在发送前可预览、移除，不提前创建会话。
- 用户历史消息和发送中的待落盘消息显示图片附件；回复中的本地 Markdown 图片通过独立 Tauri adapter 解析，Wails binding 保留。
- Go 持有会话归属锁，使用真实运行时工作区和 `os.Root` 读取。绝对路径、`file://localhost` 仅在当前工作区内有效；不授权任意文件、外部文件夹或别的配置目录。
- 原文件最多 16 MiB，解码最多 4000 万像素；预览最长边 1200，重新编码为 PNG，最多 8 MiB。仅 PNG/JPEG/GIF/WebP；SVG、格式伪装、越界和外部符号链接不返回图片字节。GIF 预览为首帧，模型输入附件保持原有传输逻辑。
- 本地预览只在图片接近视口后解析；切换会话或图片来源时隐藏旧结果，过期异步回包不得显示到新会话。没有新增 transcript 原生滚动写入。
- 图片接口使用认证 bridge 和 typed schema，主窗口命令检查保留；没有放宽 CSP、通用文件权限或 clipboard plugin 的图片命令权限。

## 已验证与边界

| 项目 | 结果 | 范围 |
| --- | --- | --- |
| Go 图片解析、HTTP 认证与归属 | 通过 | 正常/编码路径、内联图片、MIME 伪装、字节/像素预算、遍历、外部符号链接、跨会话、关闭运行时、未知字段拒绝 |
| Go bridge / desktopbridge / protocolgen 全量测试 | 通过 | 独立临时 `REASONIX_HOME`，未读取用户配置 |
| Tauri 前端全量回归、生产构建 | 通过 | JSX、hook、单滚动 writer、样式与包预算；最终新增 IPC 契约另行复跑 173 passed |
| transcript 全量回归 | 通过 | 现有 kernel、几何提交、阅读保持、历史与分页契约 |
| Rust 全量回归 | 241 passed / 0 failed / 5 ignored | 默认环境，不能替代需要真实 sidecar 条件的测试 |
| Chrome 实际界面 | 通过 | `127.0.0.1:5198/image-qa`，1440×1000 与 390×844，真实组件/CSS；仅 native adapter 和模型服务为桩 |
| 图片交互 | 通过 | 历史附件、本地 Markdown、FileReader 粘贴、原生接口桩草稿预览、移除、重新打开；草稿粘贴不创建会话 |
| macOS arm64 应用及 DMG | 构建通过 | 本地 ad-hoc 签名，未公证，不覆盖已安装应用，不正式发布 |
| macOS 隔离包启动/退出 | 通过 | managed/explicit 两个新配置，独立凭证身份、Global 工作区、sidecar readiness 和退出清理；窗口在左屏 |

Browser plugin not available，界面验证使用仓库已安装的 Playwright 与 Chrome。
测试脚本、截图和运行日志保存在仓库外的临时目录，不把 QA 文件或用户图片加入仓库。

当前交付：`desktop/tauri/target/release/bundle/dmg/Reasonix-Tauri-Preview_0.1.0_macos-arm64_images-20261008.dmg`。
SHA-256：`8c9cbe2254334b66f356f595fe8dfae3efc8490c5d5a5b71279095bb7ab20fc1`。
安装包来自上述未提交增量，并非单独的 `d564c5c3e` 历史构建；仅供本机 Preview 验收。

## 本轮 review 与独立 app 交付

- 上一轮 app 已复制到 `/Users/jerry/Downloads/Reasonix-Tauri-Preview-images-20261008.app`，可双击运行；不覆盖已安装应用。主程序 SHA-256 为 `e7fa03101e8650ba66e21a688867277e1d9a9c7b99cd7c3ad6f49203320d3a22`，与原构建完全一致，`codesign --verify --deep --strict` 通过。
- 对下载目录的副本重新执行 managed / explicit 隔离启动验收，真实 sidecar、独立配置归属、Global 工作区与退出清理均通过，窗口固定在左屏。
- 本轮完整 review 后复跑 Go 三个包、Rust（241 passed / 5 ignored）、Tauri 前端回归、生产构建与 Chrome 图片交互验收；不是仅复用上一轮测试结果。
- 修复图片剪贴板 fixture 的准备操作抛异常/超时时绕过恢复的问题；Python 回归 8 项通过。Swift PNG/TIFF 的多格式恢复与新一代剪贴板拒绝覆盖测试仅操作私有 named pasteboard，通过；没有操作系统剪贴板。
- 新增原生图片 Paste / 历史重开验收模块已通过 Rust 编译，但尚未连接独立模型 fixture 并执行包级流程。它只是后续验收准备，不构成真实原生粘贴通过证据，也不在上面交付的上一轮 app 中。
- 交付 app 的生产功能代码与本轮图片改造一致；本轮额外变化是验收模块和恢复工具，不将旧包描述为本次提交的重新构建。

## 第一阶段仍需完成

- [ ] 当前安装包真实 WKWebView 的截图/图片 Cmd+V；必须保留并恢复系统剪贴板所有格式，验证退出/取消时私有临时图片清理。
- [ ] 真实包中关闭、重开后的图片历史和旧 Global 附件兼容；只使用隔离测试数据，不自动迁移用户资料。
- [ ] 受限远程图片代理（地址、重定向、字节/像素和超时门禁）；当前 HTTP(S) 图片保持原有 WebView 直接加载，不视为 Wails 代理已迁移。
- [ ] 更多格式与图片查看器策略；当前不支持非 base64 的内联 data 图片或 SVG。
- [ ] Windows/Linux 原生图片粘贴与当前版本包级复测。

图片阶段收敛后依次推进内置终端、应用内远程会话/controller、bot Desktop 联动，再收敛跨平台、复杂管理页和存储恢复门禁。Go 引擎保留，updater 排除。真实外部账号联调、用户资料迁移和正式发布须另获授权。
