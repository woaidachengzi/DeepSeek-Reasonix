# 当前60076bd5完整窗口门禁失败

未更改完整window门禁及Rust断言，当前真实安装包运行两档案矩阵。managed前8项通过：exercise、restore-maximized、restore-normal、appearance-rollback-unconfigured、application-hide、background-close、menu-shortcuts、menu-settings-hidden。第9项menu-settings-minimized失败；managed后续阶段及explicit全部未执行，不能视为通过。

实际minimize请求前page finished、active/key/visible/occlusion前提成立，样式miniaturizable。请求后原生will-mini/did-mini/did-demini均0，10秒等待后nativeMinimized=false，key/occlusion变false，visible仍true。root /private/tmp/reasonix-native-window-smoke-fcflxv4l保存原始失败夹具，failure-native复制其窗口结果/trace。runner1保留，签名/artifact hash前后相同。

该结果再次说明窗口重复启动稳定性未通过；源码钥匙串改善不能解决或替代窗口验收。不能仅归因于未激活、页面未就绪或已完成restore的缺失；底层原因仍待对照。没有改生产最小化行为、增加重试、移除代理或放宽门禁。D未完成，E未开启。
