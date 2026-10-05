# 模型服务删除入口修正

用户截图点到的是钥匙串删除；密钥来自其他来源时返回“钥匙串中没有密钥”并不会移除模型服务。原上方服务配置的删除功能已存在。本次在同一模型卡片增加明确的“删除模型服务”，点击先读取配置身份/revision，在当前设置窗口内确认，再调用原 delete_provider_config，保留后端身份/revision/内置服务/剩余默认模型保护。原按钮明确改名“清除钥匙串密钥”。不删除会话或已有钥匙串凭据，不修改 Wails 数据或用户当前配置。

配置操作与凭据/模型测试互斥；删除确认期间编辑器 disabled，密钥操作及测试不可执行，失败在独立 alert 显示，成功刷新配置编辑器和模型摘要。

回归：原 settings-api-key 全流程通过，新增同卡片确认/取消、带 revision 删除、配置删除不调用 keychain_delete。目标 ESLint 与完整前端 build 通过。浏览器使用 CUA in-app browser，真实 TauriSettings + 假 deepseek-pro 配置，http://127.0.0.1:5197/service-delete-qa.html / Reasonix Service Delete QA；内容/无 overlay/console error-warn 空、确认按钮/取消保留/确认卡片消失通过，截图见本目录。临时 fixture、页面和服务器均清理。未操作用户真实模型/密钥，真实安装包删除交互仍待用户实测。

用户改为直接交付 .app，不再生成 ZIP；构建产物与哈希另有回执。既有 D/E/A/B/C 待验缺口保持。
