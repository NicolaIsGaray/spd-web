// Cliente HTTP base: todos los servicios pasan por acá. En desarrollo Vite hace de proxy hacia
// el gateway (ver vite.config.ts); en producción, el reverse-proxy debe publicar /api y /ws.
//
// El backend responde los errores siempre {"error": "<mensaje>"} con el código HTTP correcto,
// o el cuerpo correspondiente (201 crea, 200/204 leen, 409 conflicto, 422 desconocido...).

const API_BASE = "/api"

export class ApiError extends Error {
  readonly status: number

  constructor(status: number, message: string) {
    super(message)
    this.name = "ApiError"
    this.status = status
  }
}

type Options = {
  /** Método HTTP. Por defecto GET. */
  method?: string
  /** Cuerpo JSON (se serializa). No usar junto a bodyRaw. */
  body?: unknown
  /** Cuerpo crudo (p. ej. un Blob o FormData). No usar junto a body. */
  bodyRaw?: BodyInit
  /** Cabeceras extra (p. ej. Content-Type para bodyRaw). */
  headers?: Record<string, string>
}

function isNotOk(response: Response) {
  return !response.ok;
}

type Result<T> = { data: T; status: number };

async function requestRaw<T>(path: string, options: Options = {}): Promise<Result<T>> {
  const { method = "GET", body, bodyRaw, headers } = options

  const response = await fetch(`${API_BASE}${path}`, {
    method,
    headers: body !== undefined ? { "Content-Type": "application/json", ...headers } : headers,
    body: body !== undefined ? JSON.stringify(body) : bodyRaw,
  })

  if (isNotOk(response)) {
    let message = `Error ${response.status}`
    try {
      const parsed: unknown = await response.json()
      if (isErrorBody(parsed) && parsed.error) message = parsed.error
    } catch {
      // Respuesta sin cuerpo JSON: nos quedamos con el mensaje genérico.
    }
    throw new ApiError(response.status, message)
  }

  // 204 sin contenido: nada que decodificar.
  if (response.status === 204) return { data: undefined as T, status: response.status }
  return { data: (await response.json()) as T, status: response.status }
}

async function request<T>(path: string, options: Options = {}): Promise<T> {
  const { data } = await requestRaw<T>(path, options)
  return data
}

function isErrorBody(value: unknown): value is { error: string } {
  return typeof value === "object" && value !== null && "error" in value
}

export function apiGet<T>(path: string): Promise<T> {
  return request<T>(path)
}

export function apiSend<T>(path: string, method: "POST" | "PATCH" | "DELETE", body?: unknown): Promise<T> {
  return request<T>(path, { method, body })
}

/** Como apiSend, pero además del cuerpo devuelve el código HTTP: los endpoints que crean (o
 * re-usan) pertenencias como POST /api/rooms/:id/members distinguen el 201 del 200. */
export function apiSendWithStatus<T>(path: string, method: "POST" | "PATCH" | "DELETE", body?: unknown): Promise<Result<T>> {
  return requestRaw<T>(path, { method, body })
}

/** Envío multipart (p. ej. la subida de presentaciones). El Content-Type lo pone el navegador. */
export function apiUpload<T>(path: string, form: FormData): Promise<T> {
  return request<T>(path, { method: "POST", bodyRaw: form })
}

