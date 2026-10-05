# 审批命令长文本边界

用户截图中的 bash 审批命令包含超长 URL/百分号编码参数，Tauri subject 只有 `white-space: pre-wrap`，连续字符串仍向卡片外绘制。Wails `approval-subject` 已有折行处理，本轮补齐 Tauri 公共提示卡片的 `overflow-wrap: anywhere`、最小/最大宽度约束，并允许标题内容收缩，保留图标尺寸。保留完整命令及原始空格、换行；没有截断、插入字符或隐藏溢出，也未修改审批协议或会话滚动逻辑。

验证入口：`http://127.0.0.1:5198/approval-qa` → 实际 `PromptCard` 组件呈现与截图同类 curl 长 URL → 复制完整文本、分别允许/拒绝。Browser plugin not available，使用已有 Playwright 1.62.1 与已安装的 Google Chrome（headless），不安装新依赖、不操作用户浏览器。临时 Vite 插件导出原组件，仅宿主审批回调与周边会话内容模拟；没有将夹具入口写入生产源码。

1280×900、768×900、390×900 三档均验证：页面身份/非空/无错误覆盖层/控制台 error 与 warning 空；每个文本行的边界留在卡片内，subject 无水平滚动溢出，按钮完整可见；允许与拒绝分别更新夹具状态。复制文本逐字等于原命令。补充长工具名称、长路径说明和暗色主题，均保持在卡片内部。

基线溢出约 964/1064/1418px，修复后均为 0；详细测量见 `browser-before.json`、`browser-after.json`，截图 before-desktop/after-desktop/after-narrow。复现脚本 `browser-qa.mjs`，默认 frontend 路径为当前工作区；执行 `node browser-qa.mjs after`。测试页面、无界面浏览器与临时 server 均关闭。

这些是实际 React/CSS 的浏览器显示验证，审批回调为模拟，不能替代原生 WebKit 或真实工具执行验收。D/E/A/B/C 缺口保持；本轮继续只构建直接运行的 `.app`，保留前一版回退，不生成 ZIP/DMG，不发布或切换默认下载。
