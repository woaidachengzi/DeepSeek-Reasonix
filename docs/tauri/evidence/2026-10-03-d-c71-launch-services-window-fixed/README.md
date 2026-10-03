# c71ab66c 正常 .app 启动完整窗口验收

实际安装包未重建或修改，宿主c71ab66c、sidecar94207d45、DMG7b6a7a5f。使用LaunchServices启动，两种私有档案各24阶段、48/48通过。每阶段必须匹配实际宿主PID和sidecar父子关系，kqueue内核退出码0，精确成功phase回执、sidecar/readiness清理、档案身份和保存的普通窗口状态保持。包/脚本/source快照见[result.json](result.json)，全部退出回执见[kernel-receipts.json](kernel-receipts.json)，测试后摘要、严格签名及无匹配运行实例见[post-run.json](post-run.json)。

包含最小化/全屏退出、最大化/普通状态重启、应用隐藏及后台关闭、Settings菜单隐藏/最小化/应用隐藏恢复、流式任务后台继续与菜单退出、外观持久化/失败回滚、真实WKWebView/系统剪贴板与完整有序原件恢复、实际AppKit取消、托盘语言、原生编辑菜单、第二实例唤起、关闭退出及重启。

原[裸宿主启动的Settings焦点失败](../2026-10-03-d-wails-env-package-continuation/README.md)保留；[首轮启动适配器空环境参数失败](../2026-10-03-d-c71-launch-services-window/README.md)也保留，修复不放宽任何原断言。正常.app启动取得系统激活上下文的[四项对照](../2026-10-03-d-c71-activation-context-control/README.md)独立通过；不能据此解释所有历史偶发故障。完整window默认门禁使用LaunchServices，裸宿主仍可显式选择window-direct诊断。

该结果是正常安装包程序窗口门禁，不替代物理托盘/Dock、真实组合键/IME、真实文件和目录选择/另存为、通知横幅/实际点击/权限重授、真实钥匙串全局来源授权/拒绝、不同缩放/拔插、零活动显示器复活后pending或任意旧版自定义共享目录互斥。D仍未完成，E未开启。当前候选的构建及官方旧版资料/配置备份回退证明保留；A/B/C和正式签名/公证仍阻碍正式发布。

本轮未重复浏览器打开验收，未正式发布或切换默认下载项。
