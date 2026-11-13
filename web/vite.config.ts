/// <reference types="node" />
import { createReadStream, existsSync } from "node:fs";
import { basename, join } from "node:path";
import { fileURLToPath } from "node:url";
import react from "@vitejs/plugin-react";
import { defineConfig, type Plugin } from "vite";

/** Serves the built 3D mascot (`out/mascot3d/`, git-ignored, BRD-001) at `/mascot3d/` while
 * developing, so `MASCOT_3D_MODEL_URL=/mascot3d/mascot.glb` works locally. Development only:
 * the model never enters `dist`, so it never enters the server binary either. */
function mascot3dModels(): Plugin {
  const dir = fileURLToPath(new URL("../out/mascot3d/", import.meta.url));
  return {
    name: "postitheka-mascot3d-models",
    apply: "serve",
    configureServer(server) {
      server.middlewares.use("/mascot3d", (req, res, next) => {
        // basename: one flat folder, nothing above it is reachable.
        const name = basename(decodeURIComponent((req.url ?? "").split("?")[0] ?? ""));
        const file = join(dir, name);
        if (!name.endsWith(".glb") || !existsSync(file)) return next();
        res.setHeader("Content-Type", "model/gltf-binary");
        res.setHeader("Cache-Control", "no-cache");
        createReadStream(file).pipe(res);
      });
    },
  };
}

// In development the API runs separately on :8095 and Vite proxies to it, so
// the app talks to the same origin it will in production, where the Go binary
// serves both.
export default defineConfig({
  plugins: [react(), mascot3dModels()],
  // The one dynamic import (the 3D mascot's scene) is a single chunk with nothing to preload,
  // so the modulepreload polyfill is only weight on the entry every visitor downloads. Vite's
  // `__vitePreload` wrapper around the import stays either way (it is not configurable).
  // Sizes are checked by scripts/size-budget.mjs, which knows the large lazy chunk is three.
  build: { modulePreload: false, chunkSizeWarningLimit: 700 },
  server: {
    port: 5173,
    strictPort: true,
    proxy: { "/v1": "http://localhost:8095", "/client-config": "http://localhost:8095" },
  },
});
