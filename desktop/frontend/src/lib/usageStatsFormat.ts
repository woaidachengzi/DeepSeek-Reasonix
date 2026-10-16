import { getLocale, type Locale } from "./i18n";

// formatUsageTokens follows the desktop language instead of leaking Chinese
// units into English UI. The explicit locale argument keeps the helper easy to
// verify while the default remains reactive through LocaleProvider rerenders.
export function formatUsageTokens(n: number, locale: Locale = getLocale()): string {
  const languageTag = locale === "zh" ? "zh-CN" : locale;
  return new Intl.NumberFormat(languageTag, {
    notation: "compact",
    maximumFractionDigits: 1,
  }).format(n);
}

// Imported service IDs carry a generated 32-hex suffix. Keep accounting keys
// intact and resolve only visible labels; historical removed services may no
// longer have display-name metadata.
export function formatUsageProvider(provider: string, names: Readonly<Record<string, string>> = {}): string {
  const displayName = Object.prototype.hasOwnProperty.call(names, provider) ? names[provider]?.trim() : "";
  return (displayName && displayName !== provider) ? displayName : provider.replace(/-[a-f0-9]{32}$/i, "");
}

export function formatUsageModel(ref: string, names: Readonly<Record<string, string>> = {}): string {
  const slash = ref.indexOf("/");
  return slash < 0 ? ref : `${formatUsageProvider(ref.slice(0, slash), names)}/${ref.slice(slash + 1)}`;
}
