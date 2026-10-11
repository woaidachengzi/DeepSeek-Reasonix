import { AlertCircle, Pin, X } from "lucide-react";
import type { TauriPinnedFile } from "../lib/tauriBridge";

export function TauriPinnedFilesShelf({ files, error, disabled, english, onRemove, onPreview, onRetry }: {
  files: TauriPinnedFile[]; error: string; disabled: boolean; english: boolean;
  onRemove: (path: string) => void; onPreview: (path: string) => void; onRetry: () => void;
}) {
  if (!files.length && !error) return null;
  return <section className="tauri-pinned-files" aria-label={english ? "Pinned context files" : "固定上下文文件"}>
    <div className="tauri-pinned-files__label"><Pin size={13} />{english ? "Pinned files · read again each turn" : "固定文件 · 每轮读取最新内容"}</div>
    {error && <div className="tauri-pinned-files__error" role="alert"><span>{english ? "Pinned files could not be updated. Retry loading the list. " : "固定文件读取或修改失败；请重试读取清单。 "}{error}</span><button type="button" onClick={onRetry}>{english ? "Retry" : "重试"}</button></div>}
    <ul>{files.map(file => <li key={file.path} className={file.error ? "is-unavailable" : undefined}>
      <button type="button" className="tauri-pinned-files__preview" onClick={() => onPreview(file.path)} title={file.error ? `${file.path}\n${file.error}` : `${file.path} · ${file.sizeBytes} bytes`}>
        {file.error ? <AlertCircle size={13} /> : <Pin size={13} />}<span>{file.path}</span><small>{file.error ? (english ? "Unavailable" : "文件失效") : `~${file.tokenEstimate} tokens`}</small>
      </button>
      <button type="button" className="tauri-pinned-files__remove" onClick={() => onRemove(file.path)} disabled={disabled} aria-label={`${english ? "Unpin" : "取消固定"} ${file.path}`} title={english ? "Unpin file" : "取消固定文件"}><X size={13} /></button>
    </li>)}</ul>
  </section>;
}
