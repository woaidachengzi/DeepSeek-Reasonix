import { useCallback, useContext, useEffect, useMemo, useRef, useState } from "react";
import type { KeyboardEvent as ReactKeyboardEvent, MouseEvent as ReactMouseEvent, ReactNode } from "react";
import { Copy, ExternalLink, FileCode, FolderOpen, Hash, Mail, Save } from "lucide-react";
import { app, openExternal } from "../lib/bridge";
import { writeClipboardText } from "../lib/clipboard";
import { t } from "../lib/i18n";
import { localPathFromHref } from "../lib/localFileUrl";
import type { ExternalOpenerView, ExternalOpenersView } from "../lib/types";
import { useToast } from "../lib/toast";
import { LocalPathActionContext, SourceReferenceContext } from "./SourceReferenceContext";
import { parseSourceReference } from "../lib/sourceReference";
import { ContextMenu, contextMenuPointFromEvent, type ContextMenuItem, type ContextMenuPoint } from "./ContextMenu";

export { localPathFromHref } from "../lib/localFileUrl";

export interface GitHubLinkInfo {
  kind: "issue" | "pull" | "commit";
  owner: string;
  repo: string;
  value: string;
  compactLabel: string;
}

export type LinkIconKind = "github" | "external" | "mail";

function linkText(children: ReactNode): string {
  if (typeof children === "string") return children;
  if (typeof children === "number") return String(children);
  if (!Array.isArray(children)) return "";
  return children
    .map((child) => typeof child === "string" || typeof child === "number" ? String(child) : "")
    .join("");
}

function githubAccessibleLabel(info: GitHubLinkInfo): string {
  const resource = info.kind === "pull"
    ? `pull request #${info.value}`
    : info.kind === "issue"
      ? `issue #${info.value}`
      : `commit ${info.compactLabel}`;
  return `GitHub ${info.owner}/${info.repo} ${resource}`;
}

export function classifyLinkIcon(href?: string): LinkIconKind | null {
  if (!href) return null;
  let url: URL;
  try {
    url = new URL(href);
  } catch {
    return null;
  }

  if (url.protocol === "mailto:") return "mail";
  if (url.protocol !== "https:" && url.protocol !== "http:") return null;
  if (url.protocol === "https:" && ["github.com", "www.github.com"].includes(url.hostname.toLowerCase())) {
    return "github";
  }
  return "external";
}

export function parseGitHubLink(href?: string): GitHubLinkInfo | null {
  if (!href) return null;
  let url: URL;
  try {
    url = new URL(href);
  } catch {
    return null;
  }
  if (url.protocol !== "https:" || !["github.com", "www.github.com"].includes(url.hostname.toLowerCase())) {
    return null;
  }

  const parts = url.pathname.split("/").filter(Boolean);
  if (parts.length !== 4) return null;
  const [owner, repo, resource, value] = parts;
  if (!owner || !repo || !value) return null;

  if (resource === "issues" && /^\d+$/.test(value)) {
    return { kind: "issue", owner, repo, value, compactLabel: `#${value}` };
  }
  if (resource === "pull" && /^\d+$/.test(value)) {
    return { kind: "pull", owner, repo, value, compactLabel: `PR #${value}` };
  }
  if (resource === "commit" && /^[0-9a-f]{7,40}$/i.test(value)) {
    return { kind: "commit", owner, repo, value, compactLabel: value.slice(0, 7) };
  }
  return null;
}

function LinkMark({ kind }: { kind: LinkIconKind }) {
  if (kind === "github") {
    return (
      <svg aria-hidden="true" fill="none" height="13" viewBox="0 0 24 24" width="13">
        <path
          d="M15 22v-4a4.8 4.8 0 0 0-1-3.5c3.3-.4 6.8-1.6 6.8-7A5.4 5.4 0 0 0 19.4 4 5 5 0 0 0 19.3.5S18.2.1 15 1.8a13.4 13.4 0 0 0-7 0C4.8.1 3.7.5 3.7.5A5 5 0 0 0 3.6 4a5.4 5.4 0 0 0-1.4 3.7c0 5.4 3.5 6.6 6.8 7A4.8 4.8 0 0 0 8 18v4"
          stroke="currentColor"
          strokeLinecap="round"
          strokeLinejoin="round"
          strokeWidth="2"
        />
        <path
          d="M8 19c-3 .9-3-1.5-4-2"
          stroke="currentColor"
          strokeLinecap="round"
          strokeLinejoin="round"
          strokeWidth="2"
        />
      </svg>
    );
  }
  if (kind === "mail") return <Mail aria-hidden="true" size={13} strokeWidth={2} />;
  return <ExternalLink aria-hidden="true" size={13} strokeWidth={2} />;
}

