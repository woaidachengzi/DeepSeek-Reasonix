# MiMo v2.6 模型目录

用户截图的 MiMo Token Plan CN 编辑表单只显示 mimo-v2.5-pro / mimo-v2.5，原因是本地 curated preset 仍使用旧的静态目录，与 API Key 就绪/系统钥匙串无关。

2026-10-05 核对小米官方模型页：
- https://mimo.mi.com/models/zh-CN/mimo-v2.6-flash
- https://mimo.mi.com/models/zh-CN/mimo-v2.6-pro

官方提供 v2.6 Flash/Pro，并说明 Token Plan 同时覆盖 v2.6 和 v2.5。更新八个 MiMo API/Anthropic/Token Plan 区域预设：新建默认 mimo-v2.6-flash，目录包含 Flash/Pro 和原 v2.5 兼容项，新增官方图像能力和已核对的国内价格元数据。没有编造 mimo-v2.6 这个无后缀型号。

旧目录升级限定在已识别预设、官方端点/协议、没有 request/chat 覆盖、完整原 v2.5 两模型列表且非单模型配置；仅在读取视图补齐目录，不修改配置文件或现有默认模型。自定义列表、网关、凭据名、显示名、显式视觉选择、自定义价格和当前选择保持。视图刷新后已有服务可以选择 v2.6 Flash；旧默认不会被静默替换。

完整 internal/config + cmd/reasonix-desktop-bridge 回归通过，见 go-regression.log。新增回归覆盖八个预设、幂等升级、默认/凭据/自定义价格保持、自定义路由/目录保持、只读加载不写盘、已有旧服务可见新型号。未读取用户 profile、API Key 或钥匙串，未请求真实 MiMo。包内 smoke 和 app-only 构建通过，详见下文；D/E/A/B/C 其他缺口保持，正式发布另行授权。

干净源码 ea2083349 构建 app-only 成功，生产前端、sidecar、Rust release 与 strict ad-hoc signature 通过，见 package-build.log / package-receipt.json。上一份已核对 sidecar hash 的 6fef0d95d 程序已备份为 .app.rollback，路径见 receipt；可停止新包后复制回原 .app 目录回退。

包内 sidecar 用一次性旧 MiMo Token Plan CN 两模型配置测试：GET provider-configs/presets 与 providers 两个设置/对话数据源都返回 v2.6 Flash/Pro；旧 default 保持 v2.5-pro，读取不写盘，新预设默认 Flash。随后仅对一次性夹具添加本机 mock request_url 并显式保存升级目录，选择 v2.6-flash 后真实对话请求使用该 ID；保存轮换 Key 后同会话新请求使用新 Key、历史/审批保留；非法配置返回 409、历史可读、恢复后发送；切换 v2.6-pro 发送、重启恢复后 probe 同样使用该 ID。共 5 次本机请求，两次 shutdown exit0/readiness 清理，档案和进程清理。见 package-smoke.json / 可重复夹具 package-smoke.py。原生 GUI 及真实 MiMo 网络不计为验收通过。
