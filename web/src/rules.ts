import type { Rules } from "./api";

export interface Draft {
  text: string;
  media: { contentType: string; size: number; durationMs?: number; width?: number; height?: number }[];
  /** What was set for this destination alone, by the keys in its rules' options. */
  options?: Record<string, string>;
  /** The post's first comment (POST-002), the same for every destination. */
  comment?: string;
}

// An emoji part, as publisher.isEmojiPart in Go defines it: any symbol (So)
// or emoji component. The spec's text_counting description is the contract.
const emojiPart = /[\p{So}\u200D\u20E3\uFE00-\uFE0F\u{1F3FB}-\u{1F3FF}\u{E0020}-\u{E007F}]/u;
const utf8 = new TextEncoder();

/**
 * Counts text the way the server does for this platform's text_counting.
 * code_points counts Unicode code points: `text.length` counts UTF-16 units
 * and would call an emoji two characters. emoji_bytes counts an emoji as its
 * UTF-8 length (Threads). x_weighted is X's count (countXWeighted).
 */
export function countText(text: string, mode: Rules["text_counting"] = "code_points"): number {
  if (mode === "x_weighted") return countXWeighted(text);
  let n = 0;
  for (const ch of text) {
    n += mode === "emoji_bytes" && emojiPart.test(ch) ? utf8.encode(ch).length : 1;
  }
  return n;
}

// The links publisher.CountLinks counts: a scheme or www., up to whitespace,
// compared without case or trailing punctuation.
const linkPattern = /\b(?:https?:\/\/|www\.)[^\s<>"]+/gi;
// Punctuation that ends a sentence, not a link.
const linkTrailing = /[.,;:!?)\]}'"]+$/;

export function countLinks(text: string): number {
  const seen = new Set<string>();
  for (const m of text.match(linkPattern) ?? []) seen.add(m.replace(linkTrailing, "").toLowerCase());
  return seen.size;
}

// Mirrors countXWeighted in Go: NFC, a link is 23, a code point in X's short
// list of ranges is 1 and any other is 2. Like the server it counts an emoji
// by its code points, where X counts the whole emoji as 2.
function countXWeighted(text: string): number {
  const s = text.normalize("NFC");
  let n = 0;
  let last = 0;
  for (const m of s.matchAll(linkPattern)) {
    n += xWeight(s.slice(last, m.index)) + 23;
    last = m.index + m[0].replace(linkTrailing, "").length;
  }
  return n + xWeight(s.slice(last));
}

function xWeight(s: string): number {
  let n = 0;
  for (const ch of s) {
    const c = ch.codePointAt(0)!;
    n += c <= 0x10ff || (c >= 0x2000 && c <= 0x200d) || (c >= 0x2010 && c <= 0x201f) || (c >= 0x2032 && c <= 0x2037) ? 1 : 2;
  }
  return n;
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

  // Only where the platform takes a comment: elsewhere the post goes alone.
  const comment = countText((draft.comment ?? "").trim(), rules.text_counting);
  if (rules.max_comment_length > 0 && comment > rules.max_comment_length) {
    out.push(`First comment is ${comment - rules.max_comment_length} characters over`);
  }

  // Mirrors Rules.optionIssues in Go, less the unknown key: a form built from
  // the rules cannot send one.
  for (const o of rules.options) {
    const value = (draft.options?.[o.key] ?? "").trim();
    const length = countText(value);
    if (value === "" && o.required) out.push(`Needs a ${o.label.toLowerCase()}`);
    else if (length > o.max_length) out.push(`${o.label} is ${length - o.max_length} characters over`);
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
    if (video) {
      videos++;
      out.push(...videoProblems(rules, m));
    }
    total += m.size;
  }
  if (rules.video_alone && videos > 0 && draft.media.length > 1) out.push("A video must be posted alone");
  if (rules.max_total_media_bytes > 0 && total > rules.max_total_media_bytes) {
    out.push(`Files total over ${formatBytes(rules.max_total_media_bytes)}`);
  }
  return out;
}

// Mirrors Rules.videoIssues in Go: a limit is checked only against a value
// the file gave up, and 0 means the platform has no such limit.
function videoProblems(rules: Rules, m: Draft["media"][number]): string[] {
  const out: string[] = [];
  if (m.durationMs && m.durationMs > 0) {
    if (rules.min_video_ms > 0 && m.durationMs < rules.min_video_ms) out.push(`A video is shorter than ${formatLength(rules.min_video_ms)}`);
    if (rules.max_video_ms > 0 && m.durationMs > rules.max_video_ms) out.push(`A video is longer than ${formatLength(rules.max_video_ms)}`);
  }
  if (m.width && m.height && m.width > 0 && m.height > 0) {
    const aspect = m.width / m.height;
    if ((rules.min_video_aspect > 0 && aspect < rules.min_video_aspect) || (rules.max_video_aspect > 0 && aspect > rules.max_video_aspect)) {
      out.push("A video's shape is not accepted");
    }
    if (rules.max_video_width > 0 && m.width > rules.max_video_width) out.push(`A video is wider than ${rules.max_video_width} pixels`);
  }
  return out;
}

/** A length limit in words: "3 seconds", "5 minutes", "1:30". */
export function formatLength(ms: number): string {
  const s = Math.round(ms / 1000);
  if (s < 60) return `${s} second${s === 1 ? "" : "s"}`;
  if (s % 60 === 0) return `${s / 60} minute${s === 60 ? "" : "s"}`;
  return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, "0")}`;
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
  reddit: "Reddit",
};
