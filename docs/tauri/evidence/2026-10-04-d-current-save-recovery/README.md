# D：当前包实际原生保存、覆盖取消及取消后重试

**基线更正：** 本轮下文将前端交互记录JSON与独立滚动诊断ZIP入口直接比较，归属有误。[对应控件与记录器核对](../2026-10-04-d-diagnostics-baseline-correction/README.md)证明Wails对应前端交互入口也导出JSON，记录器字节一致。下文原对照判断保留供审计，不再作为该入口格式缺口；实际保存回执未改，其他入口与默认目录等仍待。

源码/权限未改，基于 `17ee3b5a4`，仍为当前真实46包（完整host/sidecar SHA与strict签名后检在postcheck）。全新explicit root `/private/tmp/reasonix-launch-services-4fd_ipo5`，host93945/sidecar93952；启动器先确认x=-3200/y142/2560×1640/scale2，使用用户确认的左侧屏幕2。CUA绑定exact live app和本root独有origin，未发送模型请求。

## 基线与实现边界

Wails 1.38.3 ExportScrollDiagnostics在原生SaveFileDialog中指定当前工作区默认目录、ZIP过滤器与安全默认文件名，取消不写。当前Tauri ExportFrontendDiagnostics校验schemaVersion2，原生host保存JSON过滤器/文件名，取消返回false；先同步0600临时兄弟文件，再persist替换，不直接截断原文件/跟随目标symlink。相关片段已冻结。main renderer仍仅dialog:allow-open；Save由受限host命令提供，没有新增通用filesystem/shell权限。

**Wails导出ZIP、当前入口导出JSON，默认目录策略也不同。** 本轮只验收当前诊断记录入口的原生文件选择/写入/取消恢复，不能由此宣布诊断资料格式、分析工具或Wails全导出功能等价。ZIP资料及各保存入口的完整对照保留为未验缺口。

## 实际操作与磁盘核对

1. Settings→诊断，开启本地前端记录后导出，真实save-panel。Go To Folder导航本夹具workspace，Save As填写「诊断 保存.json」，Save后文件真实存在，1475bytes、schemaVersion2有效JSON、mode0600。UI回到off/非待导出。
2. 第二次记录导出同名文件，原生覆盖确认「already exists / Replace」。点击Cancel回到Save面板，此时原文件sha256/mtimeNs/mode逐项与初次保持。
3. 复制原测试文件为本root内.original.json备份，再Save→Replace；新文件有效schema2、1232bytes且摘要改变，仍0600；原备份摘要保持。只覆盖本次代理创建的测试数据，恢复副本保留。
4. 第三次记录，在Save面板填写「取消 不应写入.json」后Cancel。该文件未生成，UI显示「待导出」及5条记录，并保留导出/取消记录按钮。
5. 重试导出，原默认reportId文件名保持。改名「取消后 重试.json」并Save，磁盘有效schema2/events恰5条/mode0600，UI回到非待导出。第一次点击被工具过期应用状态保护拒绝，刷新AX后才实际重试，不算产品写入失败。
6. 实际Cmd+Q，原session58847 terminal0，kernel/openexit0、sidecar/ready无残留；后检exact host不存活、无Preview、签名0与两SHA保持。退出后未再读/重绑死亡CUA对象。

固定元数据、退出回执、动作/AX人工转录与源码片段归档；不复制真实诊断payload、私有HOME/core、keychain、原始剪贴板、二进制或其他目录。事件数量来自文件JSON和实际AX，保存结果不以面板消失代替磁盘核对。SHA256SUMS覆盖其余所有文件。

## 未完成项

只覆盖当前46包一份explicit档案的诊断保存。managed、过滤器实际限制、权限拒绝/磁盘失败、symlink真实面板行为、所有主题/会话/技能/附件等入口、ZIP资料等价仍待，不能继承旧包通过。源码只读复核没有发现Focused(false)触发自动恢复：恢复所有者是明确Show/Dock/托盘/单实例和启动意图，菜单Minimize为原生角色；此结论不是最小化根因定位。最小化/间歇稳定、其他D及官方GUI全资料回退仍未完成；E完整验收继续待D稳定，A/B/C和正式签名公证等缺口不变。无新增延期、push、正式发布或默认下载切换。
