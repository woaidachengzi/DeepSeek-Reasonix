import { useCallback, useEffect, useRef, useState } from "react";
import { deleteTauriMCPServer, resolveTauriMCPMarketplace, saveTauriMCPServer, searchTauriMCPMarketplace, setTauriMCPServerEnabled, tauriMCPServers, tauriMessageFrom, type TauriMCPMarketplace, type TauriMCPMarketplaceEntry, type TauriMCPServer } from "../lib/tauriBridge";
import { emptyMCPDraft, mcpCredentialHint, mcpDraftForEditing, mcpDraftFromMarketplace, mcpDraftToInput, mcpTransportSummary, type MCPDraft } from "./tauriMCPServers";

function runtimeSummary(server: TauriMCPServer): string {
  if (server.runtimeStatus === "connected") return server.enabled ? `当前会话已连接 · ${server.toolCount ?? 0} 项工具` : "当前会话仍已连接；应用设置后停用";
  if (server.runtimeStatus === "initializing") return "当前会话正在连接";
  if (server.runtimeStatus === "failed") return "当前会话启动失败";
  return server.enabled ? "新会话按需连接" : "已停用";
}

export function TauriMCPSettings({ workspaceRoot, currentSessionState, currentSessionHasAttachments, onApplyToCurrentSession }: {
  workspaceRoot?: string;
  currentSessionState?: "idle" | "running" | "paused";
  currentSessionHasAttachments?: boolean;
  onApplyToCurrentSession?: () => Promise<boolean>;
}) {
  const [servers, setServers] = useState<TauriMCPServer[]>([]);
  const [notice, setNotice] = useState("");
  const [draft, setDraft] = useState<MCPDraft | null>(null);
  const [busy, setBusy] = useState(false);
  const [loading, setLoading] = useState(true);
  const [pendingApply, setPendingApply] = useState(false);
  const [status, setStatus] = useState("");
  const [marketplaceOpen, setMarketplaceOpen] = useState(false);
  const [marketplaceQuery, setMarketplaceQuery] = useState("");
  const [marketplace, setMarketplace] = useState<TauriMCPMarketplace | null>(null);
  const generation = useRef(0);
  const mutationBusy = useRef(false);

  const refresh = useCallback(async () => {
    const request = ++generation.current;
    setLoading(true);
    try {
      const result = await tauriMCPServers(workspaceRoot);
      if (request === generation.current) {
        setServers(result);
        setNotice("");
      }
    } catch (cause) {
      if (request === generation.current) setNotice(tauriMessageFrom(cause));
    } finally {
      if (request === generation.current) setLoading(false);
    }
  }, [workspaceRoot]);

  useEffect(() => {
    setDraft(null);
    mutationBusy.current = false;
    setBusy(false);
    setServers([]);
    setNotice("");
    setPendingApply(false);
    setStatus("");
    setMarketplaceOpen(false);
    setMarketplace(null);
    void refresh();
    return () => { generation.current += 1; };
  }, [refresh]);

  const save = async () => {
    if (!draft || mutationBusy.current) return;
    let input;
    try {
      input = mcpDraftToInput(draft);
    } catch (cause) {
      setNotice(tauriMessageFrom(cause));
      return;
    }
    const request = generation.current;
    mutationBusy.current = true;
    setBusy(true);
    try {
      const result = await saveTauriMCPServer(input, workspaceRoot);
      if (request === generation.current) {
        setServers(result.servers);
        setDraft(null);
        setNotice("");
        setPendingApply(true);
      }
    } catch (cause) {
      if (request === generation.current) setNotice(tauriMessageFrom(cause));
    } finally {
      if (request === generation.current) {
        mutationBusy.current = false;
        setBusy(false);
      }
    }
  };

  const browseMarketplace = async (query: string) => {
    if (mutationBusy.current) return;
    const request = generation.current;
    mutationBusy.current = true;
    setBusy(true);
    setMarketplaceOpen(true);
    setNotice("");
    try {
      const result = await searchTauriMCPMarketplace(query);
      if (request === generation.current) setMarketplace(result);
    } catch (cause) {
      if (request === generation.current) setNotice(tauriMessageFrom(cause));
    } finally {
      if (request === generation.current) {
        mutationBusy.current = false;
        setBusy(false);
      }
    }
  };

  const prepareMarketplace = async (entry: TauriMCPMarketplaceEntry) => {
    if (mutationBusy.current || marketplace?.cached) return;
    const request = generation.current;
    mutationBusy.current = true;
    setBusy(true);
    setNotice("");
    try {
      const current = await resolveTauriMCPMarketplace(entry.name);
      const next = mcpDraftFromMarketplace(current, servers, workspaceRoot ? "project" : "global");
      if (request === generation.current) {
        setDraft(next);
        setMarketplaceOpen(false);
        setStatus(`已载入 ${current.title || current.name} 的最新配置，请检查命令或 URL 后保存。`);
      }
    } catch (cause) {
      if (request === generation.current) setNotice(tauriMessageFrom(cause));
    } finally {
      if (request === generation.current) {
        mutationBusy.current = false;
        setBusy(false);
      }
    }
  };

  const remove = async (server: TauriMCPServer) => {
    if (mutationBusy.current || !window.confirm(`删除 MCP 服务器“${server.name}”？`)) return;
    const request = generation.current;
    mutationBusy.current = true;
    setBusy(true);
    try {
      const result = await deleteTauriMCPServer(server.name, workspaceRoot);
      if (request === generation.current) {
        setServers(result.servers);
        if (draft?.name === server.name) setDraft(null);
        setNotice("");
        setPendingApply(true);
      }
    } catch (cause) {
      if (request === generation.current) setNotice(tauriMessageFrom(cause));
    } finally {
      if (request === generation.current) {
        mutationBusy.current = false;
        setBusy(false);
      }
    }
  };

  const toggle = async (server: TauriMCPServer) => {
    if (mutationBusy.current) return;
    const request = generation.current;
    mutationBusy.current = true;
    setBusy(true);
    try {
      const result = await setTauriMCPServerEnabled(server.name, !server.enabled, workspaceRoot);
      if (request === generation.current) {
        setServers(result.servers);
        setNotice("");
        setStatus(`${server.name} 已${server.enabled ? "停用" : "启用"}，新会话将使用此设置。`);
        setPendingApply(true);
      }
    } catch (cause) {
      if (request === generation.current) setNotice(tauriMessageFrom(cause));
    } finally {
      if (request === generation.current) {
        mutationBusy.current = false;
        setBusy(false);
      }
    }
  };

  const applyCurrent = async () => {
    if (mutationBusy.current || currentSessionState !== "idle" || currentSessionHasAttachments || !onApplyToCurrentSession) return;
    mutationBusy.current = true;
    setBusy(true);
    try {
      if (await onApplyToCurrentSession()) {
        setPendingApply(false);
        setStatus("当前会话已重新载入 MCP 设置。");
        setNotice("");
      } else setNotice("当前会话未能更新，请在运行诊断中重试。");
    } catch (cause) { setNotice(tauriMessageFrom(cause)); }
    finally { mutationBusy.current = false; setBusy(false); }
  };

  return <section className="tauri-mcp-settings" aria-label="MCP 服务器设置">
    <div className="tauri-mcp-settings__heading">
      <div><h3>MCP 服务器</h3><p>选择哪些服务器供新会话使用；项目级写入工作区的 <code>reasonix.toml</code>，全局级写入用户配置。密钥只写不回传。</p></div>
      <div className="tauri-mcp-settings__actions">
        <button type="button" className="tauri-settings-button" onClick={() => void refresh()} disabled={busy || loading}>刷新</button>
        {marketplaceOpen ? <button type="button" className="tauri-settings-button" onClick={() => setMarketplaceOpen(false)} disabled={busy}>返回服务器</button> : <><button type="button" className="tauri-settings-button" onClick={() => void browseMarketplace("")} disabled={busy || draft !== null}>浏览目录</button><button type="button" className="tauri-settings-button" onClick={() => setDraft(emptyMCPDraft(workspaceRoot ? "project" : "global"))} disabled={busy || draft !== null}>添加</button></>}
        {pendingApply && currentSessionState && onApplyToCurrentSession && <button type="button" className="tauri-settings-button" onClick={() => void applyCurrent()} disabled={busy || currentSessionState !== "idle" || currentSessionHasAttachments}>应用到当前会话</button>}
      </div>
    </div>
    {notice && <p className="tauri-diagnostic-error" role="alert">{notice}</p>}
    {status && <p className="tauri-settings-hint" role="status">{status}</p>}
    {marketplaceOpen ? <div className="tauri-mcp-marketplace">
      <h4>官方 MCP 目录</h4><p>从目录查找可直接配置的服务器。选择后先检查配置，再保存到 Preview。</p>
      <form className="tauri-mcp-marketplace__search" onSubmit={event => { event.preventDefault(); void browseMarketplace(marketplaceQuery); }}><input type="search" aria-label="搜索 MCP 目录" placeholder="搜索服务器名称或用途" value={marketplaceQuery} onChange={event => setMarketplaceQuery(event.target.value)} /><button type="submit" disabled={busy}>搜索</button></form>
      {marketplace?.cached && <p className="tauri-settings-hint" role="status">当前显示离线缓存；恢复连接后可重新搜索并配置。</p>}
      {busy && !marketplace ? <p className="tauri-settings-loading">正在查询目录…</p> : marketplace && marketplace.servers.length === 0 ? <p className="tauri-settings-empty">没有匹配的服务器。</p> : marketplace && <ul className="tauri-mcp-marketplace__list">{marketplace.servers.map(entry => <li key={entry.name}><div><strong>{entry.title || entry.name}</strong><small>{entry.name}{entry.version ? ` · ${entry.version}` : ""}{entry.transport ? ` · ${entry.transport}` : ""}</small>{entry.description && <p>{entry.description}</p>}{!entry.installable && <small>{entry.unavailableReason || "需要手动配置"}</small>}</div>{entry.installable && <button type="button" onClick={() => void prepareMarketplace(entry)} disabled={busy || marketplace.cached}>配置</button>}</li>)}</ul>}
    </div> : loading ? <p className="tauri-settings-loading">正在读取 MCP 服务器…</p> : servers.length === 0 ? <p className="tauri-settings-empty">还没有配置 MCP 服务器。</p> : <ul className="tauri-mcp-list">{servers.map(server => <li key={`${server.scope}:${server.name}`}>
      <div className="tauri-mcp-list__row"><strong>{server.name}</strong><span className="tauri-mcp-list__meta">{server.scope === "project" ? "项目" : server.scope === "global" ? "全局" : server.scope} · {server.type}</span></div>
      <small className={`tauri-mcp-list__runtime${server.runtimeStatus === "failed" ? " is-error" : ""}`}>{runtimeSummary(server)}{server.errorKind ? ` · ${server.errorKind}` : ""}</small>
      <code className="tauri-mcp-list__transport">{mcpTransportSummary(server) || "—"}</code>
      {server.runtimeStatus === "connected" && (server.toolList?.length ?? 0) > 0 && <details className="tauri-mcp-list__tools"><summary>查看工具列表</summary><ul>{server.toolList?.map(tool => <li key={tool.name}><strong>{tool.name}</strong>{tool.description && <span>{tool.description}</span>}</li>)}</ul></details>}
      {mcpCredentialHint(server) && <small>{mcpCredentialHint(server)}</small>}
      {server.managedByPackage && <small>由已安装插件包管理，不能在此修改。</small>}
      <div className="tauri-mcp-list__actions"><button type="button" className="tauri-mcp-list__toggle" role="switch" aria-checked={server.enabled} aria-label={`${server.name} ${server.enabled ? "已启用" : "已停用"}`} onClick={() => void toggle(server)} disabled={busy}>{server.enabled ? "已启用" : "已停用"}</button><button type="button" onClick={() => setDraft(mcpDraftForEditing(server))} disabled={busy || server.managedByPackage}>编辑</button><button type="button" onClick={() => void remove(server)} disabled={busy || server.managedByPackage}>删除</button></div>
    </li>)}</ul>}
    {draft && <form className="tauri-mcp-form" onSubmit={event => { event.preventDefault(); void save(); }}>
      <label>名称<input value={draft.name} onChange={event => setDraft({ ...draft, name: event.target.value })} disabled={draft.editing || busy} aria-label="MCP 服务器名称" /></label>
      <label>作用域<select value={draft.scope} onChange={event => setDraft({ ...draft, scope: event.target.value === "project" ? "project" : "global" })} disabled={draft.editing || busy}><option value="project" disabled={!workspaceRoot}>项目（写入工作区 reasonix.toml）</option><option value="global">全局（写入用户配置）</option></select></label>
      <label>传输<select value={draft.type} onChange={event => setDraft({ ...draft, type: event.target.value === "http" ? "http" : event.target.value === "sse" ? "sse" : "stdio" })} disabled={busy}><option value="stdio">stdio</option><option value="http">http</option><option value="sse">sse</option></select></label>
      {draft.type === "stdio" ? <><label>命令<input value={draft.command} onChange={event => setDraft({ ...draft, command: event.target.value })} placeholder="uvx" aria-label="MCP 服务器命令" disabled={busy} /></label><label>参数<input value={draft.args} onChange={event => setDraft({ ...draft, args: event.target.value })} placeholder="mcp-server-time" aria-label="MCP 服务器参数" disabled={busy} /></label></> : <label>URL<input value={draft.url} onChange={event => setDraft({ ...draft, url: event.target.value })} placeholder="https://…" aria-label="MCP 服务器 URL" disabled={busy} /></label>}
      <label>环境变量<textarea value={draft.env} onChange={event => setDraft({ ...draft, env: event.target.value })} rows={2} placeholder="KEY=value，每行一个；留空保持原值" aria-label="MCP 服务器环境变量" disabled={busy} /></label>
      <label>请求头<textarea value={draft.headers} onChange={event => setDraft({ ...draft, headers: event.target.value })} rows={2} placeholder="Header=value，每行一个；留空保持原值" aria-label="MCP 服务器请求头" disabled={busy} /></label>
      <div className="tauri-mcp-form__actions"><button type="submit" disabled={busy}>{draft.editing ? "保存修改" : "添加服务器"}</button><button type="button" onClick={() => setDraft(null)} disabled={busy}>取消</button></div>
    </form>}
  </section>;
}
