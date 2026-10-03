# 单实例协作激活实验：未通过，已撤回

对照前一 74fefe10 完整窗口显式 second-instance 失焦与独立持久化 y344→342，尝试在 macOS 单实例插件 setup 前由新实例向唯一已有同 Bundle ID/reasonix-tauri 进程 yieldActivation，主窗口恢复路径补现代 activate；老系统按 selector availability 保留原路径。没有增加重试、延长超时或放宽焦点/几何/持久化断言；2px 问题没有混入改动。

锁定 Tauri PluginStore 的初始化 Vec 按注册顺序遍历；单实例插件通知 Unix socket 后 std::process::exit(0)。Tao set_focus 在主线程 makeKeyAndOrderFront 后调用旧 activateIgnoringOtherApps。Apple 的 [yieldActivation](https://developer.apple.com/documentation/appkit/nsapplication/yieldactivation%28to%3A%29?language=objc) 及 [activate](https://developer.apple.com/documentation/appkit/nsapplication/activate%28%29) 支持协作激活用法，但不保证成功。本次是对假设的实验，没有证明之前故障原因。

初版 clippy 因 objc2 PID API 所需 libc feature 与插件配置泛型缺失失败；补齐后 strict clippy 和使用原安装 74 的同版真实 bridge Rust 227 项回归通过，5 ignored。实验源码改动及新模块完整保存在 source.patch 与 single_instance_activation.rs。app/DMG 构建、只读复制安装、严格签名通过；源码 HEAD835266186 且 dirty，生产差异含4个跟踪文件和新模块。实验 app cb85c6b52aa7a82f8cd5e74e2a06297a98c4aea928236c4afc2a34111ee5511c、sidecar26939f1bfac4d46ded6541e1d6315a05dfcdcc7146b611b5c31f14eb8cc7de50，DMGee881a1d；完整摘要与位置见 install.json。

全新 managed 档案使用真实 exercise 生成窗口种子，restore-maximized、restore-normal 均通过；second-instance 失败，原 active/key/focus=false，restoreRequests/Completions=1，几何 x920/y344/2000×1400 正确，主宿主 kernel exit2。completed=3，explicit未运行，未运行完整48阶段或其他门禁。不能记为修复、稳定候选或2px问题通过。原生结果及内核回执在 native，现场root留存 control.json。

实验无效果，生产 Cargo.lock/Cargo.toml/main/tray 恢复 HEAD，新模块从生产路径删除，保留归档；本轮没有可放行的窗口修复。撤回后未需要重跑源码回归（实际恢复此前已验证的提交内容），也未把实验包当作正式候选。当前迁移候选仍是上一74的固定身份，其完整窗口失败与12组独立门禁维持原范围；cb 为失败实验。

CUA getState 本轮34.8785秒超时并reset；未重试或执行UI操作。这不证明屏幕锁定。只读 NSWorkspace/CoreGraphics 元数据的最终结果确认 loginDone=true/onConsole=true，当前另一个应用 frontmostActive=true；屏幕锁定字段缺失/null。初版使用错误OnConsole字符串与缺失值false默认值的输出不作为状态证据，复核根据当前SDK CGSession.h 的 kCGSSessionOnConsoleKey并采用null。最终源码与结果归档；此为测试后快照，不证明失败瞬间的状态或焦点竞争因果。

下一步需要与实际失败PID和阶段同时记录只读前台/活跃应用切换，定位未激活与已激活后失焦的差异；另继续定位有效种子恢复后2px坐标变动，保持精确持久化断言。D未完成，E未开启；系统授权/物理UI、多显示器pending与A/B/C等清单缺口保持。未发布、切换下载或推送。
