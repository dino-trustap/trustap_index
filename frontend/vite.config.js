import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// Served by the Go binary at /dashboard/ in production (go:embed of dist/).
// In dev, vite proxies API calls to the local Go server.
export default defineConfig({
  base: "/dashboard/",
  plugins: [react()],
  server: {
    proxy: {
      "/api": "http://localhost:8090",
      "/feeds": "http://localhost:8090",
      "/shop": "http://localhost:8090",
    },
  },
});
