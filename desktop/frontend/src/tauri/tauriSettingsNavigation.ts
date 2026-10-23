import { SlidersHorizontal, Globe, Cable, Mic, ChartNoAxesColumn, Monitor, Bot, Server, Sparkles, Users, Package, Database, Webhook, Activity, ShieldCheck, Box, Palette, Keyboard, RefreshCw, Info } from "lucide-react";
import type { Translator, DictKey } from "../lib/i18n";

export type TauriSettingsTab = "computer_use" | "speech" | "general" | "appearance" | "model" | "providers" | "stats" | "bots" | "mcp" | "remote" | "skills" | "subagents" | "plugins" | "hooks" | "memory" | "permissions" | "sandbox" | "network" | "diagnostics" | "data" | "shortcuts" | "updates" | "about";

export const SETTINGS_GROUPS = (t: Translator) => [
  { label: t("settings.navGroup.preferences"), items: [{ id: "general", label: t("settings.tab.general"), description: t("settings.tabSub.general"), searchTerms: [
    "settings.desktopLayoutStyle", "settings.language", "settings.currency", "settings.sessionExperience",
    "settings.closeBehavior", "settings.defaultToolApprovalMode", "settings.general.notifications",
    "settings.sound", "settings.statusBarStyle", "settings.statusBarItems",
  ].map(key => t(key as DictKey)).join(" "), icon: SlidersHorizontal }] },
  { label: t("settings.tab.models"), items: [
    { id: "model", label: t("settings.models.preferences"), description: t("settings.tabSub.models"), icon: Globe },
    { id: "providers", label: t("settings.models.services"), description: t("settings.tabSub.providers"), icon: Cable },
    { id: "speech", label: "语音识别", description: "MiMo 云端与自定义识别服务", icon: Mic },
    { id: "stats", label: t("settings.modelTab.stats"), description: t("settings.tabSub.models"), icon: ChartNoAxesColumn },
  ] },
  { label: t("settings.navGroup.connections"), items: [{ id: "computer_use", label: t("settings.tab.computerUse"), description: t("settings.tabSub.computerUse"), icon: Monitor }, { id: "bots", label: t("settings.tab.bots"), description: t("settings.tabSub.bots"), icon: Bot }, { id: "mcp", label: t("settings.tab.mcp"), description: t("settings.tabSub.mcp"), icon: Server }, { id: "remote", label: t("settings.tab.remote"), description: t("settings.tabSub.remote"), icon: Cable }] },
  { label: t("settings.navGroup.capabilities"), items: [{ id: "skills", label: t("settings.tab.skills"), description: t("settings.tabSub.skills"), icon: Sparkles }, { id: "subagents", label: t("settings.tab.subagents"), description: t("subagents.tabHint"), icon: Users }, { id: "plugins", label: t("settings.tab.plugins"), description: t("settings.tabSub.plugins"), icon: Package }] },
  { label: t("settings.navGroup.context"), items: [{ id: "memory", label: t("settings.tab.memory"), description: t("settings.tabSub.memory"), icon: Database }] },
  { label: t("settings.navGroup.automation"), items: [{ id: "hooks", label: t("settings.tab.hooks"), description: t("settings.tabSub.hooks"), icon: Webhook }, { id: "diagnostics", label: t("settings.tab.diagnostics"), description: t("settings.tabSub.diagnostics"), icon: Activity }] },
  { label: t("settings.navGroup.security"), items: [
    { id: "permissions", label: t("settings.tab.permissions"), description: t("settings.tabSub.permissions"), icon: ShieldCheck },
    { id: "sandbox", label: t("settings.tab.sandbox"), description: t("settings.tabSub.sandbox"), icon: Box },
    { id: "network", label: t("settings.tab.network"), description: t("settings.tabSub.network"), icon: Globe },
  ] },
  { label: t("settings.navGroup.application"), items: [
    { id: "appearance", label: t("settings.tab.appearance"), description: t("settings.tabSub.appearance"), icon: Palette },
    { id: "shortcuts", label: t("settings.tab.shortcuts"), description: t("settings.tabSub.shortcuts"), icon: Keyboard },
    { id: "data", label: t("settings.tab.storage"), description: t("settings.tabSub.storage"), icon: Database },
    { id: "updates", label: t("settings.tab.updates"), description: t("settings.tabSub.updates"), icon: RefreshCw },
    { id: "about", label: t("settings.about.navLabel"), description: t("settings.about.hint"), icon: Info },
  ] },
] as const;

export const SETTINGS_TITLES = (t: Translator): Record<TauriSettingsTab, { title: string; description: string }> => ({
  computer_use: { title: t("settings.tab.computerUse"), description: t("settings.tabSub.computerUse") },
  speech: { title: "语音识别设置", description: "独立配置语音服务，录音转文字后由你确认发送" },
  general: { title: t("settings.tab.general"), description: t("settings.tabSub.general") },
  model: { title: t("settings.models.preferences"), description: t("settings.tabSub.models") },
  providers: { title: t("settings.models.services"), description: t("settings.tabSub.providers") },
  stats: { title: t("settings.modelTab.stats"), description: t("settings.tabSub.models") },
  bots: { title: t("settings.tab.bots"), description: t("settings.pageDesc.bots") },
  mcp: { title: t("settings.tab.mcp"), description: t("settings.tabSub.mcp") },
  remote: { title: t("settings.tab.remote"), description: t("settings.tabSub.remote") },
  skills: { title: t("settings.tab.skills"), description: t("settings.tabSub.skills") },
  plugins: { title: t("settings.tab.plugins"), description: t("settings.tabSub.plugins") },
  subagents: { title: t("settings.tab.subagents"), description: t("subagents.tabHint") },
  hooks: { title: t("settings.tab.hooks"), description: t("settings.tabSub.hooks") },
  memory: { title: t("settings.tab.memory"), description: t("settings.tabSub.memory") },
  permissions: { title: t("settings.tab.permissions"), description: t("settings.tabSub.permissions") },
  sandbox: { title: t("settings.tab.sandbox"), description: t("settings.tabSub.sandbox") },
  network: { title: t("settings.tab.network"), description: t("settings.tabSub.network") },
  diagnostics: { title: t("settings.tab.diagnostics"), description: t("settings.tabSub.diagnostics") },
  appearance: { title: t("settings.tab.appearance"), description: t("settings.tabSub.appearance") },
  shortcuts: { title: t("settings.tab.shortcuts"), description: t("settings.tabSub.shortcuts") },
  data: { title: t("settings.tab.storage"), description: t("settings.tabSub.storage") },
  updates: { title: t("settings.tab.updates"), description: t("settings.tabSub.updates") },
  about: { title: t("settings.about.title"), description: t("settings.about.hint") },
});
