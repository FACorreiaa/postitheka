/** What a video file says about itself, as the server's upload request takes it. */
export interface VideoFacts {
  durationMs?: number;
  width?: number;
  height?: number;
}

/**
 * Reads a video's length and displayed size from its metadata (VID-001). The
 * server cannot measure a file, so this is the only place these come from.
 *
 * Resolves to nothing known when the browser cannot read the file (a codec it
 * does not play, a damaged file) or takes too long: the platform's limits are
 * then not checked before publishing, and the platform itself has the last
 * word.
 */
export function readVideoFacts(url: string, timeoutMs = 8000): Promise<VideoFacts> {
  return new Promise((resolve) => {
    const video = document.createElement("video");
    let done = false;
    const finish = (facts: VideoFacts) => {
      if (done) return;
      done = true;
      clearTimeout(timer);
      video.removeAttribute("src");
      video.load(); // lets the browser drop what it buffered
      resolve(facts);
    };
    const timer = setTimeout(() => finish({}), timeoutMs);
    video.preload = "metadata";
    video.muted = true;
    video.onloadedmetadata = () => finish(factsFrom(video.duration, video.videoWidth, video.videoHeight));
    video.onerror = () => finish({});
    video.src = url;
  });
}

/** Keeps only values that mean something: a live stream reports Infinity, an audio-only file 0 by 0. */
export function factsFrom(durationSeconds: number, width: number, height: number): VideoFacts {
  const facts: VideoFacts = {};
  const ms = Math.round(durationSeconds * 1000);
  if (Number.isFinite(ms) && ms > 0) facts.durationMs = ms;
  if (width > 0 && height > 0) {
    facts.width = width;
    facts.height = height;
  }
  return facts;
}
