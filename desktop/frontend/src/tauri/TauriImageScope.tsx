import { lazy, Suspense, useCallback, useEffect, useState, type ReactNode } from "react";
import { MarkdownImagePreviewContext, MarkdownImageResolverContext, type MarkdownImagePreview } from "../components/MarkdownImageContext";
import { tauriWorkspaceImage } from "../lib/tauriBridge";
import type { MarkdownImageView } from "../lib/markdownImage";

const ImageViewer = lazy(() => import("../components/ImageViewer").then(module => ({ default: module.ImageViewer })));

export function TauriImageScope({ sessionId, children }: { sessionId?: string; children: ReactNode }) {
  const [preview, setPreview] = useState<(MarkdownImagePreview & { sessionId: string }) | null>(null);
  const openPreview = useCallback((image: MarkdownImagePreview) => {
    if (sessionId) setPreview({ ...image, sessionId });
  }, [sessionId]);
  const closePreview = useCallback(() => setPreview(null), []);
  useEffect(() => { setPreview(null); }, [sessionId]);
  // Hide the previous session's image in the same render as the session switch.
  const activePreview = preview?.sessionId === sessionId ? preview : null;
  const resolve = useCallback(async (source: string): Promise<MarkdownImageView> => {
    // Preserve external raster URLs; local paths are never assigned to img.src.
    // Remote image proxying is a separate gate, not a filesystem capability.
    if (/^(https?:\/\/|\/\/)/i.test(source)) {
      const url = source.startsWith("//") ? `https:${source}` : source;
      return { url, openHref: url };
    }
    if (!sessionId) return { url: "", errorCode: "no-session" };
    return tauriWorkspaceImage(sessionId, source);
  }, [sessionId]);
  return <MarkdownImageResolverContext.Provider value={resolve}>
    <MarkdownImagePreviewContext.Provider value={openPreview}>
      {children}
      {activePreview && <Suspense fallback={null}><ImageViewer open imageUrl={activePreview.url} imageName={activePreview.name} onClose={closePreview} /></Suspense>}
    </MarkdownImagePreviewContext.Provider>
  </MarkdownImageResolverContext.Provider>;
}
