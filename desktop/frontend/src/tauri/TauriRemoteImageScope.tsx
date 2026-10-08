import { lazy, Suspense, useCallback, useLayoutEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { MarkdownImagePreviewContext, MarkdownImageResolverContext, type MarkdownImagePreview } from "../components/MarkdownImageContext";
import type { RemoteControllerLease } from "../lib/remoteControllerPool";

const ImageViewer = lazy(() => import("../components/ImageViewer").then(module => ({ default: module.ImageViewer })));
const unavailable = () => ({ url: "", errorCode: "remote-preview-unavailable" });

// Display-only authority. Never use a local session, opener, or direct URL as
// fallback. Scope identity also fences A -> B -> A replacement and old clicks.
export function TauriRemoteImageScope({ lease, sessionPath, surface, children }: {
  lease: RemoteControllerLease; sessionPath: string; surface: string; children: ReactNode;
}) {
  const scope = useMemo(() => ({}), [lease, sessionPath, surface]);
  const owner = useRef<object | null>(null);
  const [preview, setPreview] = useState<(MarkdownImagePreview & { scope: object }) | null>(null);
  useLayoutEffect(() => {
    owner.current = scope;
    setPreview(null); // Release prior pixels, not merely hide their old scope.
    return () => { if (owner.current === scope) owner.current = null; };
  }, [scope]);
  const resolve = useCallback(async (source: string) => {
    if (owner.current !== scope) return unavailable();
    try {
      const image = await lease.sessionImage(sessionPath, source);
      return owner.current === scope ? image : unavailable();
    } catch { return unavailable(); }
  }, [lease, sessionPath, scope]);
  const openPreview = useCallback((image: MarkdownImagePreview) => {
    if (owner.current === scope && image.url.length <= 11184900 && /^data:image\/png;base64,[A-Za-z0-9+/]+={0,2}$/.test(image.url)) {
      setPreview({ ...image, scope });
    }
  }, [scope]);
  const closePreview = useCallback(() => setPreview(null), []);
  const active = preview?.scope === scope ? preview : null;
  return <MarkdownImageResolverContext.Provider value={resolve}>
    <MarkdownImagePreviewContext.Provider value={openPreview}>
      {children}
      {active ? <Suspense fallback={null}><ImageViewer open imageUrl={active.url} imageName={active.name} onClose={closePreview} /></Suspense> : null}
    </MarkdownImagePreviewContext.Provider>
  </MarkdownImageResolverContext.Provider>;
}
