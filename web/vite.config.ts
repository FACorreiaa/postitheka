import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

// In development the API runs separately on :8095 and Vite proxies to it, so
// the app talks to the same origin it will in production, where the Go binary
// serves both.
export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    strictPort: true,
    proxy: { "/v1": "http://localhost:8095" },
  },
});
