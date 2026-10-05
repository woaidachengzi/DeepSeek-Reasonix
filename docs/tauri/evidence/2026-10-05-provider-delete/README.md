# 模型服务删除入口修正

用户截图点到的是钥匙串删除；密钥来自其他来源时返回“钥匙串中没有密钥”并不会移除模型服务。原上方服务配置的删除功能已存在。本次在同一模型卡片增加明确的“删除模型服务”，点击先读取配置身份/revision，在当前设置窗口内确认，再调用原 delete_provider_config，保留后端身份/revision/剩余默认模型保护。原按钮明确改名“清除钥匙串密钥”。不删除会话或已有钥匙串凭据，不修改 Wails 数据或用户当前配置。

配置操作与凭据/模型测试互斥；删除确认期间编辑器 disabled，密钥操作及测试不可执行，失败在独立 alert 显示，成功刷新配置编辑器和模型摘要。

回归：原 settings-api-key 全流程通过，新增同卡片确认/取消、带 revision 删除、配置删除不调用 keychain_delete。目标 ESLint 与完整前端 build 通过。浏览器使用 CUA in-app browser，真实 TauriSettings + 假 deepseek-pro 配置，http://127.0.0.1:5197/service-delete-qa.html / Reasonix Service Delete QA；内容/无 overlay/console error-warn 空、确认按钮/取消保留/确认卡片消失通过，截图见本目录。临时 fixture、页面和服务器均清理。未操作用户真实模型/密钥，真实安装包删除交互仍待用户实测。

用户改为直接交付 .app，不再生成 ZIP；构建产物与哈希另有回执。既有 D/E/A/B/C 待验缺口保持。

## 内置 DeepSeek 补齐

首次 UI mock 验证的是 removable 自定义服务，不能证明内置 DeepSeek 可删除。后续 review 发现后端曾拒绝 api.deepseek.com 入口；现在按 Wails 保留配置/凭据、移除访问权限的原则处理当前服务，清空或回退默认、planner、guardian、recovery、vision、subagent 和 bot 模型引用。自定义服务仍物理删除配置并保留最后默认服务保护；当前运行会话需要用户另行切换模型，本次不改写其会话记录。内置服务单独移除，不连带删除其他服务。显式 provider_access=[] 保持移除最后一个内置服务；未声明访问表时先物化现有服务列表。

摘要不再返回隐藏服务；配置列表保留 hidden 标记用于“已移除的服务 / 重新添加”，恢复调用同一受鉴权、身份、revision、配置锁保护的命令，明确 restore=true，无密钥读写。旧 deepseek-pro 官方别名的恢复沿用配置层规范化，可显示为 canonical deepseek 服务。修正模型设置渲染遗漏 guardian_model，使引用回退实际落盘。

完整 Go bridge + internal/config 回归通过，覆盖有/无备用服务、移除重载不复现、revision 过期拒绝、配置/凭据来源/未来字段保留、恢复与引用回退。settings-api-key 回归新增隐藏服务重新添加、不触碰钥匙串。完整前端生产 build 通过。隔离 CUA 浏览器真实 TauriProviderEditor + mock host，5197/service-restore-qa.html，展开和恢复实际交互通过，错误/警告空；截图 browser-restored.png。fixture/server/tab 已清理。测试未读取或修改用户真实配置，也未进行真实付费模型请求。
