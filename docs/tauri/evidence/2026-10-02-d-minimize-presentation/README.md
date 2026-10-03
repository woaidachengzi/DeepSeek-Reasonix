# macOS D：最小化与原生呈现诊断

2026-10-02。D 未验收，E 未推进。前一轮为已验证的启动恢复修复；本轮专门诊断
窗口失败，没有修改生产窗口恢复策略，也未替换或放宽完整窗口门禁。

## 新证据

首先在上一安装包 `49e8b9c…` 通过独立的真实 Minimize 菜单角色复现失败。
该路径走 AppKit responder action，同样 Will/Did Miniaturize 为 0；因此不能把
问题只归因于 Tauri minimize 的异步消息路径。日志在 `previous-native-role.log`。

增加仅在原生 smoke 中安装的 NSWindow 遮挡状态、NSApplication 激活/失活观察器，
保留原有最小化和 key 事件计数，增加最多 32 条单调时间事件及本窗口编号/level。
不记录其他应用、标题或内容。观察器仍在主线程注册/移除，回调仅持有线程安全计数。
原生菜单检查失败现在保留私有夹具，不再由 TemporaryDirectory 删除。

新 `--presented` 探针在独立档案中要求应用激活、主 key 窗口且应用/窗口实际可见
呈现后才调用原 API；它是诊断对照，不能替代原完整门禁或原 API 样本。

## 本包结果

完整生产 app/DMG 构建、严格 clippy、DMG 校验/只读临时安装/签名/卸载镜像通过。
本包 host SHA-256：`c29beab60b5b30e7634b2ff2114b4dfe189ec673e44324fb13ae487d8d8b5bbf`。
sidecar 摘要与上一包相同。安装路径、DMG 摘要、源码补丁及逐项结果见
[result.json](result.json)。源码仍未提交。

- **原 API 样本失败**，托管首项失败，显式及后续 native role 未执行。
  `settings-minimize-ready/requested` 时 applicationActive、nativeKeyWindow 和
  nativeOcclusionVisible 都是 true；Will/Did Miniaturize 仍为 0。
  时间序列：609 ms 可见状态变化，618 ms resign key，1182 ms 可见状态变化，
  3835 ms application resign active。此项证据反驳“仅等待可见即可解决”的简单解释。
- **可见呈现对照失败**，托管前提一直不能同时满足，未请求最小化；显式和
  后续 native role 未执行。初始窗口曾可见，hide/show 后为 key 但不可见，随后
  1027/1028 ms 失去 key/应用激活。不能据此声称对照下最小化通过或失败。
- **正常 package smoke 通过**：两种档案正常启动、真实 bridge、档案隔离、私有
  凭据身份、系统通知授权查询、Global 工作区和退出清理通过。

两个诊断失败分别保留原日志和私有夹具路径，没有重试覆盖或统计成通过。只增加时序
诊断未证明根因，也未证明代码或环境归属。完整窗口、物理托盘/Dock/IME、混合缩放/
拔插、通知/钥匙串权限交互、旧宿主目录互斥及 A/B/C/E 仍待验收。没有正式发布授权。

独立复跑（需新私有档案，脚本拒绝普通 Preview 已运行）：

```sh
python3 -B tools/tauri/smoke-native-menu-window.py '/path/to/Reasonix Tauri Preview.app' --api-startups 1
python3 -B tools/tauri/smoke-native-menu-window.py '/path/to/Reasonix Tauri Preview.app' --presented
```

两个命令是不同实验；后者不是前者失败后的重试。原完整窗口命令及成功条件保持。
