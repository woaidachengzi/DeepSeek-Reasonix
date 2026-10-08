// Workspace navigation owns this drawer's independent scroller. It never
// scrolls the transcript or an ancestor through scrollIntoView.
export function restoreWorkspaceChangesPosition(body: HTMLDivElement, top: number) {
  body.scrollTo({ top, behavior: "instant" });
  const selected = body.querySelector<HTMLButtonElement>('.tauri-workspace-change-entry[aria-current="true"]');
  if (!selected) return;
  const viewport = body.getBoundingClientRect();
  const row = selected.getBoundingClientRect();
  // If the window resized while reading, keep the chosen file visible.
  const delta = row.bottom <= viewport.top ? row.top - viewport.top
    : row.top >= viewport.bottom ? row.bottom - viewport.bottom : 0;
  if (delta) body.scrollBy({ top: delta, behavior: "instant" });
  selected.focus({ preventScroll: true });
}
