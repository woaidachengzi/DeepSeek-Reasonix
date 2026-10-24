# 2026-10-24 双屏鼠标投递验收

范围：用户明确临时授权的 Reasonix Pointer QA，仅操作自身原生按钮和内联本地 WKWebView 页面；不加载浏览器用户数据，不输入真实邮箱或发送消息。

## 结果

[result.json](result.json) 保留四次独立计数：主屏 native/web 各 +1；左侧副屏 native/web 各 +1，副屏点击 X=-960。生产 `RXPointerBlock` 与 `RXPostWindowServerClick` 直接参与测试，没有跳过预检。测试后移除临时辅助功能权限；新进程的只读 preflight 返回 accessibility=0、input_posting=false、events_posted=0。

## 修正依据

第一次预检拒绝时，CG 窗口列表中 Dock layer=20 和 ChatGPT layer=3 的矩形覆盖了整屏。只读检查表明 AppKit 的实际鼠标命中查询返回其下真正接收点击的窗口，而不是这些覆盖层。现在检查实际 mouse-down 窗口的精确 ID，再独立核对 CG 所有者、层级、边界与 AX 前台焦点窗口。没有按进程名称豁免覆盖层；真正截获输入的其他窗口仍拒绝。

网页元素也可能不提供 AXWindow；改为核对已批准应用的 AX 焦点窗口，而不是依赖远程网页叶节点的所属窗口属性。测试夹具将预检放到工作线程，保留自身主线程响应 AX 请求。

[Apple windowNumberAtPoint 文档](https://developer.apple.com/documentation/appkit/nswindow/windownumber(at:belowwindowwithwindownumber:)) 描述返回鼠标按下实际命中的最前窗口。

## 后台进程补充验证与限制

四次实际点击时，QA 已有 NSApplication。随后补充了后台 Go 进程的惰性 NSApplicationLoad 初始化；独立只读工作线程实测及原生回归验证能够得到真实鼠标窗口、激活策略为 prohibited、没有创建窗口或发送事件。这项初始化在已有 NSApplication 的 QA 路径不改变命中逻辑。

本记录证明本机双屏原生投递网关及 WKWebView 按钮响应。它不证明完整模型/审批/回执链、Chromium、键盘输入、QQ 邮箱填写或发信流程已经验收。仍需按实际工具返回和页面回显判断任务成功，不能把 dispatched=true 当作任务成功。
