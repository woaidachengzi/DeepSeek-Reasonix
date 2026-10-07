import type { TauriBridgeAttachment, TauriStagedImage } from "../lib/tauriBridge";

type PasteData = Pick<DataTransfer, "files" | "items" | "getData">;

/** null = ordinary text; [] = native image fallback; File[] = WebView image data. */
export function pastedImageFiles(data: PasteData): File[] | null {
  const files = Array.from(data.files).filter(file => file.type.startsWith("image/"));
  if (files.length) return files;
  const items = Array.from(data.items).filter(item => item.kind === "file" && item.type.startsWith("image/"));
  const images = items.flatMap(item => { const file = item.getAsFile(); return file ? [file] : []; });
  if (images.length) return images;
  return data.getData("text/plain") ? null : [];
}

export function pastedImageDataURL(file: File): Promise<string> {
  if (!/^image\/(png|jpeg|gif|webp)$/.test(file.type)) return Promise.reject(new Error("图片格式不受支持；请使用 PNG、JPEG、GIF 或 WebP。"));
  if (file.size === 0 || file.size > 16 * 1024 * 1024) return Promise.reject(new Error("图片为空或超过 16 MiB；请压缩图片后重试。"));
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => typeof reader.result === "string" ? resolve(reader.result) : reject(new Error("无法读取图片；请重新复制后粘贴。"));
    reader.onerror = () => reject(new Error("无法读取图片；请重新复制后粘贴。"));
    reader.readAsDataURL(file);
  });
}

/** One image transport, including native-only screenshots. Every stale result is discarded. */
export async function attachPastedImages(files: File[], options: {
  sessionId?: string;
  isCurrent: () => boolean;
  stage: (dataUrl?: string) => Promise<TauriStagedImage | null>;
  discard: (token: string) => Promise<void>;
  attach: (sessionId: string, path: string) => Promise<TauriBridgeAttachment>;
  add: (attachment: TauriBridgeAttachment) => void;
  queue: (image: TauriStagedImage) => void;
  read?: (file: File) => Promise<string>;
}) {
  for (const file of files.length ? files : [undefined]) {
    if (!options.isCurrent()) return;
    const dataUrl = file ? await (options.read ?? pastedImageDataURL)(file) : undefined;
    if (!options.isCurrent()) return;
    const image = await options.stage(dataUrl);
    if (!image) continue;
    if (!options.isCurrent()) { await options.discard(image.token); return; }
    if (!options.sessionId) { options.queue(image); continue; }
    try {
      const attachment = await options.attach(options.sessionId, image.path);
      if (options.isCurrent()) options.add(attachment);
    } finally {
      await options.discard(image.token);
    }
  }
}
