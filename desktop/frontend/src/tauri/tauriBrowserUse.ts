import type { TauriMCPServer } from "../lib/tauriBridge";
import { emptyMCPDraft, type MCPDraft } from "./tauriMCPServers";

// Pin the reviewed server release; upgrading it also requires the live MCP test.
export const BROWSER_MCP_PACKAGE = "@playwright/mcp@0.0.83";
export const BROWSER_MCP_ARGS = ["-y", BROWSER_MCP_PACKAGE, "--browser", "chrome", "--isolated", "--caps", "vision", "--no-webmcp"];

export function findBrowserUseServer(servers: TauriMCPServer[]): TauriMCPServer | undefined {
  return servers.find(server => server.type === "stdio" && server.args?.includes(BROWSER_MCP_PACKAGE));
}

export function browserUseDraft(servers: TauriMCPServer[], scope: MCPDraft["scope"]): MCPDraft {
  const used = new Set(servers.map(server => server.name));
  const base = "reasonix-browser";
  let name = base;
  for (let suffix = 2; used.has(name); suffix += 1) name = `${base}-${suffix}`;
  return {
    ...emptyMCPDraft(scope),
    name,
    command: "npx",
    args: JSON.stringify(BROWSER_MCP_ARGS),
    startupTimeoutSeconds: 120,
    callTimeoutSeconds: 90,
  };
}

export const browserUseCopy = {
  zh: {
    title: "浏览器操作（试用）",
    description: "让 Agent 打开网页、点击、输入、滚动和截图。基于 Microsoft Playwright MCP，每个会话使用独立浏览器环境。",
    requirements: "需要 Node.js 24+ 和 Google Chrome。首次连接会下载工具；不使用日常浏览器的登录状态，关闭会话后登录状态不保留。",
    configure: "配置浏览器操作",
    manage: "查看浏览器配置",
    steps: "保存配置后，新建会话，或在空闲会话中点击「应用到当前会话」。随后发送：",
    example: "/browser-use 打开 https://example.com，查看页面内容并截图",
    vision: "网页结构操作可使用支持工具调用的模型；理解截图需要视觉模型或已配置的图片识别服务。",
  },
  "zh-TW": {
    title: "瀏覽器操作（試用）",
    description: "讓 Agent 開啟網頁、點擊、輸入、捲動和截圖。基於 Microsoft Playwright MCP，每個對話使用獨立瀏覽器環境。",
    requirements: "需要 Node.js 24+ 和 Google Chrome。首次連線會下載工具；不使用日常瀏覽器的登入狀態，關閉對話後登入狀態不保留。",
    configure: "設定瀏覽器操作",
    manage: "檢視瀏覽器設定",
    steps: "儲存設定後，建立新對話，或在閒置對話中點擊「套用至目前對話」。接著傳送：",
    example: "/browser-use 開啟 https://example.com，查看網頁內容並截圖",
    vision: "網頁結構操作可使用支援工具呼叫的模型；理解截圖需要視覺模型或已設定的圖片辨識服務。",
  },
  en: {
    title: "Browser use (preview)",
    description: "Let the agent open pages, click, type, scroll, and take screenshots. Uses Microsoft Playwright MCP with an isolated browser environment per session.",
    requirements: "Requires Node.js 24+ and Google Chrome. The first connection downloads the tools. Your everyday browser logins are separate; logins are discarded when the session closes.",
    configure: "Configure browser use",
    manage: "View browser configuration",
    steps: "Save, then start a new session or choose Apply to current session while idle. Send:",
    example: "/browser-use Open https://example.com, inspect the page, and take a screenshot",
    vision: "Page structure actions work with tool-capable models. Understanding screenshots requires a vision model or a configured image recognition service.",
  },
};
