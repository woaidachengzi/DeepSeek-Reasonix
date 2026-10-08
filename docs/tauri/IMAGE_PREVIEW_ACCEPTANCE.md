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
- HTTP(S) / 协议相对图片也使用 bridge，不再让 WebView 直接下载。Tauri 与 Wails 共用应用代理 transport：每跳校验全部 DNS 地址、连接固定到通过检查的 IP，不向图片服务带入模型凭据、Cookie 或 Referer；代理失败不直连兜底。
- 远程图片最多 10 MiB、4000 万像素，重新编码为受限 PNG；最多 5 次请求（含首跳）、20 秒网络预算、2 个并发槽。网络期间不持有 controller 锁；回包必须匹配会话归属 epoch，切走再切回也拒绝旧回包。
- 图片专用地址策略额外拒绝保留/文档/协议空间与 NAT64、6to4 等转换前缀；此保守策略不用于普通模型请求。地址分类依据 [IANA IPv4](https://www.iana.org/assignments/iana-ipv4-special-registry) 与 [IANA IPv6](https://www.iana.org/assignments/iana-ipv6-special-registry)，不声称可以识别任意自定义网络转换规则。

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

上一轮交付（不包含后续清理/远程代理增量）：`desktop/tauri/target/release/bundle/dmg/Reasonix-Tauri-Preview_0.1.0_macos-arm64_images-20261008.dmg`。
SHA-256：`8c9cbe2254334b66f356f595fe8dfae3efc8490c5d5a5b71279095bb7ab20fc1`。
安装包来自随后提交的图片阶段增量，并非单独的 `d564c5c3e` 历史构建；仅供本机 Preview 验收。

## Review 与独立 app 交付（`a2e3c851a`）

- 上一轮 app 已复制到 `/Users/jerry/Downloads/Reasonix-Tauri-Preview-images-20261008.app`，可双击运行；不覆盖已安装应用。主程序 SHA-256 为 `e7fa03101e8650ba66e21a688867277e1d9a9c7b99cd7c3ad6f49203320d3a22`，与原构建完全一致，`codesign --verify --deep --strict` 通过。
- 对下载目录的副本重新执行 managed / explicit 隔离启动验收，真实 sidecar、独立配置归属、Global 工作区与退出清理均通过，窗口固定在左屏。
- 本轮完整 review 后复跑 Go 三个包、Rust（241 passed / 5 ignored）、Tauri 前端回归、生产构建与 Chrome 图片交互验收；不是仅复用上一轮测试结果。
- 修复图片剪贴板 fixture 的准备操作抛异常/超时时绕过恢复的问题；Python 回归 8 项通过。Swift PNG/TIFF 的多格式恢复与新一代剪贴板拒绝覆盖测试仅操作私有 named pasteboard，通过；没有操作系统剪贴板。
- 新增原生图片 Paste / 历史重开验收模块已通过 Rust 编译，但尚未连接独立模型 fixture 并执行包级流程。它只是后续验收准备，不构成真实原生粘贴通过证据，也不在上面交付的上一轮 app 中。
- 交付 app 的生产功能代码与本轮图片改造一致；本轮额外变化是验收模块和恢复工具，不将旧包描述为本次提交的重新构建。

## 后续原生验收准备（未提交增量）

- 增加 `tools/tauri/smoke-native-images.py`：只在新建私有 managed/explicit 配置中，使用自有 PNG/TIFF fixture、已安装 AppKit Paste role、真实 Go controller 与 loopback vision 模型服务；验证发送字节与 Global 附件一致、图片历史重开、没有重复模型请求，恢复原剪贴板的全部格式。不操作正在运行的用户 Preview。
- 模型服务验证真实 image_url 输入、PNG chunk CRC、64×40 像素流预算及自有问题 nonce，不输出请求内容、图片或剪贴板数据；独立 Python 请求门禁回归 4 项通过（含真实 loopback HTTP/SSE 完成门禁和错误内容不回显），剪贴板恢复回归 8 项通过。
- 退出路径最后调用 `process::exit`，不能依赖 managed state 的 Drop。新增显式 `PastedImages.shutdown()`，在 `RunEvent::Exit` 删除所有自己持有的临时文件并关闭 staging，拒绝迟到的 worker。保留 worker 引用的清理/拒绝/idempotency 回归通过，Rust 当前 242 passed / 0 failed / 5 ignored。
- 原生模块在退出前留下一个真实未发送临时图片，外部 runner 必须确认文件已经删除；重开必须确认未发送草稿没有重放。以上为验收断言，尚无包级通过证据。
- 首次原生 PNG managed 验收因检测到正在运行的 Preview 在启动前被安全拒绝，未修改系统剪贴板。需要用户退出应用后继续，不能把这次拒绝当作通过。
- 曾构建仅含显式退出清理的中间 App，主程序 SHA-256：`354af41b958e711d7dd0d58bfc65db9792e17bc2903f21a24a7e36c1f304b70e`。它已被构建目录的后续代理版本替换，没有进行原生运行验收；下载目录的上一轮 app 和 DMG 未覆盖。
- AppKit Paste role 不等于物理键盘 Cmd+V。键盘、输入法和跨平台粘贴门禁仍保留。

## 受限远程图片代理增量（未提交）

- Go netclient / desktopbridge / bridge / protocolgen 全量测试通过；公共地址与会话归属的定向 race 回归通过。Wails `RemoteMarkdownImage|ResolveMarkdownImage` 回归通过，覆盖原有代理、重定向及 DNS 重绑定检查。
- 认证 HTTP bridge → 读取独立配置的应用代理 → CONNECT 已校验的公网数字 IP → 受限图片解码的真实 Go 链路通过。目标仅为自有 loopback proxy fixture，没有请求外部网站，也没有使用用户 API Key。
- 额外直接启动当前 App **包内 sidecar**（不是 handler/controller 桩）验收通过：真实 Global 会话归属、未授权/未拥有会话拒绝、配置代理 CONNECT、2×2 PNG 像素返回、内网图片在代理调用前拒绝，以及正常退出/ready 文件清理。只启动自有子进程，不启动 GUI、不操作剪贴板；仍不等于原生界面验收。
- Review 修正重定向时 Go 自动添加的 Referer，避免把前一 URL 的查询参数发给下一站；初始请求与每跳 transport 均清理私有请求头。远程 SVG/HTML、过大正文、私有重定向及取消/迟到回包回归通过。
- Rust 全量 243 passed / 0 failed / 5 ignored，Tauri 前端全量及生产构建通过。Chrome 1440×1000 / 390×844 真实组件/CSS 的远程图片、粘贴、移除和重开通过：远程来源调用 typed bridge，DOM 只显示返回的 data PNG，未触发外部图片网络请求。该界面验收的 native adapter 与模型服务为桩，不能替代真实 WKWebView。
- 中间代理 App 已构建并通过 `codesign --verify --deep --strict`。主程序 SHA-256：`1aac5c20d193ce749c7905150a34943fe5bcc47f8d4db457e683d8d05a663591`；包内 bridge：`c7e0c89cb896ca17e82f0783323a664e80b434a17fdeefd158f59d5bbffca9d3`。该中间版本已被下述附件根目录修复版本替换；Downloads app/旧 DMG 未覆盖。
- 当前工作区另有并行的文件引用解析修改，测试与 App 也包含构建时的这些源码。此处不将它们记为本图片增量已 review/提交，也不把未提交包说成只对应 `a2e3c851a`。
- transcript 全量回归再次通过，未新增滚动 writer；上述截图与 adapter 测试应用 React/界面测试规范，保留会话切换的过期回包门禁。

## 原生粘贴发现的模型输入缺失（未提交修复）

- Preview 空闲时实际执行 managed / PNG 原生流程：WKWebView 的草稿预览、移除和第二次粘贴/提交完成，随后在真实 loopback 模型完成门禁失败。自有失败进程已退出、原剪贴板全部格式已恢复；这是失败证据，不是完整原生通过。
- 原因：Global controller 持有显式工作区，但模型附件读取仍走旧 CLI 的进程 cwd 路径。附件已复制到 Global，预览可用，模型请求却没有图片。独立启动包内 sidecar 的实际 attach → submit 请求也复现了相同问题。
- 改为显式工作区附件读取：`os.OpenRoot` 限定根目录，逐级拒绝符号链接，校验常规文件/64 MiB 上限及打开、读取后的文件身份/长度。不修改 cwd、不创建缺失目录、不回退到同名 cwd 文件；仅无显式根目录的旧 CLI 保留原路径。
- 新回归覆盖同名 cwd 诱饵、Global 附件送入真实模型请求、工作区缺失、遍历、内部/外部链接以及读取不创建目录。最终读取校验的定向 race 回归通过；最后再次复跑 Go control / boot / bridge 全量，全部通过。
- 修复版 App 位于 `desktop/tauri/target/release/bundle/macos.noindex/Reasonix Tauri Preview.app`，重新构建并通过严格签名验证。主程序 SHA-256：`70cb3d020b7332201533d9cebff0da907b46693cd42aa60b637cf7ce11c7ec5a`；包内 bridge：`7e5dd6d9056fe09fbcb6ff376b7896f8c5aac36d9cf01ea22d72e88b504f9603`。只有本地 ad-hoc 签名；不覆盖历史 Downloads app/DMG，不正式发布。
- 对该 **实际包内 sidecar** 的 `TestWorkspaceImageActualPackageProxySmoke` 通过：认证、会话归属、真实配置代理、公网图片重编码与内网拒绝，再执行实际 attach → submit → SSE 模型回答 → 历史保存。模型收到的唯一图片与自有文件字节一致；恰好一次模型请求、一次代理请求，正常退出清理 ready 文件。模型和代理均为自有 loopback 服务，不访问真实账号/外部模型。
- 最新四组原生矩阵启动前再次检测到用户 Preview 正在运行，安全拒绝，未修改剪贴板。修复后原生完整发送/退出清理/历史重开、物理 Cmd+V 仍未验收；不能用包内 sidecar 测试替代这些门禁。
- 日志位于仓库外 `/private/tmp/reasonix-images.CUK5nR/`：`image-actual-package-complete.log`、`image-root-go-full.log`、`image-root-race.log`、`image-root-package-final.log`、`native-image-final.log`。同期另有并行 UI/文件引用修改；构建只能绑定上述二进制哈希，不能声称与之后变化的整个工作区或某个干净提交完全一致。

主要复现命令（配置与 cache 必须指向自有临时目录）：

```sh
go test ./internal/netclient ./internal/desktopbridge ./cmd/reasonix-desktop-bridge ./internal/desktopbridge/protocolgen -count=1
REASONIX_IMAGE_PACKAGE_BIN='<明确选择的 app>/Contents/MacOS/reasonix-desktop-bridge' go test ./cmd/reasonix-desktop-bridge -run '^TestWorkspaceImageActualPackageProxySmoke$' -count=1 -v
# desktop/frontend 中
pnpm test:tauri
pnpm test:transcript
pnpm tauri:build -- --bundles app
# 仅在所有同 bundle ID Preview 已退出时，使用左屏 geometry 文件继续：
python3 tools/tauri/smoke-native-images.py '<明确选择的 app>' --window-state-template '<自有左屏 geometry JSON>'
```

## 第一阶段仍需完成

- [ ] 当前安装包真实 WKWebView 的截图/图片 Cmd+V；必须保留并恢复系统剪贴板所有格式，验证退出/取消时私有临时图片清理。
- [ ] 真实包中关闭、重开后的图片历史和旧 Global 附件兼容；只使用隔离测试数据，不自动迁移用户资料。
- [ ] 受限远程图片代理的原生界面验收；源码、隔离 Go/浏览器及实际包内 sidecar 链路已通过，尚未放行原生实际网络使用。
- [ ] 更多格式与图片查看器策略；当前不支持非 base64 的内联 data 图片或 SVG。
- [ ] Windows/Linux 原生图片粘贴与当前版本包级复测。

图片阶段收敛后依次推进内置终端、应用内远程会话/controller、bot Desktop 联动，再收敛跨平台、复杂管理页和存储恢复门禁。Go 引擎保留，updater 排除。真实外部账号联调、用户资料迁移和正式发布须另获授权。
