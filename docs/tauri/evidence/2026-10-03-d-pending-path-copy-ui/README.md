# 当前 d1 候选托管档案路径复制 GUI

使用原始 smoke-native-ui-clipboard.py --profile managed --scenario path。实际 CUA 设置页配置目录复制按钮点击后 Help 为已复制；共享原生剪贴板精确值/代次 claim 通过。空默认/当前工作区复制禁用。返回工作区 Cmd+V 的实际文本与 control.json expectedPath 完全相符，随后清空草稿并 Cmd+Q。runner exit0，原件/身份保持、sidecar/readiness退出清理与原剪贴板所有条目顺序、类型、字节恢复校验通过。

观测文件是工具操作及AX返回的转录摘要，未另存截图。不保存用户原剪贴板快照，仅保存夹具自己生成的 owned marker。夹具成功后由原工具清理。本轮未改生产代码；静态搜索确认当前Tauri组件没有直接BrowserOpenURL或ClipboardSetText调用，其余旧入口和事件直连不能因此计为全量迁移完成。explicit、消息/hooks复制和真实拒绝等未验项保留，D未完成/E未开启。
