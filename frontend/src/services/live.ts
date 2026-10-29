import type { ApiPresentationDetail, ApiSlideState } from "../types"
import { apiGet, apiSend, apiUpload } from "./http"

// Dominios "presentaciones" y "tiempo real" del gateway: la subida y metadatos de una
// presentación, la imagen de cada diapositiva, y el WebSocket con el estado compartido en vivo.

// ─── Presentations ──────────────────────────────────────────────────────────────────────────

const MAX_UPLOAD_MB = 100 // alinear con el MAX_UPLOAD_MB que configure el backend

/** Una presentación: metadatos y la lista de diapositivas. GET /api/presentations/:id */
export function getPresentation(id: string): Promise<ApiPresentationDetail> {
  return apiGet<ApiPresentationDetail>(`/presentations/${id}`)
}

/** Sube un archivo (PDF o PPTX) y devuelve la presentación convertida con sus diapositivas.
 * POST /api/presentations/upload (multipart/form-data, campo "file"). 400 si el formato no
 * sirve, 413 si supera el límite, 503 si faltan los convertidores. */
export function uploadPresentation(file: File): Promise<ApiPresentationDetail> {
  const form = new FormData()
  form.append("file", file)
  return apiUpload<ApiPresentationDetail>("/presentations/upload", form)
}

export { MAX_UPLOAD_MB }

// ─── Realtime ───────────────────────────────────────────────────────────────────────────────

/** Orden del presentador: pasar a la siguiente, a la anterior, o ir a una diapositiva concreta. */
export type ControlCommand = { action: "next" | "prev" } | { action: "goto"; slide: number }

/**
 * Control del presentador. Afecta la sesión en vivo: el Hub difunde el nuevo estado a todos los
 * visores (incluido quien ejecutó el comando). 404 si la presentación no está en vivo; 400 si
 * el comando está mal.
 */
export function controlPresentation(presentationId: string, command: ControlCommand): Promise<ApiSlideState> {
  const body = "slide" in command ? { action: command.action, slide: command.slide } : { action: command.action }
  return apiSend<ApiSlideState>(`/presentations/${presentationId}/control`, "POST", body)
}

/**
 * Se conecta al WebSocket de una sesión en vivo y devuelve la función de desconexión. Cada
 * mensaje del servidor es un estado (ApiSlideState): el bucle lo entrega a onState.
 */
export function connectPresentationSocket(
  presentationId: string,
  onState: (state: ApiSlideState) => void,
  onError?: (error: Event | CloseEvent) => void,
): () => void {
  // Mismo origen: en desarrollo el proxy de Vite reenvía /ws al gateway.
  const protocol = window.location.protocol === "https:" ? "wss:" : "ws:"
  const socket = new WebSocket(`${protocol}//${window.location.host}/ws/presentation/${presentationId}`)

  socket.onmessage = (event) => {
    try {
      onState(JSON.parse(event.data) as ApiSlideState)
    } catch {
      // Mensaje no entendible: ignorar sin cortar la sesión.
    }
  }
  if (onError) {
    socket.onclose = onError
    socket.onerror = onError
  }

  return () => {
    // 1000: cierre normal de parte del cliente (el back responde 1001 al reiniciar).
    socket.close(1000)
  }
}
