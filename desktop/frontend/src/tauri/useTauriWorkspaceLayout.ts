import { useEffect, useLayoutEffect, useRef, useState, type CSSProperties, type KeyboardEvent, type PointerEvent } from "react";
import { createPointerResizeLifecycle, createRafResizeUpdater, type PointerResizeLifecycle } from "../lib/resizeDrag";
import { availableWorkspacePanelWidth } from "../lib/workspaceLayout";

const WIDTH_KEY = "reasonix.tauri.workspace-width.v1";
const DEFAULT_WIDTH = 520;
const MIN_WIDTH = 320;
const MAX_WIDTH = 960;

function readWidth() {
  try {
    const value = Number(localStorage.getItem(WIDTH_KEY));
    if (Number.isFinite(value) && value >= MIN_WIDTH && value <= MAX_WIDTH) return value;
  } catch { /* Storage may be unavailable in a private WebView. */ }
  return DEFAULT_WIDTH;
}

export function useTauriWorkspaceLayout(open: boolean, sidebarVisible: boolean, desktopLayout: string, onLayoutChange?: () => void) {
  const shellRef = useRef<HTMLElement>(null);
  const [preferredWidth, setPreferredWidth] = useState(readWidth);
  const [capacity, setCapacity] = useState(0);
  const drag = useRef<PointerResizeLifecycle | null>(null);
  const mounted = useRef(true);
  const liveWidth = useRef(preferredWidth);
  const docked = open && capacity >= MIN_WIDTH;
  const maxWidth = Math.max(MIN_WIDTH, Math.min(MAX_WIDTH, capacity));
  const width = Math.min(drag.current ? liveWidth.current : preferredWidth, maxWidth);
  const limits = useRef({ docked, maxWidth, width });
  limits.current = { docked, maxWidth, width };
  const layoutChange = useRef(onLayoutChange);
  layoutChange.current = onLayoutChange;
  const previousLayout = useRef({ docked, width });
  useLayoutEffect(() => {
    const previous = previousLayout.current;
    previousLayout.current = { docked, width };
    if ((previous.docked || docked) && (previous.docked !== docked || previous.width !== width)) layoutChange.current?.();
  }, [docked, width]);

  useLayoutEffect(() => {
    const shell = shellRef.current;
    if (!shell) return;
    const sidebar = shell.querySelector<HTMLElement>(".tauri-sidebar");
    const measure = () => {
      const viewportWidth = shell.getBoundingClientRect().width;
      setCapacity(viewportWidth <= 640 ? 0 : availableWorkspacePanelWidth({
        viewportWidth, sidebarCollapsed: !sidebarVisible,
        sidebarWidth: sidebar?.getBoundingClientRect().width ?? 0,
        chatMinWidth: 480, resizerWidth: 0,
      }));
    };
    measure();
    const observer = typeof ResizeObserver === "undefined" ? null : new ResizeObserver(measure);
    observer?.observe(shell);
    if (sidebar) observer?.observe(sidebar);
    window.addEventListener("resize", measure);
    return () => { observer?.disconnect(); window.removeEventListener("resize", measure); };
  }, [sidebarVisible, desktopLayout]);

  useEffect(() => {
    if (!docked) drag.current?.finish();
  }, [docked]);
  useLayoutEffect(() => {
    mounted.current = true;
    return () => { mounted.current = false; drag.current?.finish(); };
  }, []);

  const commit = (value: number) => {
    setPreferredWidth(value);
    try { localStorage.setItem(WIDTH_KEY, String(value)); } catch { /* Keep the in-memory preference. */ }
  };
  const clamp = (value: number) => Math.round(Math.max(MIN_WIDTH, Math.min(limits.current.maxWidth, value)));
  const onPointerDown = (event: PointerEvent<HTMLButtonElement>) => {
    if (event.button !== 0 || !limits.current.docked || drag.current) return;
    event.preventDefault();
    const shell = shellRef.current;
    if (!shell) return;
    const startX = event.clientX, startWidth = limits.current.width;
    liveWidth.current = startWidth;
    shell.dataset.workspaceResizing = "true";
    const updater = createRafResizeUpdater({ target: shell, separator: event.currentTarget, cssVar: "--tauri-workspace-width", onApply: () => layoutChange.current?.() });
    drag.current = createPointerResizeLifecycle({
      separator: event.currentTarget, pointerId: event.pointerId,
      onMove: pointer => { liveWidth.current = clamp(startWidth + startX - pointer.clientX); updater.schedule(liveWidth.current); },
      onFinish: () => {
        if (mounted.current) updater.flush(); else updater.cancel();
        delete shell.dataset.workspaceResizing;
        drag.current = null;
        if (mounted.current) commit(liveWidth.current);
      },
    });
  };
  const onKeyDown = (event: KeyboardEvent<HTMLButtonElement>) => {
    if (!limits.current.docked) return;
    const step = event.shiftKey ? 64 : 24;
    const next = event.key === "ArrowLeft" ? width + step : event.key === "ArrowRight" ? width - step
      : event.key === "Home" ? MIN_WIDTH : event.key === "End" ? maxWidth : null;
    if (next === null) return;
    event.preventDefault();
    commit(clamp(next));
  };
  return {
    shellRef, docked,
    style: { "--tauri-workspace-width": `${docked ? width : preferredWidth}px` } as CSSProperties,
    separatorProps: {
      role: "separator", "aria-orientation": "vertical", "aria-valuemin": MIN_WIDTH,
      "aria-valuemax": maxWidth, "aria-valuenow": width, "aria-controls": "tauri-workspace-panel",
      onPointerDown, onKeyDown, onDoubleClick: () => commit(clamp(DEFAULT_WIDTH)),
    } as const,
  };
}
