# 371a731af 提交后当前候选真实安装验收

生产源码未改。提交 371a731af2c1af632cc68d7b9fc0f2961aaaab0f 的 pnpm tauri:build 构建 app/DMG 成功（Preview source 非 dirty），DMG 只读挂载并复制到新私有临时目录，严格签名通过。本地 ad-hoc 候选宿主 74fefe10fb3b8a8be689f58c1eef6505a6252c47a0bd4a474f8a35c4328cbae8，sidecar 16e3761d616c6bc751acd4a3250d4f7b2851ac985ecd2d3f5219e21d4423d093，DMG b3a4555f0815db077491aa6b4a9c1511615e37d2604ebeac2ea917ed12a904ce，完整路径见 install.json。不继承 c71 的验收。

## 当前程序门禁

第一批 gates/result.json：app-data-boundary、boundary、explicit-boundary、parent-resolution、identity、package、profile、startup、failure-exit 九组通过；window 失败，之后三项按规则未运行。

window：managed 24/24 通过；explicit 完成 21 阶段后 second-instance 失败。恢复请求/完成均为 1，主窗口可见且几何保持，但 active/key/focus 为 false，原超时断言不放宽。当前主宿主精确内核退出码 2；总 46 份主宿主回执，45 exit0/1 exit2，见 window-kernel-receipts.json。失败原生 result/trace/普通状态及 kernel 回执另存 window-failure；私有原现场保留。本候选完整窗口不通过，不能记为 48/48。

continuation/result.json：独立补跑 lifetime、displays、notifications 三组通过。累计 12 个不同程序门禁通过。lifetime 包含双档案 idle/streaming、SIGTERM/SIGKILL 8 个场景；双屏包含 secondary 与跨屏种子恢复/重启，双档案 8/8，当前两屏均 scale=2，不证明拔插/不同缩放/零活动显示器复活；通知投递/移除/重启不证明实际横幅或物理点击。window 内通过部分不提升为另一个完整门禁通过。

## 原生全局来源迁移

native-env/result.json、native-test.log：新自有私有 HOME/假钥匙串，显式执行默认 ignored 的 native_wails_env_import_restart_delete_preserves_original，1/1 通过。Rust 测试宿主由当前源码编译，使用本轮实际安装 sidecar；test binary SHA 04991007b04d56b2833ce867c1419d29f282e14347976db3e50debc767569d5d。精确原生 default/search 指向自有 fixture，测试进程关闭 Security 交互，CLI 解锁仅自有假钥匙串。原生全局 .env 导入、拒绝重复覆盖、精确 dummy 读回、同档案重启就绪、删除以及来源 byte/inode/mode/mtime 保持通过；目标不复制 .env。正常用户 default/search/preference hash 前后相同；只归档结果，不复制正常用户钥匙串或私有源数据库。

这证明当前安装 sidecar 与原生凭据 backend 的 fixture 链路，不能替代安装主宿主设置页实际点击、managed/跨档案、真实系统授权/取消/拒绝重授，也不证明旧 Wails keyring 来源读取故障已修复。

## 单阶段单实例对照

使用全新夹具、从原失败现场只读复制普通窗口种子；源失败现场不改，不重试，不修改生产代码。managed second-instance 通过。explicit 原生 phase/result=true 且 kernel exit0，但持久化 gate 检测 y 从 344 改为 342，判失败（保留其他正常窗口字段与原断言）。这与完整流程失焦是不同观测，不能把 isolated 原生成功当成完整 gate 通过，也不能据此统一推断原因。两种现场分别在 reasonix-single-instance-control-* 目录。

锁定单实例插件通过 Unix socket 通知后立即 exit(0)，已有应用回调在主线程执行恢复；锁定 Tao set_focus 使用 key-window 与旧 activation 接口。Apple 的 [yieldActivation](https://developer.apple.com/documentation/appkit/nsapplication/yieldactivation%28to%3A%29?language=objc) 和 [activate](https://developer.apple.com/documentation/appkit/nsapplication/activate%28%29) 描述协作激活。本次只核对源码及文档，尚未证明焦点丢失因果或实现修复，不引入延迟重试/强行激活其他应用来制造通过。

## 当前候选回退及结束状态

legacy-rollback.log：固定摘要的官方 Wails 1.38.3 Desktop/内嵌 CLI 验证通过；其实际历史/tab/model、Global 文件和文本/图片附件引用、文件编辑及检查点、活跃会话 writer 拒绝、Preview 导入与持久化、原版同会话继续与最新检查点代码回退、较早冲突拒绝、旧 CLI 配置回读及原树 byte/mode/mtime 保持通过。不是 Preview 图片渲染、全资料快照或任意旧版自定义目录 lifetime 锁。

backup-apply/rollback-receipt.json：当前包真实生成备份，在 Preview 和 sidecar 退出后复制到另一私有旧 HOME，恢复 default_model=native-import/alpha，固定官方 CLI 回读 currency=CNY/exit0；恢复树与 Preview/备份树不变。

post-run.json：本轮新包及官方旧包严格签名通过；新 host/sidecar hash 保持，无匹配当前包/旧版运行进程。原失败保留。用户已确认的浏览器到达链路没有重复运行。D 未完成，E 未开启；下一项优先定位单实例失焦及精确坐标持久化差异，随后继续物理菜单/快捷键/IME/托盘/文件目录保存/通知权限/系统钥匙串授权、多显示器 pending 等清单项目。A/B/C 与正式签名/公证缺口保持；未正式发布或切换默认下载项。
