import { useEffect, useState } from "react";
import { app, onDesktopShellStatus, type DesktopShellStatusView } from "../lib/bridge";

export function DesktopCloseBehaviorHint({
  backgroundSelected,
  hint,
  unavailableHint,
}: {
  backgroundSelected: boolean;
  hint: string;
  unavailableHint: string;
}) {
  const [status, setStatus] = useState<DesktopShellStatusView | null>(null);
  useEffect(() => {
    let active = true;
    const update = (value: unknown) => {
      if (!active || !value || typeof value !== "object") return false;
      const candidate = value as Partial<DesktopShellStatusView>;
      setStatus({
        trayState: candidate.trayState === "ready" || candidate.trayState === "unavailable" ? candidate.trayState : "probing",
        backgroundCloseAvailable: candidate.backgroundCloseAvailable === true,
        reason: typeof candidate.reason === "string" ? candidate.reason : undefined,
      });
      return true;
    };
    let eventRevision = 0;
    const off = onDesktopShellStatus((value) => {
      if (update(value)) eventRevision++;
    });
    const snapshotRevision = eventRevision;
    const getStatus = app.GetDesktopShellStatus;
    if (typeof getStatus === "function") void getStatus.call(app).then((value) => {
      if (eventRevision === snapshotRevision) update(value);
    }).catch(() => undefined);
    return () => {
      active = false;
      off();
    };
  }, []);
  if (!backgroundSelected || !status || status.backgroundCloseAvailable) return hint;
  return <>{hint}<br />{unavailableHint}</>;
}
