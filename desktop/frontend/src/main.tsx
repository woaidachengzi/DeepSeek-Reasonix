import "./lib/compat";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { ErrorBoundary } from "./components/ErrorBoundary";
import { installPerformancePressureMonitor } from "./lib/crash";
import { installGlobalCrashHandlers } from "./lib/globalCrashHandlers";
import { installBreadcrumbConsoleHook } from "./lib/breadcrumbs";
import { installMessageSelectionCopy } from "./lib/messageSelectionCopy";
import { installPerfDebugHook } from "./lib/perfDebug";
import { initFontFamily } from "./lib/fontFamily";
import { initTextSize } from "./lib/textSize";
import { initTypographyPreferences } from "./lib/typographyPreferences";
import { initTheme } from "./lib/theme";
import { initConversationWidth } from "./lib/conversationWidth";
import appShellStylesheetURL from "./styles.css?url";

function isTauriRuntime(): boolean {
  // Mirrors @tauri-apps/api/core's isTauri() check without pulling the native
  // API into the Wails/browser entry chunk. The Tauri-specific module imports
  // that official API only after this host marker is present.
  return (globalThis as typeof globalThis & { isTauri?: unknown }).isTauri === true;
}

// Apply the saved appearance (auto/light/dark) before the first paint.
function initTypographyPlatform() {
  if (typeof document === "undefined" || typeof navigator === "undefined") return;
  const params = new URLSearchParams(window.location.search);
  const override = params.get("platform");
  const marker = `${navigator.platform} ${navigator.userAgent}`;
  const platform =
    override === "darwin" || override === "windows" || override === "linux"
      ? override
      : /Win/i.test(marker)
        ? "windows"
        : /Mac/i.test(marker)
          ? "darwin"
          : "linux";
  document.documentElement.setAttribute("data-platform", platform);
}

initTypographyPlatform();
initTheme();
initConversationWidth();
initTextSize();
initFontFamily();
initTypographyPreferences();

// Pre-warm font fallback stacks so the first frame doesn't flicker between the
// browser default font and the app's configured typeface. Inserting a hidden span
// with CJK + emoji + math glyphs forces the OS font subsystem to resolve and
// cache the fallback chains before React mounts.
function prewarmFontFallbacks() {
  const span = document.createElement("span");
  span.style.cssText = "position:absolute;visibility:hidden;font-size:1px;pointer-events:none";
  span.textContent = "中文日本語한국어 математика 😀🎉✓⚠∑∏∫";
  document.body.appendChild(span);
  // Force layout so the browser resolves font fallback chains.
  void span.offsetHeight;
  requestAnimationFrame(() => {
    requestAnimationFrame(() => {
      span.remove();
    });
  });
}
prewarmFontFallbacks();

installMessageSelectionCopy(document);

const root = document.getElementById("root");
if (!root) throw new Error("missing #root");
const rootElement = root;

async function mountApp() {
  const tauriRuntime = isTauriRuntime();

  // The Wails adapter is intentionally outside Tauri's startup graph. Import
  // it before the generic crash handlers so its drag rejection filter retains
  // the established ordering on the stable desktop path.
  if (!tauriRuntime) {
    const { installWailsNonFileDragErrorSuppression } = await import("./lib/bridge");
    installWailsNonFileDragErrorSuppression();
    if (typeof window !== "undefined" && window.runtime) {
      window.addEventListener("contextmenu", (e) => {
        const target = e.target as HTMLElement | null;
        if (!target?.closest("input, textarea") && !target?.closest(".terminal-view")) e.preventDefault();
      });
    }
  }

  // Install next so startup/runtime failures paint a useful error instead of a
  // featureless webview background, with the recent console trail attached.
  installGlobalCrashHandlers();
  installBreadcrumbConsoleHook();
  installPerformancePressureMonitor();
  installPerfDebugHook();

  // The HTML boot shell paints immediately with critical inline styles. Load
  // the full stylesheet and detected locale in parallel, then replace that
  // shell in one React commit so users never see an unstyled application.
  const wailsModules = tauriRuntime
    ? null
    : Promise.all([import("./App"), import("./lib/i18n"), import("./lib/toast")]);
  const preloadLocaleForMount = wailsModules
    ? wailsModules.then(async ([, { preloadDetectedLocale }]) => {
      await preloadDetectedLocale();
    })
    : Promise.resolve();
  const stylesResult = await Promise.allSettled([
    new Promise<void>((resolve, reject) => {
      const link = document.createElement("link");
      link.rel = "stylesheet";
      link.href = appShellStylesheetURL;
      link.onload = () => resolve();
      link.onerror = () => reject(new Error(`failed to load desktop stylesheet: ${appShellStylesheetURL}`));
      document.head.appendChild(link);
    }),
    preloadLocaleForMount,
  ]);
  const [styleResult, localeResult] = stylesResult;
  if (styleResult.status === "rejected") {
    console.error("failed to load desktop stylesheet", styleResult.reason);
    return;
  }
  if (localeResult.status === "rejected") console.error("failed to preload desktop locale", localeResult.reason);
  let application;
  if (tauriRuntime) {
    const { TauriSessionPreview } = await import("./tauri/TauriSessionPreview");
    application = <TauriSessionPreview />;
  } else {
    const [appModule, i18n, toast] = await wailsModules!;
    const App = appModule.default;
    const { LocaleProvider } = i18n;
    const { ToastProvider } = toast;
    application = (
      <LocaleProvider>
        <ToastProvider>
          <App />
        </ToastProvider>
      </LocaleProvider>
    );
  }
  createRoot(rootElement).render(
    <StrictMode>
      <ErrorBoundary>
        {application}
      </ErrorBoundary>
    </StrictMode>,
  );

  if (!tauriRuntime) {
    void import("./lib/desktopWebViewHeartbeat").then(({ installDesktopWebViewHeartbeat }) => {
      installDesktopWebViewHeartbeat();
    });
  }
}

void mountApp();
