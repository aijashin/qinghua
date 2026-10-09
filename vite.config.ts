import path from "path"
import react from "@vitejs/plugin-react"
import { defineConfig } from "vite"

// https://vite.dev/config/
export default defineConfig({
  base: './',
  // kimi-plugin-inspect-react is an optional authoring inspector. It invokes
  // Babel during every transform and currently fails under pnpm's isolated
  // dependency layout, so keep the runnable demo on the standard React plugin.
  plugins: [react()],
  server: {
    port: 3000,
    proxy: { "/api": { target: process.env.VITE_API_PROXY_TARGET || "http://127.0.0.1:8080", changeOrigin: true } },
  },
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
});
