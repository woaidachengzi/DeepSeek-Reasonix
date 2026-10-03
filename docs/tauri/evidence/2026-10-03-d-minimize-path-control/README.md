# 当前22ee包最小化路径对照

2026-10-03，生产代码未改，固定已安装 host `22ee53cbfe6324f63a1f63d73d477cb3017be21cd5866ee4dfab1f50efc20e35` / sidecar `6f7f6352af9eed4043abe9600b6ad80712088d9363b4a94d69d824628035e447`。来源/DMG及此前最小化失败见[全屏检查点](../2026-10-03-d-fullscreen-completion/README.md)，不把当前HEAD当作该包的干净构建来源。

## 实际对照与范围

同包、每例全新0700私有档案，各执行一次：两种档案 × Tauri API、主线程直接NSWindow::miniaturize、原生Minimize菜单、要求occlusion呈现的API，共 **8/8**。均使用已有settings阶段，在最小化后实际Settings菜单恢复并验证事件/几何；API与menu各自原断言保留，直接/原生菜单还有Will/Did/Deminiaturize事件要求。8例独立native结果与kernel exit0均保留；成功root仅在回执复制后清理，没有失败重试，没有剪贴板或用户配置访问。

每例前附只读NSWorkspace/WindowServer元数据观察器，日志不含窗口标题或像素；编译出的observer二进制没有归档，源代码已保存。控制前后严格签名、host/sidecar精确SHA保持；每例launch继续检查唯一sidecar、鉴权拒绝、凭据身份及退出ready清理。

动作前8例均mainPageFinished=true、实际active/key=true、restoreRequests/Completions=1；最小化时nativeMiniaturized=true、Will/Did=1，程序化Settings恢复通过。多个成功样本动作前nativeOcclusionVisible=false，因此“必须已经occlusion visible”不是这些样本的必要条件，不能把呈现等待自动称为修复。

## 与失败的区别与下一步

此前同包explicit exercise请求时mainPageFinished=false、active/key=true、nativeOcclusionVisible=false、restoreRequests/Completions=2，随后最小化超时、Will/Did=0。现在不同样本同时改变了页面状态、初始化/几何轨迹与Show次数；不能从8项成功认定加载、Show或API dispatcher为根因。当前源码调用链：Tauri Minimize消息→Tao set_minimized(true)→NSWindow::miniaturize，直接AppKit与原生菜单也成功，未支持固定某一路径失效。历史未完成页面时成功的样本与Cocoa失败也仍保留，不改写成单一归因。

下一项以原exercise为基线，分别控制是否先等可信主页面Finished和是否发出额外首次Show；每种组合只执行一次，用同一精确几何/事件要求，不增加sleep、动作重试或容差。在获得差异前不修改产品窗口流程，不调整完整矩阵来提高通过率。

## 可重跑工具

实际执行的是固定22ee身份的 control.py，原始日志/result在controls。新增 tools/tauri/probe-minimize-paths.py 从该控制提取app/output参数，继续8例一次性对照、失败原始回执/私有root保留，并在任一case失败时返回非零。新通用版本本轮只运行--help/语法检查，不能称其已独立跑过真实8例；原控制版本是真实执行证据。两者代码均归档，新增工具未重建或改变产品包。

最小化历史失败保持未关闭，此对照不是完整窗口、物理UI或稳定性验收。全屏之前的双档案切片不变，当前包其余D/官方备份回退及现场授权等继续待验；D未完成/E未开启，A/B/C和正式签名公证仍阻碍发布。仅Windows/Linux已有延期，没有新增延期、push、发布或默认下载切换。
