# 原exercise：页面就绪/首次Show两因素诊断

2026-10-03。源码基线bba177b61加source.patch，生产窗口行为、权限与依赖未修改；新增opt-in三种诊断phase，原exercise仍调用false/false分支，几何/最小化/恢复/最大化断言保留。新增最小化成功后的只读原生快照，不改变动作前提或容差。

## 当前真实安装身份

安装 `/private/tmp/reasonix-d-minimize-factors-installed-a_1t60sl/Reasonix Tauri Preview.app`。host `f6bc6f89e6370e7deb3c041b70e39afcc3c76ae561a729b1470dbdd4e4c1e30f`，sidecar `cef6f224eea76402475d3af3c87b324c0c42248fe1ecd2b5f3848dcb7f759452`，DMG `c1a1058aea000c6305c9c13d05c4abf962581d0b974913771946065afeede966`。真实构建、只读复制安装/严格ad-hoc签名通过，目录与SHA见install.json；不是干净HEAD构建，不替换用户安装。

## 八例原始结果

每种组合/档案只执行一次，新建私有0700档案。未等待页面不等于页面必然未完成；本次动作时实际值见comparison.json。

| 档案 | 页面等待 | 原额外首次Show | 实际请求时Finished | Show请求计数 | 结果 |
| --- | --- | --- | --- | --- | --- |
| managed | 无 | 保留 | false | 2 | 通过 |
| managed | 有 | 保留 | true | 2 | 通过 |
| managed | 无 | 去掉 | false | 1 | 最小化失败/kernel2 |
| managed | 有 | 去掉 | true | 1 | 通过 |
| explicit | 无 | 保留 | false | 2 | 最小化失败/kernel2 |
| explicit | 有 | 保留 | true | 2 | 通过 |
| explicit | 无 | 去掉 | false | 1 | 通过 |
| explicit | 有 | 去掉 | true | 1 | 通过 |

6例通过，各实际nativeMiniaturized=true且Will/Did=1；2例失败，5秒原有超时、Will/Did=0、nativeMiniaturized=false。错误没有自动重试，CLI最终返回1；不把6项成功包装为整组通过。所有native result/trace/kernel和只读NSWorkspace时序均在factors。两个失败root保留，后续没有复用，因此没有覆盖原失败回执。成功root只在回执复制后清理。此对照不验证完整48阶段，也不访问系统剪贴板。

## 能排除什么，不能证明什么

一次Show和两次Show均有失败，因此额外首次Show不是这些失败的必要条件；一次Show也不能保证成功。等待页面完成的4例均成功，但不等待的4例也有2例成功，不能把页面未完成当作充分失败条件，更不能据4项成功断言根因/修复。Loaded等待还改变启动时序，仍需要按原状态链与基线控制验证。

基线源码wails-main.go明确StartHidden=true；wails-app.go的domReady恢复几何后调用showMainWindowFrom。当前Tauri的实际exercise-ready/request快照在页面Finished=false时已visible/key=true，存在可操作窗口暴露时序差异。下一项收敛隐藏启动、可信主页面与宿主初始化完成后的一次性呈现，以及启动期间Show/退出请求的状态归属；这项基线行为差异值得修正，但不是已证明的历史最小化根因。不得只改变原测试使其更容易通过。

## 相关源码与同包回归

strict clippy通过；使用当前安装sidecar的Rust **227 passed / 0 failed / 5 ignored**，不把ignored视为通过。app/DMG构建内前端lint、类型/契约、CSS和原预算通过。当前同包独立package/failure-exit/lifetime三组通过（后者双档案×idle/streaming×SIGTERM/SIGKILL共8项，sidecar kernel exit0、清理/原件/重启保持），来源/工具冻结与签名见boundary/result.json。这些通过不代替失败的最小化组合。

原窗口生产流程没有改动。源码、基线及完整失败均保留，不继承22ee/e7等历史包全屏/窗口/通知/凭据/官方回退为当前f6包验收。当前包其余D和备份实际回退仍待继续；物理UI/授权/显示器等缺口不新增延期。D未完成/E未开启，A协议事件错误、B旧Global图片/大历史、C真实工具进程生命周期及正式签名公证仍阻碍发布；没有push、发布或默认下载切换。
