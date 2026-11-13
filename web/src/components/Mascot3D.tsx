import { useEffect, useRef, useState } from "react";
import { fetchMascot3dModelUrl } from "../api";
import mascot from "../assets/mascot.webp";

/** The visitor asked for less motion or less data: the flat mascot, and no request at all. */
function prefersFlat(): boolean {
  return (
    window.matchMedia("(prefers-reduced-motion: reduce)").matches ||
    (navigator as { connection?: { saveData?: boolean } }).connection?.saveData === true
  );
}

/** Whether WebGL 2 exists, asked before three downloads. The probe's context is released at
 * once, so the page still holds one. */
function hasWebGL(): boolean {
  const gl = document.createElement("canvas").getContext("webgl2");
  gl?.getExtension("WEBGL_lose_context")?.loseContext();
  return gl !== null;
}

/** The mascot on the sign-in page. Flat, unless the server has a model for it (BRD-001: beta
 * only, `MASCOT_3D_MODEL_URL`, read from `GET /client-config`): then the 3D mascot replaces the flat one once its first frame is
 * drawn, and never under reduced motion, Save-Data, without WebGL, or when the model fails to
 * load. The flat image is the first paint and the fallback. One per page: each is a WebGL
 * context. Everything past the checks lives in `lib/mascot3d/scene.ts`, which downloads with
 * three. */
export function Mascot3D() {
  const host = useRef<HTMLDivElement>(null);
  const [live, setLive] = useState(false);

  useEffect(() => {
    const box = host.current;
    if (!box || prefersFlat()) return;

    let gone = false;
    let stop: (() => void) | undefined;
    // Made here, not rendered: each mount gets its own canvas, so a remount can never draw
    // into a context the last one is still releasing.
    // Styled as `.mascot3d canvas`. Without fallback content a screen reader passes it by, and
    // the flat image beside it says nothing either (alt="").
    const canvas = document.createElement("canvas");

    // After the first paint: the flat mascot is on screen before anything else downloads.
    const frame = requestAnimationFrame(async () => {
      const modelUrl = await fetchMascot3dModelUrl();
      if (!modelUrl || gone || !hasWebGL()) return;
      box.append(canvas);
      try {
        const { mountMascot } = await import("../lib/mascot3d/scene");
        if (gone) return;
        const dispose = await mountMascot(box, canvas, { modelUrl, onFirstFrame: () => setLive(true) });
        if (gone) dispose();
        else stop = dispose;
      } catch {
        // No WebGL after all, or the model did not arrive: the flat mascot stays.
        canvas.remove();
      }
    });

    return () => {
      gone = true;
      cancelAnimationFrame(frame);
      stop?.();
      canvas.remove();
    };
  }, []);

  return (
    <div ref={host} className={live ? "mascot3d live" : "mascot3d"}>
      <img src={mascot} alt="" width={718} height={646} />
    </div>
  );
}
