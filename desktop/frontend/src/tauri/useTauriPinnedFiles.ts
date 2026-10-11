import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
import { tauriChangePinnedFile, tauriMessageFrom, tauriPinnedFiles, type TauriPinnedFile } from "../lib/tauriBridge";

type Options = { sessionId?: string; ready: boolean; idle: boolean; readOnly: boolean; busy: boolean };
type Snapshot = { owner: string; files: TauriPinnedFile[]; loading: boolean; busy: boolean; error: string };

export function useTauriPinnedFiles(options: Options) {
  const owner = options.sessionId && options.ready && !options.readOnly ? options.sessionId : "";
  const current = useRef(options);
  const generation = useRef(0);
  const request = useRef(0);
  const mutation = useRef<{ generation: number } | null>(null);
  const loaded = useRef<{ owner: string; generation: number } | null>(null);
  const [snapshot, setSnapshot] = useState<Snapshot>({ owner: "", files: [], loading: false, busy: false, error: "" });
  const [refresh, setRefresh] = useState(0);

  useLayoutEffect(() => { current.current = options; });
  useLayoutEffect(() => {
    generation.current++;
    mutation.current = null;
    loaded.current = null;
    return () => { generation.current++; };
  }, [owner]);

  useEffect(() => {
    if (!owner) return;
    const epoch = generation.current;
    const ticket = ++request.current;
    loaded.current = null;
    const valid = () => generation.current === epoch && request.current === ticket;
    setSnapshot(previous => ({ owner, files: previous.owner === owner ? previous.files : [], loading: true, busy: false, error: "" }));
    void tauriPinnedFiles(owner).then(files => {
      if (valid()) {
        loaded.current = { owner, generation: epoch };
        setSnapshot({ owner, files, loading: false, busy: false, error: "" });
      }
    }).catch(cause => {
      if (valid()) setSnapshot(previous => ({ ...previous, loading: false, error: tauriMessageFrom(cause) }));
    });
    return () => { if (request.current === ticket) request.current++; };
  }, [owner, refresh]);

  const change = useCallback(async (path: string, pin: boolean) => {
    const active = current.current;
    if (!active.sessionId || !active.ready || !active.idle || active.readOnly || active.busy || mutation.current || loaded.current?.owner !== active.sessionId || loaded.current.generation !== generation.current) return;
    const identity = { generation: generation.current };
    const sessionId = active.sessionId;
    mutation.current = identity;
    request.current++;
    const valid = () => generation.current === identity.generation && mutation.current === identity;
    setSnapshot(previous => ({ ...previous, owner: sessionId, loading: false, busy: true, error: "" }));
    try {
      const files = await tauriChangePinnedFile(sessionId, path, pin);
      if (valid()) setSnapshot({ owner: sessionId, files, loading: false, busy: false, error: "" });
    } catch (cause) {
      if (valid()) setSnapshot(previous => ({ ...previous, busy: false, error: tauriMessageFrom(cause) }));
    } finally {
      if (mutation.current === identity) mutation.current = null;
    }
  }, []);

  const visible = snapshot.owner === owner ? snapshot : null;
  return {
    files: visible?.files ?? [],
    error: visible?.error ?? "",
    loading: Boolean(owner && (!visible || visible.loading)),
    busy: visible?.busy ?? false,
    disabled: !owner || !options.idle || options.busy || Boolean(visible?.busy || visible?.loading || visible?.error || !visible),
    change,
    retry: () => { if (!mutation.current) setRefresh(value => value + 1); },
  };
}
