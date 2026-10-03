# Wails nil-sender 对照

冻结 Wails desktop/go.mod 使用 Wails v2.13.0；对应本地模块 WailsContext.m 的 Minimise/UnMinimise 使用 miniaturize:nil/deminiaturize:nil。保存只读源摘录。Tauri 原 direct 对照以窗口作 sender，新私有 example nil 选项只在 direct 分支验证，不属于生产 host。

首次编译类型不匹配保留；修复后离线 release/Clippy通过。二进制d3a241a21a470a43ed14e753fcba4b2807012983bac8d974c21deb1af13dd05f，PID17729。24次采样均 nilSender=true，激活/key/页面完成前提成立，loaded重新显示执行，mini/demini各调用一次，nativeMiniaturized仍全部false、kernel2/open0。nil sender不足以修复这一有效失败；runner0仅表示采集失败回执成功。生产未改。
