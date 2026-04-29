import { defineConfig } from "vitest/config";

export default defineConfig({
  server: {
    proxy: {
      "/ws": { target: "ws://localhost:8080", ws: true, changeOrigin: true },
      "/session": { target: "http://localhost:8080", changeOrigin: true },
      "/healthz": { target: "http://localhost:8080", changeOrigin: true },
    },
  },
  test: {
    environment: "node",
    include: ["src/**/*.test.ts"],
  },
});
