import adapter from "@sveltejs/adapter-auto";
import { sveltekit } from "@sveltejs/kit/vite";
import { defineConfig } from "vitest/config";

export default defineConfig({
  plugins: [sveltekit({ adapter: adapter() })],
  test: {
    include: ["tests/unit/**/*.test.ts"],
    environment: "node",
    clearMocks: true,
    restoreMocks: true,
  },
});
