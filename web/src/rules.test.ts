import { describe, expect, it } from "vitest";
import type { Rules } from "./api";
import { countText, formatLength, problems } from "./rules";
import { factsFrom } from "./lib/video-facts";

const rules: Rules = {
  max_text_length: 100,
  text_counting: "code_points",
  max_links: 0,
  max_media: 4,
  max_media_bytes: 1 << 30,
  max_video_bytes: 0,
  max_total_media_bytes: 0,
  media_types: ["video/mp4", "image/jpeg"],
  requires_text: false,
  requires_media: false,
  video_alone: false,
  min_video_ms: 3000,
  max_video_ms: 300_000,
  min_video_aspect: 0.5,
  max_video_aspect: 2,
  max_video_width: 1920,
  options: [],
  max_comment_length: 0,
};

const video = (durationMs?: number, width?: number, height?: number) =>
  problems(rules, { text: "", media: [{ contentType: "video/mp4", size: 1, durationMs, width, height }] });

describe("video rules", () => {
  it("accepts a video within every limit, and at the limits", () => {
    expect(video(60_000, 1080, 1920)).toEqual([]);
    expect(video(3000, 1080, 1920)).toEqual([]);
    expect(video(300_000, 1920, 1080)).toEqual([]);
  });

  it("names the limit a video breaks", () => {
    expect(video(2500, 1080, 1920)).toEqual(["A video is shorter than 3 seconds"]);
    expect(video(372_000, 1080, 1920)).toEqual(["A video is longer than 5 minutes"]);
    expect(video(60_000, 400, 1000)).toEqual(["A video's shape is not accepted"]);
    expect(video(60_000, 3840, 2160)).toEqual(["A video is wider than 1920 pixels"]);
  });

  it("does not check what the file did not give up", () => {
    expect(video()).toEqual([]);
    expect(video(undefined, 4000, undefined)).toEqual([]);
    expect(video(600_000)).toEqual(["A video is longer than 5 minutes"]);
  });

  it("has no video limits where the platform states none", () => {
    const free = { ...rules, min_video_ms: 0, max_video_ms: 0, min_video_aspect: 0, max_video_aspect: 0, max_video_width: 0 };
    expect(problems(free, { text: "", media: [{ contentType: "video/mp4", size: 1, durationMs: 9e6, width: 50000, height: 1 }] })).toEqual([]);
  });

  it("never measures an image as a video", () => {
    expect(problems(rules, { text: "", media: [{ contentType: "image/jpeg", size: 1, durationMs: 9e6, width: 9000, height: 10 }] })).toEqual([]);
  });
});

describe("formatLength", () => {
  it("writes limits in words", () => {
    expect(formatLength(3000)).toBe("3 seconds");
    expect(formatLength(1000)).toBe("1 second");
    expect(formatLength(60_000)).toBe("1 minute");
    expect(formatLength(900_000)).toBe("15 minutes");
    expect(formatLength(90_000)).toBe("1:30");
  });
});

describe("factsFrom", () => {
  it("keeps only values that mean something", () => {
    expect(factsFrom(12.3456, 1080, 1920)).toEqual({ durationMs: 12346, width: 1080, height: 1920 });
    expect(factsFrom(Infinity, 0, 0)).toEqual({});
    expect(factsFrom(NaN, 640, 0)).toEqual({});
  });
});

describe("options", () => {
  const withOptions: Rules = {
    ...rules,
    options: [
      { key: "title", label: "Title", max_length: 5, required: true },
      { key: "flair", label: "Flair", max_length: 5, required: false },
    ],
  };
  const check = (options?: Record<string, string>) => problems(withOptions, { text: "x", media: [], options });

  it("accepts what fits, counted in characters", () => {
    expect(check({ title: "Hello", flair: "news" })).toEqual([]);
    expect(check({ title: " 😀😀😀😀😀 " })).toEqual([]);
  });

  it("asks for a required option and names one that is too long", () => {
    expect(check()).toEqual(["Needs a title"]);
    expect(check({ title: "   " })).toEqual(["Needs a title"]);
    expect(check({ title: "Hello!", flair: "breaking" })).toEqual(["Title is 1 characters over", "Flair is 3 characters over"]);
  });
});

// The same cases as TestCountXWeighted in Go and the kit's RulesTests.swift.
describe("x_weighted counting", () => {
  it.each([
    ["", 0],
    ["Hello, world!", 13],
    ["café", 4],
    ["cafe\u0301", 4],
    ["日本語", 6],
    ["👋", 2],
    ["“quoted” – dash", 15],
    ["https://example.com", 23],
    ["https://example.com/a/very/long/path?with=query", 23],
    ["see https://example.com.", 28],
    ["www.example.com and http://a.test", 51],
    ["🙋🏽", 4],
  ])("%j counts %i", (text, want) => {
    expect(countText(text, "x_weighted")).toBe(want);
  });
});

describe("first comment", () => {
  it("is held to the platform's limit, trimmed as the server trims it", () => {
    const takes = { ...rules, max_comment_length: 5 };
    expect(problems(takes, { text: "hi", media: [], comment: "  #abc  " })).toEqual([]);
    expect(problems(takes, { text: "hi", media: [], comment: "#abcdefg" })).toEqual(["First comment is 3 characters over"]);
  });

  it("is no problem where the platform takes none", () => {
    expect(problems(rules, { text: "hi", media: [], comment: "#".repeat(5000) })).toEqual([]);
  });
});