function localPathErrorText(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

function localPathSaveErrorText(error: unknown): string {
  const message = localPathErrorText(error);
  if (message === "destination is the source file; choose another path"
    || message === "destination is the same as the source") {
    return t("externalOpener.saveSameSource");
  }
  if (message.startsWith("cannot access document:") || message.startsWith("cannot read source:") || message.startsWith("source file changed;")) {
    return t("externalOpener.saveSourceUnavailable");
  }
  if (/^cannot (?:create destination|save document):.*(?:Permission denied|Operation not permitted|os error (?:13|1)\b)/i.test(message)) {
    return t("externalOpener.saveWriteDenied");
  }
  return t("externalOpener.saveFailed", { error: message });
}

// Menu actions transform information the link already carries (open, copy,
// derive a compact reference); no network or async work is introduced here.
function richLinkMenuItems(
  href: string,
  github: GitHubLinkInfo | null,
  closeMenu: () => void,
  copyText: (text: string) => void,
  open: () => void,
): ContextMenuItem[] {
  const isMail = classifyLinkIcon(href) === "mail";
  let copyTarget = href;
  if (isMail) {
    const address = new URL(href).pathname;
    try {
      copyTarget = decodeURIComponent(address);
    } catch {
      // Keep malformed escapes readable without breaking the menu.
      copyTarget = address;
    }
  }
  const reference = github === null
    ? null
    : github.kind === "commit"
      ? `${github.owner}/${github.repo}@${github.compactLabel}`
      : `${github.owner}/${github.repo}#${github.value}`;
  return [
    {
      key: "open",
      icon: <ExternalLink size={13} />,
      label: isMail ? t("richLink.composeEmail") : t("richLink.openInBrowser"),
      onSelect: () => {
        closeMenu();
        open();
      },
    },
    { type: "separator", key: "open-separator" },
    {
      key: "copy-link",
      icon: <Copy size={13} />,
      label: isMail ? t("richLink.copyEmail") : t("richLink.copyLink"),
      onSelect: () => copyText(copyTarget),
    },
    ...(reference !== null
      ? [{
          key: "copy-reference",
          icon: <Hash size={13} />,
          label: t("richLink.copyReference"),
          onSelect: () => copyText(reference),
        }]
      : []),
  ];
}

function LocalPathMarkdownLink({
  href,
  path,
  children,
}: {
  href: string;
  path: string;
  children: ReactNode;
}) {
  const { showToast } = useToast();
  const [menuPoint, setMenuPoint] = useState<ContextMenuPoint | null>(null);
  const [openers, setOpeners] = useState<ExternalOpenersView>({ openers: [], preferred: "" });

  const closeMenu = useCallback(() => setMenuPoint(null), []);
  const openDefault = useCallback(() => {
    closeMenu();
    void app.OpenLocalPath(path).catch(error => {
      showToast(t("externalOpener.failed", { name: t("externalOpener.openDefault"), error: localPathErrorText(error) }), "error");
    });
  }, [closeMenu, path, showToast]);
  const openerRequestRef = useRef(0);
  const mountedRef = useRef(true);
  const refreshOpeners = useCallback(() => {
    const request = ++openerRequestRef.current;
    void app.ExternalOpeners().then((next) => {
      if (!mountedRef.current || request !== openerRequestRef.current) return;
      setOpeners({
        openers: Array.isArray(next.openers) ? next.openers : [],
        preferred: next.preferred ?? "",
      });
    }).catch(() => {});
  }, []);

  useEffect(() => {
    // React StrictMode replays mount effects in development. Reset the guard
    // during every setup so the replayed mount can still accept discoveries.
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
      openerRequestRef.current += 1;
    };
  }, []);

  const openWith = useCallback((opener: ExternalOpenerView) => {
    closeMenu();
    void app.OpenLocalPathInExternalOpener(path, opener.id).catch((error) => {
      showToast(t("externalOpener.failed", { name: opener.name, error: localPathErrorText(error) }), "error");
    });
  }, [closeMenu, path, showToast]);

  const menuItems = useMemo<ContextMenuItem[]>(() => {
    const openerItems = openers.openers.filter((opener) => opener.kind !== "file-manager").map((opener) => ({
      key: `open-with-${opener.id}`,
      label: t("externalOpener.openIn", { name: opener.name }),
      onSelect: () => openWith(opener),
    }));
    return [
      {
        key: "open-default",
        icon: <ExternalLink size={13} />,
        label: t("externalOpener.openDefault"),
        onSelect: () => {
          closeMenu();
          openDefault();
        },
      },
      ...(openerItems.length > 0
        ? [{ type: "separator" as const, key: "open-with-separator" }, ...openerItems]
        : []),
      { type: "separator" as const, key: "path-separator" },
      {
        key: "reveal",
        icon: <FolderOpen size={13} />,
        label: t("externalOpener.reveal"),
        onSelect: () => {
          closeMenu();
          void app.RevealPath(path).catch((error) => {
            showToast(t("externalOpener.failed", { name: t("externalOpener.reveal"), error: localPathErrorText(error) }), "error");
          });
        },
      },
      {
        key: "copy-path",
        icon: <Copy size={13} />,
        label: t("projectTree.copyPath"),
        onSelect: () => {
          closeMenu();
          void writeClipboardText(path);
        },
      },
      {
        key: "save-as",
        icon: <Save size={13} />,
        label: t("externalOpener.saveAs"),
        onSelect: () => {
          closeMenu();
          void app.SaveLocalPathAs(path).then((savedPath) => {
            if (savedPath) {
              showToast(t("externalOpener.saved", { path: savedPath }), "info");
            }
          }).catch((error) => {
            showToast(localPathSaveErrorText(error), "error");
          });
        },
      },
    ];
  }, [closeMenu, openDefault, openWith, openers.openers, path, showToast]);

  return (
    <>
      <a
        className="md-rich-link md-rich-link--local"
        href={href}
        onClick={(event) => {
          event.preventDefault();
          closeMenu();
          openDefault();
        }}
        onAuxClick={(event) => {
          if (event.button !== 1) return;
          event.preventDefault();
          openDefault();
        }}
        onMouseDown={(event) => {
          if (event.button === 1) event.preventDefault();
        }}
        onContextMenu={(event) => {
          event.preventDefault();
          event.stopPropagation();
          setMenuPoint(contextMenuPointFromEvent(event));
          refreshOpeners();
        }}
      >
        <ExternalLink aria-hidden="true" size={13} strokeWidth={2} />
        <span className="md-rich-link__label">{children}</span>
      </a>
      <ContextMenu
        open={menuPoint !== null}
        point={menuPoint}
        items={menuItems}
        onClose={closeMenu}
        minWidth={220}
        ariaLabel={t("externalOpener.choose")}
      />
    </>
  );
}

