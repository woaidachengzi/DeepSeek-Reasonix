import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { app } from "./bridge";
import type { MarkdownImageView } from "./markdownImage";

type Resolver = ((source: string) => Promise<MarkdownImageView>) | null;
const EMPTY: Record<string,string> = {};

// A scoped resolver is authoritative: refusal never falls back to a local
// AttachmentDataURL grant. Cache and late results belong to this exact owner.
export function useAttachmentImageScope(resolver: Resolver, identity: string, paths: string) {
  const owner = useMemo(() => ({urls:new Map<string,string>(),pending:new Map<string,Promise<string>>()}),[resolver,identity]);
  const current = useRef<typeof owner | null>(null);
  const [cache,setCache] = useState<{owner:typeof owner;urls:Record<string,string>} | null>(null);
  useLayoutEffect(() => {
    current.current = owner;
    return () => { if (current.current === owner) current.current = null; };
  },[owner]);
  const load = useCallback(async (path: string) => {
    if (current.current !== owner) return undefined;
    const cached = owner.urls.get(path);
    if (cached) return cached;
    let pending = owner.pending.get(path);
    if (!pending) {
      pending = Promise.resolve().then(() => {
        if (current.current !== owner) return "";
        return resolver ? resolver(path).then(view => view.errorCode ? "" : view.url) : app.AttachmentDataURL(path);
      }).catch(() => "");
      owner.pending.set(path,pending);
    }
    const url = await pending;
    if (owner.pending.get(path) === pending) owner.pending.delete(path);
    if (current.current !== owner || !url) return undefined;
    owner.urls.set(path,url);
    setCache(previous => previous?.owner === owner && previous.urls[path] === url ? previous : {owner,urls:Object.fromEntries(owner.urls)});
    return url;
  },[owner,resolver]);
  useEffect(() => {
    for (const path of paths ? paths.split("\n") : []) void load(path);
  },[load,paths]);
  return {owner,load,urls:cache?.owner === owner ? cache.urls : EMPTY};
}
