# 实际键盘剪贴板往返与失败恢复工具修复（2026-10-02）

同一安装候选 host `e0fc4d12`，本批没有修改生产剪贴板实现或放宽内容/generation
门禁。新增 `probe-interactive-clipboard.py` 只启动明确的签名 Preview/sidecar，
先以已有 Swift fixture 保存全部 ordered item/type/bytes，再在新的私有档案等待
CUA 实际操作及确切 PID 的 kernel Quit 回执。所有证据只含测试 canary/元数据，
没有复制或输出原剪贴板内容/恢复载荷。没有发送模型请求。

## 首轮失败与实际影响

root `/private/tmp/reasonix-interactive-clipboard-ysnc1ptw`，host 私有 root
`/private/tmp/reasonix-launch-services-yit94j4z`、PID 87386/sidecar 87394/open 87384。
句首 canary 在原生文本建议/选中时变成 `Reasonix-...`，不满足夹具精确小写 canary。
没有记 Copy 通过。后续 AX 文本选择返回 -10005；300 秒等待最终超时，runner
按确切私有 PID 清理，未取得成功 Quit 回执，也未查询失效 binding 或重启普通档案。

**实际缺陷：旧 NativeClipboardFixture 在恢复因变化后的 generation/内容被拒绝时，
保留当前剪贴板，却仍删除原快照。因此该轮开始时的原剪贴板没有恢复，备份已删除，
无法从本夹具取回。已向用户说明；不声称本次缺陷被后续通过追溯修复。**
没有读取当前其他内容来猜测原内容，也没有调整系统文本建议/自动更正设置。

## 恢复工具修复

恢复返回非 0（包括保护新内容的 changed=2）时，在调用方可能清理的目录之外
创建 owner-only recovery 目录，保留 original.plist 与 ownership marker；中断时
保留原异常并报告恢复位置，正常结束遇到变化仍报错。只有实际恢复成功 0 才删除
原快照。不覆盖其他应用的新内容，不把 snapshot 文件存在当作恢复完成。
假数据回归验证调用方目录删除后恢复副本仍存在且为 0700/0600：旧代码三个用例中
一失败、一错误，修复后三个通过；这些回归不访问系统剪贴板。

手动验收等待改为 900 秒，只有该 clipboard wrapper 显式采用；普通 launcher
交互默认仍 300 秒、非交互 25 秒。只读窗口观察器仍至多 300 秒，不延长或改变
生产窗口行为。测试 operator 应保留当前 profile/PID，不查询 Quit 后 binding。

## 新样本真实往返通过

clipboard root `/private/tmp/reasonix-interactive-clipboard-4tn_bslf`，host root
`/private/tmp/reasonix-launch-services-emopdmw_`，host 87917/sidecar 87924/open 87915。
已核对当前空会话 origin `13ebbf8a2af22fb2d37481d97141de45` 与私有 profile metadata
一致。输入 `0 reasonix-native-clipboard-<nonce>`，通过实际 Cmd+Left/Right×2/
Cmd+Shift+Right 选中唯一小写子串，避免句首建议的夹具前提变化；没有改生产输入设置。

- 实际 Cmd+C 后，helper 只读核对精确子串、单 item 与比 Copy 前更新的 generation，
  保存 copy ownership 回执。
- 实际 Cmd+X 后，AX 草稿只剩 `0 `，helper 核对相同精确子串、再次增加的 generation。
- 实际 Cmd+V 后，AX 草稿重新精确为原测试全文；helper 核对 Cut generation 未改变。
- 实际 Cmd+A/Backspace 后占位文案出现、发送禁用；没有发送消息或保留草稿。
- 实际 Cmd+Q，kernel/open 均 0，host/sidecar/ready 无残留；退出后没有读取旧 binding。
- wrapper 最后核对 ownership，Swift 恢复**第二轮开始时**的所有 item/type/data/order
  并逐字节比较成功后，才删除原载荷；`originalFormatsRestored=true` 仅指这一轮，
  不指首轮已经失去的备份。

本批补齐一个实际 keyboard/plain-text 往返样本，不代表富文本、全部 IME、全部
快捷键、剪贴板 ACL/多窗口或整个 D/E 通过；此前门禁仍按包归属保留。窗口最小化/
完整全屏、物理托盘/Dock、多显示器异缩放/拔插、通知/钥匙串拒绝交互、旧数据广义
互斥/回退、A/B/C/E 与正式签名等缺口继续保留；未发布或切换默认下载。
