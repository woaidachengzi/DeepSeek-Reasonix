# 空输入继续候选包 0.1.5

源码 `29c41076e`，macOS ARM64 原资料接管 App／DMG 0.1.5。App 及只读挂载 DMG 内的 App 均通过严格签名核验，host 与 sidecar 二进制一致；摘要见 `package.json`。构建包含 hooks lint、类型检查、滚动写入及资源大小门禁。

功能回归见[空输入继续](../../EMPTY_COMPOSER_CONTINUE.md)。补充 race 用例验证 provider 返回错误后，直接继续能够得到恢复回答，用户问题数仍为 1。当前包未替换 Applications 中的副本。

本轮 0.1.5 的新行为通过 Go／Rust／React 回归及包完整性校验；此前原生点击验收记录为 0.1.4，不将旧记录改写为新版通过。
