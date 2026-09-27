# Preview settings parity with Reasonix 1.38.3

This inventory compares the stable desktop settings navigation in
`desktop/frontend/src/components/SettingsNavigation.tsx` with the Tauri Preview
settings in `desktop/frontend/src/tauri/TauriSettings.tsx`. It records what the
Preview settings UI actually reads or changes as of 2026-09-27. “Not in settings”
does not imply that the Go runtime lacks the underlying capability.

| Stable section | Tauri Preview status | Current behavior / missing work |
| --- | --- | --- |
| General | Partial | Appearance mode, conversation width, text size, standard/deep process disclosure, notifications, and macOS close behavior work. Language, currency, approval defaults, sound, and status bar controls are not wired. |
| Model preferences | Partial | Default model is read and saved through the bridge for new conversations. Stable runtime model preferences and assignment controls are not present. |
| Model services | Partial | Provider readiness and model counts are shown; required API keys can be saved to or deleted from the system keychain. Provider creation/editing is not available. |
| Usage statistics | Not in settings | No Preview settings view or verified usage data contract. |
| Bots | Not in settings | No Preview bot-management view or verified mutation contract. |
| MCP and tools | Partial | Global and project MCP servers can be listed, added, edited, and deleted through the native bridge. Other stable tool settings are not represented. |
| Remote SSH | Not in settings | No Preview remote-management view or verified host contract. |
| Agent Skills | Not in settings | No Preview skill-management view or verified mutation contract. |
| Subagents | Not in settings | No Preview subagent-management view or verified mutation contract. |
| Plugins | Not in settings | No Preview plugin-management view or verified mutation contract. |
| Memory | Not in settings | No Preview memory-management view or verified mutation contract. |
| Hooks | Not in settings | No Preview hook-management view or verified mutation contract. |
| Diagnostics | Partial | Bridge health/restart and session catalog audit/refresh are available in settings. The existing runtime drawer also contains event logs and session details. |
| Shortcuts | Read only | The active Preview keyboard shortcuts are listed; shortcut customization is not implemented. |
| Permissions | Not in settings | No Preview permission-policy editor or verified mutation contract. |
| Sandbox | Not in settings | No Preview sandbox settings view or verified mutation contract. |
| Network | Not in settings | No Preview network settings view or verified mutation contract. |
| Appearance | Partial | Theme mode/style, conversation width, text size, UI font, and code font persist locally. Remaining stable appearance controls are not present. |
| Storage | Partial | Preview profile location, backup-before-import, project-folder import, and unclaimed session review are available in the Data tab. Broader stable storage management is not present. |
| Updates | Not in settings | No Preview update settings UI or verified updater flow in this page. |

## Implementation rule

Add a control only with an identified source of truth, a read path, a save or
action path where appropriate, and an observable result or error. Keep unsupported
stable categories out of the Preview navigation until those paths exist. The
Preview profile and host preferences remain separate from the stable user profile.
