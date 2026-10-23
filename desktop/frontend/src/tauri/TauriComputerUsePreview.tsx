import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
import { ChevronDown, ChevronUp, GripHorizontal, Monitor, Square } from "lucide-react";
import { clampPreviewPosition, type ComputerUsePreview } from "./computerUsePreview";
import "./TauriComputerUsePreview.css";

const actionLabels: Record<string, string> = { observe: "查看窗口", activate: "切换窗口", click: "点击", type: "输入文字", key: "按键", scroll: "滚动" };

export function TauriComputerUsePreview({ preview, busy, onStop }: { preview: ComputerUsePreview; busy: boolean; onStop: () => void }) {
  const [collapsed, setCollapsed] = useState(false);
  const [position, setPosition] = useState(() => ({ x: Math.max(8, window.innerWidth - 340), y: 100 }));
  const box = useRef<HTMLElement>(null);
  const drag = useRef<{ id: number; x: number; y: number; left: number; top: number } | null>(null);
  const move = useCallback((x: number, y: number) => {
    const rect = box.current?.getBoundingClientRect();
    if (rect) {
      const next = clampPreviewPosition(x, y, rect.width, rect.height, window.innerWidth, window.innerHeight);
      setPosition(previous => previous.x === next.x && previous.y === next.y ? previous : next);
    }
  }, []);
  useLayoutEffect(() => { move(position.x, position.y); }, [collapsed, move, position.x, position.y]);
  useEffect(() => {
    const resize = () => setPosition(p => {
      const rect = box.current?.getBoundingClientRect();
      return rect ? clampPreviewPosition(p.x, p.y, rect.width, rect.height, window.innerWidth, window.innerHeight) : p;
    });
    window.addEventListener("resize", resize);
    return () => window.removeEventListener("resize", resize);
  }, []);

  return <aside ref={box} className="tauri-computer-preview" aria-label="电脑操作预览" style={{ left: position.x, top: position.y }}>
    <div className="tauri-computer-preview__header">
      <button type="button" className="tauri-computer-preview__drag" aria-label="移动操作预览（方向键可移动）" title="拖动移动，或使用方向键"
        onPointerDown={event => {
          if (event.button !== 0) return;
          drag.current = { id: event.pointerId, x: event.clientX, y: event.clientY, left: position.x, top: position.y };
          event.currentTarget.setPointerCapture(event.pointerId);
        }}
        onPointerMove={event => {
          const d = drag.current;
          if (d?.id === event.pointerId) move(d.left + event.clientX - d.x, d.top + event.clientY - d.y);
        }}
        onPointerUp={() => { drag.current = null; }} onPointerCancel={() => { drag.current = null; }} onLostPointerCapture={() => { drag.current = null; }}
        onKeyDown={event => {
          const delta: Record<string, [number, number]> = { ArrowLeft: [-16, 0], ArrowRight: [16, 0], ArrowUp: [0, -16], ArrowDown: [0, 16] };
          const step = delta[event.key];
          if (step) { event.preventDefault(); move(position.x + step[0], position.y + step[1]); }
        }}><GripHorizontal size={14} aria-hidden="true" /><Monitor size={14} aria-hidden="true" /><span>{preview.appName}</span></button>
      <button type="button" aria-label={collapsed ? "展开操作预览" : "收起操作预览"} aria-expanded={!collapsed} onClick={() => setCollapsed(value => !value)}>{collapsed ? <ChevronDown size={16} /> : <ChevronUp size={16} />}</button>
    </div>
    {!collapsed && <div className="tauri-computer-preview__image"><img src={preview.image} alt={`${preview.appName} 最近一次观察到的窗口`} draggable={false} /></div>}
    <div className="tauri-computer-preview__footer"><span>{actionLabels[preview.action]} · 最近画面</span><button type="button" onClick={onStop} disabled={busy}><Square size={12} aria-hidden="true" />{busy ? "正在停止…" : "停止操作"}</button></div>
  </aside>;
}
