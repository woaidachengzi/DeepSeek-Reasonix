import { createContext } from "react";
import type { MarkdownImageView } from "../lib/markdownImage";

export const MarkdownImageTabContext = createContext("");

// Feature-scoped adapter: Tauri does not impersonate the legacy Wails API.
export const MarkdownImageResolverContext = createContext<((source: string) => Promise<MarkdownImageView>) | null>(null);

export interface MarkdownImagePreview { url: string; name?: string }
export const MarkdownImagePreviewContext = createContext<((image: MarkdownImagePreview) => void) | null>(null);
