import { useCallback, useEffect, useRef, useState } from "react";
import { deleteTauriMCPServer, saveTauriMCPServer, tauriMCPServers, tauriMessageFrom, type TauriMCPServer } from "../lib/tauriBridge";
import { emptyMCPDraft, mcpCredentialHint, mcpDraftForEditing, mcpDraftToInput, mcpTransportSummary, type MCPDraft } from "./tauriMCPServers";

export function TauriMCPSettings({ workspaceRoot }: { workspaceRoot?: string }) {
  const [servers, setServers] = useState<TauriMCPServer[]>([]);
  const [notice, setNotice] = useState("");
  const [draft, setDraft] = useState<MCPDraft | null>(null);
  const [busy, setBusy] = useState(false);
  const [loading, setLoading] = useState(true);
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

  return <section className="tauri-mcp-settings" aria-label="MCP 服务器设置">
    <div className="tauri-mcp-settings__heading">
      <div><h3>MCP 服务器</h3><p>项目级写入工作区的 <code>reasonix.toml</code>，全局级写入用户配置。密钥只写不回传。</p></div>
      <div className="tauri-mcp-settings__actions">
        <button type="button" className="tauri-settings-button" onClick={() => void refresh()} disabled={busy || loading}>刷新</button>
        <button type="button" className="tauri-settings-button" onClick={() => setDraft(emptyMCPDraft(workspaceRoot ? "project" : "global"))} disabled={busy || draft !== null}>添加</button>
      </div>
    </div>
    {notice && <p className="tauri-diagnostic-error" role="alert">{notice}</p>}
    {loading ? <p className="tauri-settings-loading">正在读取 MCP 服务器…</p> : servers.length === 0 ? <p className="tauri-settings-empty">还没有配置 MCP 服务器。</p> : <ul className="tauri-mcp-list">{servers.map(server => <li key={`${server.scope}:${server.name}`}>
      <div className="tauri-mcp-list__row"><strong>{server.name}</strong><span className="tauri-mcp-list__meta">{server.scope === "project" ? "项目" : server.scope === "global" ? "全局" : server.scope} · {server.type}</span></div>
      <code className="tauri-mcp-list__transport">{mcpTransportSummary(server) || "—"}</code>
      {mcpCredentialHint(server) && <small>{mcpCredentialHint(server)}</small>}
      {server.managedByPackage && <small>由已安装插件包管理，不能在此修改。</small>}
      <div className="tauri-mcp-list__actions"><button type="button" onClick={() => setDraft(mcpDraftForEditing(server))} disabled={busy || server.managedByPackage}>编辑</button><button type="button" onClick={() => void remove(server)} disabled={busy || server.managedByPackage}>删除</button></div>
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
