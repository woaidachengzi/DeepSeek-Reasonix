import { useCallback, type ReactNode } from "react";
import { MarkdownImageResolverContext } from "../components/MarkdownImageContext";
import { tauriWorkspaceImage } from "../lib/tauriBridge";
import type { MarkdownImageView } from "../lib/markdownImage";

export function TauriImageScope({ sessionId, children }: { sessionId?: string; children: ReactNode }) {
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
  return <MarkdownImageResolverContext.Provider value={resolve}>{children}</MarkdownImageResolverContext.Provider>;
}
