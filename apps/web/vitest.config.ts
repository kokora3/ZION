import { defineConfig } from "vitest/config";
import path from "node:path";

export default defineConfig({
  test: { environment: "jsdom", setupFiles: ["./src/test/setup.ts"], restoreMocks: true, include: ["src/**/*.test.{ts,tsx}"] },
  resolve: { alias: { "@": path.resolve(import.meta.dirname, "src") } },
});