export function RichMarkdownLink({
  href,
  children,
}: {
  href?: string;
  children: ReactNode;
}) {
  // Menu state lives here so web/mail links get a context menu; local file
  // links keep their own richer menu inside LocalPathMarkdownLink below.
  const { showToast } = useToast();
  const github = parseGitHubLink(href);
  const openSource = useContext(SourceReferenceContext);
  const scopedPathAction = useContext(LocalPathActionContext);
  const source = parseSourceReference(href);
  const [menuPoint, setMenuPoint] = useState<ContextMenuPoint | null>(null);
  const closeMenu = useCallback(() => setMenuPoint(null), []);
  const copyText = useCallback((text: string) => {
    closeMenu();
    void writeClipboardText(text).then((copied) => {
      if (copied) showToast(t("richLink.copied"), "info");
      else showToast(t("richLink.copyFailed"), "error");
    });
  }, [closeMenu, showToast]);
  const open = useCallback(() => {
    if (!href) return;
    void openExternal(href).then(opened => {
      if (!opened) showToast(t("settings.about.openLinkFailed"), "error", {
        actionLabel: t("richLink.copyLink"), onAction: () => copyText(href),
      });
    });
  }, [copyText, href, showToast]);
  const menuItems = useMemo(
    () => richLinkMenuItems(href ?? "", github, closeMenu, copyText, open),
    [closeMenu, copyText, github, href, open],
  );
  const openMenu = (event: ReactMouseEvent<HTMLAnchorElement> | ReactKeyboardEvent<HTMLAnchorElement>) => {
    event.preventDefault();
    event.stopPropagation();
    setMenuPoint(contextMenuPointFromEvent(event));
  };

  const nativeLocal = localPathFromHref(href);
  if (scopedPathAction && (source || nativeLocal !== null)) {
    const path = source?.path ?? nativeLocal!;
    const activate = (event: ReactMouseEvent<HTMLAnchorElement>) => { event.preventDefault(); scopedPathAction(path); };
    return <a className="md-rich-link md-rich-link--source" href={href} title={path}
      onClick={activate} onAuxClick={event => { if (event.button === 1) activate(event); }}
      onMouseDown={event => { if (event.button === 1) event.preventDefault(); }}
      onContextMenu={event => { event.preventDefault(); event.stopPropagation(); }}>
      <FileCode aria-hidden="true" size={13} strokeWidth={2} /><span className="md-rich-link__label">{children}</span>
    </a>;
  }
  // Existing file:// links retain their native opener and rich context menu.
  // A line suffix is an explicit request for the workspace source preview.
  if (openSource && source && (source.line !== undefined || nativeLocal === null)) {
    const activate = (event: ReactMouseEvent<HTMLAnchorElement>) => {
      event.preventDefault();
      openSource(source);
    };
    return <a className="md-rich-link md-rich-link--source" href={href}
      title={`${source.path}${source.line ? ` (line ${source.line})` : ""}`}
      onClick={activate} onAuxClick={event => { if (event.button === 1) activate(event); }}
      onMouseDown={event => { if (event.button === 1) event.preventDefault(); }}>
      <FileCode aria-hidden="true" size={13} strokeWidth={2} />
      <span className="md-rich-link__label">{children}</span>
    </a>;
  }
  const local = nativeLocal ?? (source && /^(?:\/|[A-Za-z]:[\\/])/.test(source.path) ? source.path : null);
  if (local !== null) {
    return <LocalPathMarkdownLink href={href ?? ""} path={local} children={children} />;
  }
  if (source?.line !== undefined && !openSource) return <code className="md-code">{children}</code>;

  const iconKind = classifyLinkIcon(href);
  const compactLabel = github && linkText(children) === href ? github.compactLabel : undefined;
  const accessibleLabel = github ? githubAccessibleLabel(github) : undefined;
  const handlers = {
    onClick: (event: ReactMouseEvent<HTMLAnchorElement>) => {
      event.preventDefault();
      open();
    },
    onAuxClick: (event: ReactMouseEvent<HTMLAnchorElement>) => {
      if (event.button !== 1) return;
      event.preventDefault();
      open();
    },
    onMouseDown: (event: ReactMouseEvent<HTMLAnchorElement>) => {
      if (event.button === 1) event.preventDefault();
    },
  };

  if (!iconKind) {
    return <a href={href} {...handlers}>{children}</a>;
  }

  return (
    <>
      <a
        aria-label={compactLabel ? accessibleLabel : undefined}
        className={`md-rich-link md-rich-link--${iconKind}`}
        href={href}
        title={github ? accessibleLabel : undefined}
        {...handlers}
        onContextMenu={openMenu}
        onKeyDown={(event) => {
          if (event.key !== "ContextMenu" && !(event.shiftKey && event.key === "F10")) return;
          openMenu(event);
        }}
      >
        <LinkMark kind={iconKind} />
        <span
          className="md-rich-link__label"
          data-display-label={compactLabel}
        >
          {children}
        </span>
      </a>
      <ContextMenu
        open={menuPoint !== null}
        point={menuPoint}
        items={menuItems}
        onClose={closeMenu}
        minWidth={200}
        ariaLabel={t("richLink.menuAriaLabel")}
      />
    </>
  );
}
