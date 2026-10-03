# 当前候选系统通知送达与清理

同一已安装候选host249dfd15/sidecar29d985c7，strict签名及hash前后一致，原通知门禁两档案通过，各3条turn_done/approval_request/ask_request。Rust核对实际Notification Center的精确内容和自有identifier，送达后精确删除且确认消失；后台2条实际hidden=true/active=false，原件hash/mode/mtime与credential身份重启稳定，sidecar/readiness清理。

原日志的成功夹具清理会删除本机receipt；随后仅在Python runner新增白名单字段输出，在删除前保存六条非敏感实际回执。所有原native送达/状态/原件/身份/退出断言未改变。第二轮详细结果见../2026-10-03-d-notification-receipts/receipts.json，两个前台样本actualActive=true，四个后台样本hidden=true。没有输出token、session路由或凭据，没有新增生产权限或改变OS通知设置。这里只更改验收工具，无需重建生产包。

此门禁不是横幅截图、实际点击、授权拒绝或重新授权、冷启动路由的现场验收；这些D项继续待验。D整体和窗口稳定性未完成，E未开启。
