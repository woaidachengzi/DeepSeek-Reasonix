# 零活动显示器时延迟恢复窗口状态

真实 cbb590ba 在私有 managed 档案、无窗口操纵的 menu-shortcuts 正常退出切片，将固定窗口种子 x=-3400 改写为0；高度/宽度/比例/y保持，来源仅自有临时 window-state.json，无 sidecar/ready 残留。CoreGraphics status0/activeCount0 与 NSScreen 缓存2的只读对照另存。

源码修复：有效保存记录的首次恢复保持 pending，只有成功枚举非空活动显示器且原生尺寸/位置/最大化恢复成功后清除；不拿缓存主屏解释空枚举为副屏断开。pending 或显示器枚举失败/为空时禁止捕获临时默认边界。真正窗口 Focused(true)、tray/Dock/single-instance 公共显示路径可重新尝试 pending；恢复成功后后续焦点不会重新覆盖用户窗口。

现有几何/持久化回归8项及 strict clippy通过。实际新 app/DMG 构建、只读挂载复制安装和 strict ad-hoc 签名通过；host 7ac271422cee864ac7a296a1b0e9504370148efc359c2904e857bd36b205acb5、sidecar cd7262010182d96253c0aa6e6a8038b18ce9f194e35cbfac5cc75106a1ba46b3、DMG 5ac3d73826ec4d97cb377486c99aae843aac98a210f0d8c488aa4fddeab797ba。同一零活动显示器反例 managed/explicit 两档案均通过：x=-3400等种子值原样保持、正常退出及无侧进程/ready残留。新增条件门禁 inactive-display-state 显式选择才执行，要求真实 CG status0/count0，前后检查；未通过模拟或修改OS显示器来制造条件。[新包七项门禁](../2026-10-03-d-deferred-window-package-run/README.md)全部通过。尚未证明活动显示器恢复后 pending 正常完成，也不是最小化修复；此项不放行 D/E。
