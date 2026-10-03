# macOS D：身份元数据启动拒绝与恢复

2026-10-02。持续目标进行中；D 未结案，E 未推进，无新增延期或发布授权。

## 实现与基线

Preview 为隔离原生钥匙串和 WebView 持久化而使用档案身份元数据。Wails 1.38.3
没有这套 Preview origin；此处不能通过丢弃损坏身份或生成新身份来绕过恢复要求。
既有身份库已经支持备份恢复和冲突拒绝，但上一包只正常处理默认目录配置拒绝。
身份解析错误从 Tauri setup 返回时，在 Ready 回调触发 panic：旧安装包实测
exit=-6，虽然包含恢复文案，仍是崩溃退出，回归明确失败。

现在初始化保持原顺序，提取为 `setup_preview`，所有返回的初始化错误由 setup
调用处输出 `Reasonix could not start: …` 并以退出码 1 结束。此项覆盖返回错误，
不承诺捕获任意 panic 或提供启动错误原生弹窗。身份检查仍在创建 WebView/bridge
之前执行；没有重写原身份、改变恢复语义或扩大 renderer capability。

## 当前候选与结果

真实 DMG 校验、只读挂载、临时复制安装、严格 ad-hoc 签名和卸载镜像通过。
当前安装：`/private/tmp/reasonix-d-identity-installed-t2jq7dom/Reasonix Tauri Preview.app`。
host SHA-256 为 `49e8b9cda038bd5f002178079aa3895690dbff3ee1d8489334dd0c8b60fedd1a`；
DMG 和 sidecar 摘要见 [installed.json](installed.json)。源码仍未提交，构建来源
补丁、脚本及逐项日志见 [result.json](result.json)。

- 完整 Rust 回归使用真实 Go bridge：218 通过、0 失败、2 项按原规则忽略。
- 严格 all-targets clippy、完整生产前端/host/sidecar/app/DMG 构建通过。
- 默认叶目录三项拒绝、正常托管/显式启动与配置导入/恢复/显式拒绝通过。
- 身份门禁 6/6：托管/显式 × 损坏/两个合法身份冲突/链接主文件。
  拒绝均 exit=1、有恢复说明，无 sidecar/readiness；主备份、链接目标和原 canary
  的字节、mode、mtime、inode 保持。仅在私有夹具修复备份后，正常启动/退出和
  package smoke 收据通过，损坏或链接主文件仍不覆盖。
- 启动中断 12/12：两种档案 × TERM/KILL × 三个启动阶段，sidecar 内核正常退出、
  原件保护、readiness 清理及同档案重启通过。
- **完整窗口失败**：托管档案先完成 8 项，随后 Settings 最小化前提失败；显式
  档案尚未开始。Will/DidMiniaturize 为 0，主窗口请求时为 key、但 occlusion visible
  为 false，超时后失去 key 状态。保留原日志及失败夹具路径，未把历史 46/46
  成功转记本包，也未重跑覆盖失败。原因仍待进一步定位。
- 串行 runner 在窗口失败后停止，原清单 lifetime 保持 not-run；同一包另行独立
  lifetime 切片 8/8 通过，摘要匹配，单独保存 [独立结果](lifetime-independent.json)。

上一候选的双屏、外链、通知、真实原生钥匙串及官方 Wails 回退仍属于
[上一包证据](../2026-10-02-d-current-candidate/README.md)，本轮未重复这些切片。
当前 sidecar 与上一包摘要一致；这不代替新 host 的全部交互或全量数据回退验收。
窗口稳定性、物理托盘/Dock/IME、混合缩放/拔插、通知权限与点击、钥匙串权限与
设置页迁移、旧宿主目录互斥、A/B/C/E 和正式签名/公证继续保留发布缺口。

复验身份拒绝与恢复：

```sh
python3 -B tools/tauri/verify-installed-d.py '/path/to/Reasonix Tauri Preview.app' \
  --output /private/tmp/new-identity-evidence --gates identity
```

脚本只使用临时 HOME、固定假元数据与 canary；禁止操作已经运行的普通 Preview。
失败保留证据，成功删除自己的夹具。此项不接受真实钥匙串拒绝或设置页交互。
