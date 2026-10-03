# 按真实状态推进的窗口最小化对照

新增私有 example `phase-timer`，main run loop每50ms检查页面完成（Cocoa额外要求WK isLoading=false/progress=1），按loaded→key→mini→demini推进。加载后按选项orderOut/show/activate；调用前原生active/key/visible与页面前提全部成立。只有native isMiniaturized=true且收到will-mini/did-mini才请求恢复；只有native false且收到did-demini才通过。原始固定步数模式条件未放宽。计数、阶段瞬时状态、通知、真实内核退出均保留。50ms定时器在成功/采样截止/退出时注销，observer照常清理。

阶段版9365791b：Cocoa+WK/loaded reorder连续两次真实最小化/恢复通过（PID22471、22614，kernel0/open0）；Tauri overlay不reorder及reorder初次均前提成立但无最小化（PID22509、22555，kernel2/open0）。失败停在mini，未虚构恢复调用。

随后仅增加原生窗口属性采样和可选移除NSWindow delegate。首次json宏递归编译失败保留，拆成独立属性对象后最终默认feature locked offline release build和Clippy通过。最后源码snapshot对应520c853b，前936版本没有后加属性/代理诊断；不把最后源码假称为此前版本完整snapshot。未改Cargo清单、生产窗口代码或权限。

520c853b同二进制overlay/loaded reorder：移除窗口代理一次通过（PID22713）；保留窗口代理连续两次也通过（PID22757、22849），均真实will-mini/did-mini/did-demini与kernel0/open0。又从保存的旧app复制并核对9365791b摘要，原失败的overlay/loaded reorder重跑也通过（PID22819）。

因此不能把改善归因于移除代理、属性getter或某个新二进制。状态推进为Tauri事件循环内真实最小化提供正样本，证明并非绝对不支持，但同二进制前后差异仍显示环境/时序不稳定。没有为了通过生产门禁而移除delegate或加入重试。

所有包均私有HOME/TMP、固定本地页，无sidecar或产品配置；strict ad-hoc及有效entitlements等于生产包。runner0只证明准确采集kernel回执，只有result原生状态和通知成立才算功能通过。

当前249dfd15真实安装候选随后独立复验完整既有窗口门禁，见[当前候选回执](../2026-10-03-d-current-window-after-phase/result.json)。诊断正样本不能直接替代生产候选或实际菜单/快捷键、多显示器等验收，D/E整体未完成。
