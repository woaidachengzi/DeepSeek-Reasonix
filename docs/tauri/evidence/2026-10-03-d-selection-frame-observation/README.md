# 43dc4305：选区失败的帧、焦点和可见性诊断

2026-10-03，本批推进失败定位，D未完成、E未开启。基线HEAD `b886f0117`；生产前端、窗口/权限、sidecar逻辑和依赖未改，仅增加私有档案 opt-in 原生探针的固定结构观察。不是稳定性修复或整组验收通过，不发布/切换默认下载。

## 实际失败的新证据

native/result.json：selection-send failed/exit1，managed ui-selection-send kernel2；explicit未执行。原队列package/failure-exit not-run。断言与15秒时限未放宽，未复用失败档案或反复运行直到通过。

`failure/reasonix-native-window-trace.jsonl` 三点 selection-before-request/requested/action-failed：mainPageFinished=true、nativeVisible=true，却 applicationActive=false/nativeKeyWindow=false/nativeOcclusionVisible=false，monitors=[]。`failure/reasonix-native-selection-observation.json` 在原断言终态记录：documentVisible=false、documentFocused=false、独立requestAnimationFrame回调未执行、settings/modal均无、选区长度0且collapsed、anchor/focus仍位于transcript、action存在但closed/disabled。

这些是**失败当时**的真实数据，不是上一批失败后的CG观察。本次已缺少可见页面和有效正文选区前提；共享按钮路径以requestAnimationFrame调度，期间独立帧回调也未执行。不能从该环境把配置映射判作失效，不能确认选区何时/为何折叠，也不能把上一b1失败或历史窗口偶发问题归为同一根因。没有强制运行帧回调、计时器回退、重复点击或改生产保护来取得通过。

探针只返回固定boolean/长度字段，不返回正文/输入内容；额外原生读回与独立rAF注册会增加观察时序，不称它完全无观察影响。设置打开/录键/本机配置/关闭的既存检查已过，真正的Add、第二轮引用、旧绑定停用和重启配置/历史检查未执行。Mac此前CUA锁定，尚无用户解锁答复；解锁并具备可见页面/活跃显示器后再做同路径与物理验收，现失败状态保留。

完整剪贴板原件恢复通过，失败日志保留；当前失败夹具未重用，未归档其HOME/core或原件剪贴板。末次无自身sidecar/readiness。

## 构建与源码回归

首版strict clippy因诊断helper未使用NSObjectProtocol import失败，原日志及first.patch保留；首版build带该warning完成，但未安装用于验收。移除import后strict clippy通过，重新实际构建/DMG安装/签名通过。当前真实内嵌sidecar Rust **231 passed / 0 failed / 5 ignored**。前端源码未改，不重复上一提交完整Tauri/transcript回归；本次build的lint/契约/类型/bundle门禁实际通过。

安装目录 `/private/tmp/reasonix-d-selection-observation-installed-ujm5lp1u/Reasonix Tauri Preview.app`。

| 对象 | SHA-256 |
| --- | --- |
| host | `43dc43050b8e7638863caf91f22f8bde2974c1a7b9cef9911f4adf48f59cba48` |
| sidecar | `bdbb0753c881c9198e4f7e953b87aba60e8b10332513758bbd67388c3defb11a` |
| DMG | `14cd9a636109c901336698611d84198d415d6a56b1f8bed778ba396430c1fd5d` |

strict ad-hoc签名/摘要末次保持，真实plist ID的Preview与Wails均无运行实例。不是正式Developer ID签名/公证。各执行源和依赖冻结，SHA256SUMS覆盖原始日志/JSON/源码，不包括二进制或秘密档案。

## 当前包独立退出与配置回退

process/result.json：package/failure-exit/lifetime三项独立通过；原队列not-run不改写。lifetime两档案idle/streaming × SIGTERM/SIGKILL共8项，真实sidecar kernel正常退出、原件保持、同档案重启身份通过；不是物理Quit或整组窗口验收。

backup/：当前包导入/编辑/重启/显式拒绝后，把实际配置备份应用到另外独立0700私有legacy HOME的0600配置；自己的host/sidecar停止后，官方Wails 1.38.3内嵌CLI固定摘要核对、只读回读和正常退出通过。来源/备份/恢复树受保护字段保持。仅配置备份应用，不代替完整资料/官方GUI历史/附件/检查点回退；本候选没有继承b6/b1的其他实际包证据。

## 剩余目标

D继续推进：选区实际发送/配置重启与物理快捷键/IME、完整窗口和全屏、重新活跃/混合缩放/拔屏、文件目录确认保存、通知权限/点击、钥匙串授权取消/旧服务、页面未Finished可见异常恢复、任意旧目录互斥及历史间歇窗口仍待验。其余A协议/事件/错误面、B旧Global/Markdown图片与大历史、C复杂进程生命周期以及正式签名公证仍阻碍发布。E remote/bot/updater/复杂页按D验收且Preview稳定后推进，未自行延期待验项目。

下一步不在不可见页面反复尝试选区；补足可见环境后验收该路径，同时继续不依赖物理解锁的D异常恢复与边界改造。
