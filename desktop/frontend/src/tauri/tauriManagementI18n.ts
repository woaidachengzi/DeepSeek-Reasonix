import { useEffect, useState } from "react";
import { useI18n, type DictKey, type Locale } from "../lib/i18n";

// Keep E's optional management vocabulary out of the general language chunks.
const english = {
  "settings.bots.addConnection": "Add connection",
  "settings.bots.newConnectionHint": "Add an account with its App ID or Account ID and secret. New connections stay disabled until you enable them.",
  "settings.bots.connectionId": "Connection ID",
  "settings.bots.connectionName": "Display name",
  "settings.bots.connectionPlatform": "Platform",
  "settings.bots.removeConnection": "Remove connection",
  "settings.bots.removeConnectionConfirm": "Remove {id}? Its routes and subscriptions will be removed. An unused app-owned credential will also be cleared.",
  "settings.bots.confirmRemoveConnection": "Confirm removal",
  "settings.bots.restartRuntime": "Restart connections",
  "settings.bots.refreshingRuntime": "Applying configuration…",
  "settings.bots.pairingRequests": "Pairing requests",
  "settings.bots.noPairingRequests": "No pending pairing requests",
  "settings.bots.pairingExpires": "Expires: {time}",
  "settings.bots.pairingApprovalConfirm": "Confirm this applicant. The first trusted user may receive administrator and approval permissions.",
  "settings.bots.confirmPairing": "Confirm approval",
  "settings.bots.approvePairing": "Approve",
  "settings.bots.rejectPairing": "Reject",
  "settings.remote.forwards": "Port forwarding",
  "settings.remote.forwardsHint": "Listen on this Mac only (127.0.0.1). Rules stay in this SSH connection, survive reconnects, and are removed when you disconnect or quit.",
  "settings.remote.forwardID": "Forward ID",
  "settings.remote.localPort": "Local port",
  "settings.remote.remoteHost": "Remote host",
  "settings.remote.remotePort": "Remote port",
  "settings.remote.addForward": "Add forward",
  "settings.remote.forwardActive": "Active",
  "settings.remote.forwardInactive": "Inactive"
} as const;
type ManagementKey = keyof typeof english;
type ManagementDict = Record<ManagementKey, string>;
const dictionaries: Partial<Record<Locale, ManagementDict>> = { en: english };
let loading: Promise<void> | undefined;
export function useManagementT() {
  const { locale, t } = useI18n();
  const [, setVersion] = useState(0);
  useEffect(() => {
    if (dictionaries[locale]) return;
    let active = true;
    loading ??= import("./tauriManagementLocales").then(module => {
      dictionaries.zh = module.chinese;
      dictionaries["zh-TW"] = module.traditionalChinese;
    }).catch(error => { loading = undefined; throw error; });
    void loading.then(() => { if (active) setVersion(value => value + 1); }).catch(() => {});
    return () => { active = false; };
  }, [locale]);
  return (key: DictKey | ManagementKey, vars?: Record<string, string | number>): string => {
    if (!(key in english)) return t(key as DictKey, vars);
    const managed = key as ManagementKey;
    let text = (dictionaries[locale] ?? english)[managed];
    for (const [name, value] of Object.entries(vars ?? {})) text = text.replaceAll(`{${name}}`, String(value));
    return text;
  };
}
