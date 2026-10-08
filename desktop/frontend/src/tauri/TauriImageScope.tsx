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
    // All images use the owned bridge; no direct WebView network fallback can
    // bypass the application's proxy or public-address/decode budgets.
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
