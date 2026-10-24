import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
export default defineConfig({
  plugins: [react()],
  clearScreen: false,
  server: {
    port: 1420,
    strictPort: true,
    watch: {
      ignored: [
        "**/src-tauri/**",
        "**/.upstream/**",
        "**/.tools/**",
        "**/artifacts/**",
        "**/docs/**",
      ],
    },
    fs: {
      deny: [
        ".env",
        ".env.*",
        "*.{crt,pem,key}",
        "**/.git/**",
        "**/.upstream/**",
        "**/.tools/**",
        "**/artifacts/**",
        "**/docs/research/**",
        "**/src-tauri/target/**",
      ],
    },
  },
  envPrefix: ["VITE_", "TAURI_ENV_"],
  build: { target: "es2022" },
});
