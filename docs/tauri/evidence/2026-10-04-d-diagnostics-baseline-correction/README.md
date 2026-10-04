# D：更正诊断导出基线归属

上一轮 `2026-10-04-d-current-save-recovery` 错将当前 FrontendDiagnosticsControl 导出与 Wails ExportScrollDiagnostics ZIP 入口直接对照，从而错误列出了该入口JSON/ZIP格式差异。本次按 Wails 1.38.3 对应控件、记录器和导出API重新核对；保留原回执/源码证据，并明确更正该判断，没有修改产品来迎合错误对照。

## 对应关系

| 记录入口 | Wails 1.38.3 | 当前 Tauri | 本轮结论 |
| --- | --- | --- | --- |
| FrontendDiagnosticsControl 前端交互记录 | frontendDiagnostics.stop → PickExportFile(application/json) → SaveExportFile(JSON.stringify(payload))，成功才reset | 同一记录器 → export_frontend_diagnostics 原生保存JSON，返回true才reset | 对应入口均为JSON，当前不存在所述JSON/ZIP格式缺口 |
| ScrollDiagnosticPanel 滚动诊断 | transcriptScrollDiagnostics → ExportScrollDiagnostics，ZIP含manifest.json/summary.json/scroll-events.jsonl/sha256.txt | 独立旧入口/API的迁移与覆盖尚未证明 | 分开追踪，不用改前端交互JSON格式来替代它 |

两个记录器都使用schemaVersion2，但事件与manifest字段不同。当前 `lib/frontendDiagnostics.ts` 与固定Wails基线**字节完全一致**，SHA256 `7db73f45b01ce045a5b51929420cf4917e92eba0abf65056b36b914770eb4fc8`。对应FrontendDiagnosticsControl基线默认文件名也是 `reasonix-frontend-diagnostics-<reportId前8位>.json`，取消后不reset，可重试；当前控件保留Wails分支，Tauri仅使用受限host保存分支。源码与SHA逐项冻结。

## 验证和限制

本轮基于 `820938679`，产品与权限未改，未新建安装候选，也未打开GUI或扰动屏幕。现有 `frontend-diagnostics.test.ts`、`transcript-scroll-diagnostics.test.ts`各exit0；覆盖各自记录器的脱敏固定字段、容量/丢弃计数、停止后冻结及schema行为等，不能把这些源码测试算作未执行的ZIP实际包验收。

上一轮当前46包的真实保存、覆盖取消保持hash/mtime/mode、0600替换、取消保留5事件后重试及正常Quit清理回执继续有效，且现在有正确的JSON入口基线归属。默认目录策略、过滤器实际限制、异常写入、其他保存入口仍待，不因格式更正而自动通过。

源码调用搜索未发现当前或基线其他TSX组件引用ScrollDiagnosticPanel，不能仅凭文件存在认定它是当前已暴露的用户入口，也不能由此宣称旧导出API已经迁移或获准延期。保留独立API清单待核对。最小化/窗口稳定、其他D、官方GUI全资料回退与A/B/C缺口保持，E完整验收仍待D稳定。没有新增延期、push、正式发布或默认下载切换。
