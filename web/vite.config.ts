import react from "@vitejs/plugin-react";
import { defineConfig } from "vitest/config";

// The build goes where internal/webui embeds it (ADR-0013); `make web-dev` proxies the
// API running on its default loopback address.
export default defineConfig({
  plugins: [react()],
  build: { outDir: "../internal/webui/dist/app", emptyOutDir: true },
  server: { proxy: { "/api": { target: "http://127.0.0.1:8080", changeOrigin: true } } }, // Host: the API's, which it allows (ADR-0022)
  test: { environment: "jsdom", include: ["src/**/*.test.{ts,tsx}"] },
});
