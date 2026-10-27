# DeepSeek / OpenCode Go 适配更新

核对日期：2026-10-27。范围是模型能力、推理参数、官方路由及费用估算；不新增独立 Search / Fetch，不改变已保存会话或用户凭据。后续 macOS Cua 安装包版本为 0.1.64，打包不等于已安装或完成真实供应商验收。

## 行为

- 官方 DeepSeek 新配置以 `deepseek-flash` 为默认模型；保留旧 Flash 与 Vision 名称、历史引用和已有默认模型。三种 Flash 名称均支持原生图片及 `low/high/max`；关闭思考沿用各协议既有字段。Pro 继续限制原生图片。逐模型图片关闭仍优先于默认能力。
- 当前官方 Flash 元数据优先于可能过时的内置 Pi 目录。自定义网关不继承官方端点能力，未知模型仍需元数据或用户声明。
- Go 推荐预设采用 `https://opencode.ai/inference/go/openai/v1`（Chat / Responses）和 `https://opencode.ai/inference/go/anthropic`（Messages）。模型列表请求使用独立的 `https://opencode.ai/inference/go/v1/models`；用户显式指定的模型列表 URL 保留。
- 新旧 Go 路径均附加 Reasonix 客户端标识和会话级稳定的 `x-opencode-session`。自定义头保持优先，仿冒域名、非 HTTPS、userinfo、查询参数等不获此适配。路由分组修复保留原配置采用的新旧路径族。
- Go 的新 Flash ID 是 `deepseek-v4.1-flash`。它在 Chat 路由提供图片、1M 上下文、384K 最大输出与低推理档位；不从旧 DeepSeek 搜索路径推断新 Anthropic / Responses 路径也支持该模型。旧搜索预设保持原路径和已验证型号，不自动扩展能力。
- 官方 Flash 高峰价格更新为每百万 token：CNY 命中输入 0.04、未命中输入 2、输出 8；USD 对应 0.006、0.30、1.20。低峰减半。旧 Flash / Vision 别名使用同表；Pro 价格不变。
- 价格按官方公布的 `2026-09-10T04:00:00Z` 划界，保留此前事件的旧费率和已保存报价。仅刷新精确官方端点上已知的旧默认价；自定义价、中转站和币种选择保留。沿用原峰谷调度，不新增法定节假日日历；费用仍为估算。

## 验证

- Chat / Messages / Responses 三种适配器捕获实际出站请求，覆盖新 Flash、两个别名、Pro，以及逐模型图片关闭；核对原始图片是否出现、模型 ID 保留和 `low` 的协议字段。
- 新旧 Go 路径 × 三协议验证实际 HTTP 请求头；同一会话保持稳定、切换会话更新标识。另测新路径的目录发现、显式 URL 保留、模型路由隔离及仿冒端点拒绝。
- 核对新默认值经启动归一化后仍为当前 Flash；核对旧价格刷新幂等、自定义配置保护、新旧费率生效边界、两币种报价及历史时间回放。
- 所有新增 HTTP 验证使用本地夹具和临时配置，不调用付费模型。真实供应商可用性、账号地区权限和已安装应用仍须另行验收。

本轮结果：适配、配置、计价、启动、事件和统计共九个包完整 race 回归通过；Agent / Controller / desktop bridge 的 DeepSeek、Go、图片能力及计价定向回归通过。修改前失败的五项官方契约检查用同一临时 overlay 复跑均通过。首次启动包回归发现一个沿用旧当前价的测试夹具，更新夹具后该包完整 race 重跑通过；没有放宽断言。最终 review 补全 Pro 元数据上限后，配置与 Provider 包完整 race 再次通过。

```sh
go test -race ./internal/config ./internal/provider ./internal/provider/openai ./internal/provider/anthropic ./internal/provider/responses ./internal/billing ./internal/boot -count=1
go test -race ./internal/boot ./internal/event ./internal/stats -count=1
go test -race ./internal/config ./internal/provider -count=1
go test ./internal/agent ./internal/control ./cmd/reasonix-desktop-bridge -run 'DeepSeek|OpenCodeGo|Vision|Billing|Pricing|CostQuote|ImageCapability' -count=1
```

执行日志位于本机临时目录 `/private/tmp/reasonix-deepseek-adaptation-audit-20261027/`。`final-race-tests.log` 保留首次启动夹具失败，最终启动结果见 `final-cost-pipeline-tests.log`，最终元数据结果见 `final-metadata-review-tests.log`。临时日志不随源码提交。

提交前 review 额外发现新 `deepseek-v4.1-flash` 被旧协议兼容目录推广到尚未验证的 Anthropic / Responses 路由。新增断言先复现该问题，再将该新 ID 限定为 Chat；旧搜索型号及路径继续兼容。另新增不覆盖请求 URL 的 transport 测试，捕获六种新旧路径实际构造出的 URL 和会话头，避免测试夹具的 `request_url` 覆盖隐藏路径错误。修复后配置及 Provider 完整 race 回归通过（`review-final-tests.log`），没有剩余阻塞发现。

## 官方依据

- [DeepSeek 模型与美元价格](https://api-docs.deepseek.com/quick_start/pricing/)
- [DeepSeek 人民币价格](https://api-docs.deepseek.com/zh-cn/quick_start/pricing/)
- [Flash 发布与价格生效时间](https://api-docs.deepseek.com/news/news260910/)。此旧公告中的 Pro 下线计划不作为实现依据；当前价格页仍明确保留 Pro。
- [图片输入](https://api-docs.deepseek.com/guides/vision/)、[推理参数](https://api-docs.deepseek.com/guides/thinking_mode/)
- [OpenCode Go 官方路由及会话头要求](https://opencode.ai/v2/docs/console/go/)
