import type { Rules } from "./api";

export interface Draft {
  text: string;
  media: { contentType: string; size: number }[];
}

/**
 * Counts text the way the server does for platforms without their own
 * counter: Unicode code points. `text.length` counts UTF-16 units and would
 * call an emoji two characters.
 */
export function countText(text: string): number {
  return [...text].length;
}

/**
 * Mirrors publisher.Validate in Go, so the composer shows what the server
 * would say before Publish is pressed. The server still checks; this is for
 * feedback while typing, not for trust.
 */
export function problems(rules: Rules, draft: Draft): string[] {
  const out: string[] = [];
  const hasText = draft.text.trim() !== "";
  if (!hasText && draft.media.length === 0) return out; // nothing written yet is not an error

  if (rules.requires_text && !hasText) out.push("Needs text");
  const n = countText(draft.text);
  if (n > rules.max_text_length) out.push(`${n - rules.max_text_length} characters over`);

  if (rules.requires_media && draft.media.length === 0) out.push("Needs a photo or video");
  if (rules.max_media === 0 && draft.media.length > 0) {
    out.push("Takes no media");
    return out;
  }
  if (draft.media.length > rules.max_media) out.push(`At most ${rules.max_media} files`);

  let total = 0;
  for (const m of draft.media) {
    if (!rules.media_types.includes(m.contentType)) out.push(`Cannot take ${m.contentType}`);
    if (m.size > rules.max_media_bytes) out.push(`A file is over ${formatBytes(rules.max_media_bytes)}`);
    total += m.size;
  }
  if (rules.max_total_media_bytes > 0 && total > rules.max_total_media_bytes) {
    out.push(`Files total over ${formatBytes(rules.max_total_media_bytes)}`);
  }
  return out;
}

export function formatBytes(n: number): string {
  if (n >= 1 << 20) return `${Math.round(n / (1 << 20))} MB`;
  if (n >= 1 << 10) return `${Math.round(n / (1 << 10))} KB`;
  return `${n} bytes`;
}

export const platformNames: Record<string, string> = {
  discord: "Discord",
  x: "X",
  instagram: "Instagram",
  facebook: "Facebook",
  tiktok: "TikTok",
  whatsapp: "WhatsApp",
};
