import { useContext, useEffect, useMemo, useState } from "react";
import { app } from "../lib/bridge";
import { hasMarkdownImageResolver, markdownImageSource, type MarkdownImageView } from "../lib/markdownImage";
import { RichMarkdownLink } from "./githubLink";
import { MarkdownImagePreviewContext, MarkdownImageResolverContext, MarkdownImageTabContext } from "./MarkdownImageContext";
import { t } from "../lib/i18n";

export function MarkdownImage({ src, alt, title }: { src?: string; alt?: string; title?: string }) {
  const tabId = useContext(MarkdownImageTabContext);
  const resolver = useContext(MarkdownImageResolverContext);
  const preview = useContext(MarkdownImagePreviewContext);
  const source = src?.trim() ?? "";
  const resolverAvailable = Boolean(resolver) || hasMarkdownImageResolver();
  const [element, setElement] = useState<HTMLElement | null>(null);
  const [nearViewport, setNearViewport] = useState(() => !resolver || typeof IntersectionObserver === "undefined");
  const legacyView = useMemo<MarkdownImageView>(
    () => ({ url: markdownImageSource(source), openHref: /^https?:\/\//i.test(source) ? source : undefined }),
    [source],
  );
  const [result, setResult] = useState<{ source: string; resolver: typeof resolver; tabId: string; view: MarkdownImageView } | null>(null);
  const [failedURL, setFailedURL] = useState<string | null>(null);
  // Never paint a previous session's pixels while the new effect is pending.
  const resolved = result?.source === source && result.resolver === resolver && result.tabId === tabId
    ? result.view : resolverAvailable ? null : legacyView;

  useEffect(() => {
    if (!resolver || typeof IntersectionObserver === "undefined") {
      setNearViewport(true);
      return;
    }
    if (!element || nearViewport) return;
    const observer = new IntersectionObserver(entries => {
      if (entries.some(entry => entry.isIntersecting)) {
        setNearViewport(true);
        observer.disconnect();
      }
    }, { rootMargin: "400px" });
    observer.observe(element);
    return () => observer.disconnect();
  }, [element, nearViewport, resolver]);

  useEffect(() => {
    let live = true;
    if (!nearViewport) return () => { live = false; };
    setFailedURL(null);
    if (!resolver && !hasMarkdownImageResolver()) {
      return () => { live = false; };
    }
    void (resolver ? resolver(source) : app.ResolveMarkdownImageForTab(tabId, source)).then((view) => {
      if (live) setResult({ source, resolver, tabId, view });
    }).catch(() => {
      if (live) setResult({ source, resolver, tabId, view: { url: "", errorCode: "resolve-failed" } });
    });
    return () => { live = false; };
  }, [nearViewport, resolver, source, tabId]);

  const unavailable = Boolean(resolved?.url && failedURL === resolved.url) || Boolean(resolved?.errorCode) || (resolved !== null && !resolved.url);
  if (unavailable) {
    const label = alt?.trim() || resolved?.filename || "Image unavailable";
    return (
      <span ref={setElement} className="md-image-fallback" role="img" aria-label={label} title={resolved?.errorCode || title}>
        <span>{label}</span>
        {resolved?.openHref && <RichMarkdownLink href={resolved.openHref}>Open image</RichMarkdownLink>}
      </span>
    );
  }
  if (!resolved) {
    return <span ref={setElement} className="md-image-placeholder" role="status" aria-label={alt?.trim() || "Loading image"} />;
  }
  const openPreview = () => preview?.({ url: resolved.url, name: alt?.trim() || resolved.filename });
  return (
    <img
      ref={setElement}
      src={resolved.url}
      alt={alt ?? ""}
      title={title ?? (preview ? t("imageViewer.clickToPreview") : undefined)}
      role={preview ? "button" : undefined}
      tabIndex={preview ? 0 : undefined}
      aria-label={preview ? `${t("imageViewer.clickToPreview")}${alt ? `: ${alt}` : ""}` : undefined}
      style={preview ? { cursor: "zoom-in" } : undefined}
      onClick={preview ? event => {
        event.preventDefault();
        event.stopPropagation();
        openPreview();
      } : undefined}
      onKeyDown={preview ? event => {
        if (event.key === "Enter" || event.key === " ") {
          event.preventDefault();
          event.stopPropagation();
          openPreview();
        }
      } : undefined}
      loading="lazy"
      decoding="async"
      referrerPolicy="no-referrer"
      onError={() => setFailedURL(resolved.url)}
    />
  );
}
