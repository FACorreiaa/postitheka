import type { Rules } from "./api";

export interface Draft {
  text: string;
  media: { contentType: string; size: number }[];
}

// An emoji part, as publisher.isEmojiPart in Go defines it: any symbol (So)
// or emoji component. The spec's text_counting description is the contract.
const emojiPart = /[\p{So}\u200D\u20E3\uFE00-\uFE0F\u{1F3FB}-\u{1F3FF}\u{E0020}-\u{E007F}]/u;
const utf8 = new TextEncoder();

/**
 * Counts text the way the server does for this platform's text_counting.
 * code_points counts Unicode code points: `text.length` counts UTF-16 units
 * and would call an emoji two characters. emoji_bytes counts an emoji as its
 * UTF-8 length (Threads).
 */
export function countText(text: string, mode: Rules["text_counting"] = "code_points"): number {
  let n = 0;
  for (const ch of text) {
    n += mode === "emoji_bytes" && emojiPart.test(ch) ? utf8.encode(ch).length : 1;
  }
  return n;
}

// The links publisher.CountLinks counts: a scheme or www., up to whitespace,
// compared without case or trailing punctuation.
const linkPattern = /\b(?:https?:\/\/|www\.)[^\s<>"]+/gi;

export function countLinks(text: string): number {
  const seen = new Set<string>();
  for (const m of text.match(linkPattern) ?? []) seen.add(m.replace(/[.,;:!?)\]}'"]+$/, "").toLowerCase());
  return seen.size;
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
  const n = countText(draft.text, rules.text_counting);
  if (n > rules.max_text_length) out.push(`${n - rules.max_text_length} characters over`);
  if (rules.max_links > 0) {
    const links = countLinks(draft.text);
    if (links > rules.max_links) out.push(`At most ${rules.max_links} link${rules.max_links === 1 ? "" : "s"}`);
  }

  if (rules.requires_media && draft.media.length === 0) out.push("Needs a photo or video");
  if (rules.max_media === 0 && draft.media.length > 0) {
    out.push("Takes no media");
    return out;
  }
  if (draft.media.length > rules.max_media) out.push(`At most ${rules.max_media} files`);

  let total = 0;
  let videos = 0;
  for (const m of draft.media) {
    if (!rules.media_types.includes(m.contentType)) out.push(`Cannot take ${m.contentType}`);
    const video = m.contentType.startsWith("video/");
    const limit = video && rules.max_video_bytes > 0 ? rules.max_video_bytes : rules.max_media_bytes;
    if (m.size > limit) out.push(`A ${video ? "video" : "file"} is over ${formatBytes(limit)}`);
    if (video) videos++;
    total += m.size;
  }
  if (rules.video_alone && videos > 0 && draft.media.length > 1) out.push("A video must be posted alone");
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
  linkedin: "LinkedIn",
  threads: "Threads",
  x: "X",
  instagram: "Instagram",
  facebook: "Facebook",
  tiktok: "TikTok",
  whatsapp: "WhatsApp",
};
