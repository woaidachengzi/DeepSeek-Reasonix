import { useState } from "react";
import { ChevronRight } from "lucide-react";
import { useT } from "../lib/i18n";
import { normalizeSearchSources } from "../lib/searchSourcesPresentation";
import type { SearchSource } from "../lib/searchSources";
import { openExternal } from "../lib/externalLinks";
import { writeClipboardText } from "../lib/clipboard";
import { useToast } from "../lib/toast";

export function SearchSourcesPanel({ sources }: { sources?: SearchSource[] }) {
  const t = useT();
  const { showToast } = useToast();
  const [open, setOpen] = useState(false);
  const copyLink = async (href: string) => {
    const copied = await writeClipboardText(href);
    showToast(t(copied ? "richLink.copied" : "richLink.copyFailed"), copied ? "info" : "error");
  };
  const openLink = async (href: string) => {
    if (!await openExternal(href)) showToast(t("settings.about.openLinkFailed"), "error", {
      actionLabel: t("sources.copyLink"), onAction: () => { void copyLink(href); },
    });
  };
  const presentation = normalizeSearchSources(sources);
  if (presentation.visible.length === 0) return null;

  const countLabel = t("sources.count", { n: presentation.visible.length });
  const hiddenLabel = presentation.hiddenCount > 0 ? ` · ${t("sources.hidden", { n: presentation.hiddenCount })}` : "";

  return (
    <section className="msg-search-sources" aria-label={t("sources.title")}>
      <button
        type="button"
        className="msg-search-sources__toggle"
        aria-expanded={open}
        onKeyDown={(event) => {
          if (event.key === "Escape" && open) {
            event.preventDefault();
            setOpen(false);
          }
        }}
        onClick={() => setOpen((value) => !value)}
      >
        <ChevronRight className={`msg-search-sources__chevron${open ? " msg-search-sources__chevron--open" : ""}`} size={15} aria-hidden="true" />
        <span>{t("sources.title")} · {countLabel}{hiddenLabel}</span>
      </button>
      {open && (
        <div className="msg-search-sources__body">
          {presentation.visible.map((source, index) => (
            <div className="msg-search-source" key={`${source.canonicalUrl}-${index}`}>
              <a
                className="msg-search-source__link"
                href={source.href}
                target="_blank"
                rel="noreferrer noopener"
                title={t("sources.open")}
                onClick={(event) => {
                  event.preventDefault();
                  void openLink(source.href);
                }}
                onAuxClick={(event) => {
                  if (event.button !== 1) return;
                  event.preventDefault();
                  void openLink(source.href);
                }}
              >
                <span className="msg-search-source__title">{source.title}</span>
                <span className="msg-search-source__url">
                  <span>{source.displayUrl}</span>
                  <span aria-hidden="true">↗</span>
                </span>
              </a>
              <button
                type="button"
                className="msg-search-source__copy"
                aria-label={t("sources.copyLink")}
                title={t("sources.copyLink")}
                onClick={(event) => {
                  event.preventDefault();
                  event.stopPropagation();
                  void copyLink(source.href);
                }}
              >
                <span aria-hidden="true">⧉</span>
              </button>
            </div>
          ))}
        </div>
      )}
    </section>
  );
}
