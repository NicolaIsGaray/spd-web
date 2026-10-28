import { defineConfig } from "vite"
import react from "@vitejs/plugin-react"
import tailwindcss from "@tailwindcss/vite"

export default defineConfig({
  plugins: [react(), tailwindcss()],
  // El CORS del back de referencia acepta http://localhost:5173
  server: { port: 5173 },
})
