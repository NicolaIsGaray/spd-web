import { defineConfig } from "vite"
import react from "@vitejs/plugin-react"
import tailwindcss from "@tailwindcss/vite"

// El gateway (backend/services/gateway, puerto 8080) es el único punto de entrada al backend.
// Durante el desarrollo el front le habla por acá: /api para REST y /ws para WebSocket
// (ws: true hace que el proxy también negocie el upgrade a WebSocket).
const gateway = process.env.SPD_GATEWAY_URL ?? "http://localhost:8080"

export default defineConfig({
  plugins: [react(), tailwindcss()],
  // El CORS del backend acepta http://localhost:5173 (variable ALLOWED_ORIGINS)
  server: {
    port: 5173,
    proxy: {
      "/api": { target: gateway, changeOrigin: false },
      "/ws": { target: gateway, ws: true, changeOrigin: false },
    },
  },
})
