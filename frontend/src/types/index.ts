// Tipos del dominio. Los que empiezan con "Api" espejan 1:1 el JSON de los servicios
// (backend/services/rooms, /presentations y /realtime): no cambiarles los nombres de campo.
// Los tipos sin prefijo son view models de la UI y se derivan de los anteriores en la capa
// services.

// ─── API: servicio rooms ────────────────────────────────────────────────────────────────────

export type ApiUser = {
  id: number
  full_name: string
  email: string
  roles: string[]
  created_at: string
  updated_at: string
}

export type ApiRoom = {
  id: number
  name: string
  url: string
  /** true: la sala pide clave de ingreso (el hash nunca sale del backend). */
  has_access_key: boolean
  created_at: string
  updated_at: string
}

export type ApiNewRoom = { name: string; url?: string; access_key?: string }

/** Miembro de una sala o de un grupo, visto desde ella (sin correo ni fechas de cuenta). */
export type ApiMember = {
  id: number
  full_name: string
  roles: string[]
  joined_at: string
}

export type ApiRoomMember = { user_id: number; room_id: number; joined_at: string }
export type ApiRoomJoin = { user_id: number; access_key?: string }

export type ApiGroup = {
  id: number
  room_id: number
  /** Cupo: máximo de integrantes (1 o más). */
  limit: number
  /** Prioridad de paso al presentar: 1 presenta primero. */
  priority: number
  created_at: string
  updated_at: string
}

export type ApiGroupFields = { limit?: number; priority?: number }
export type ApiGroupMember = { user_id: number; group_id: number; joined_at: string }

export type ApiPresentation = { presentation_id: string; group_id: number; slides: number[] }

// ─── API: servicio presentations ────────────────────────────────────────────────────────────

export type ApiSlide = {
  /** Número de diapositiva, empezando en 1 (equivale a 001.png). */
  slide: number
  /** Nombre del archivo en disco: 001.png, 002.png... */
  file: string
  /** Ruta pública de la imagen a través del gateway. */
  url: string
}

export type ApiPresentationDetail = {
  presentation_id: string
  slide_count: number
  slides: ApiSlide[]
}

// ─── API: servicio realtime ─────────────────────────────────────────────────────────────────

/** Lo que difunde el WebSocket de /ws/presentation/:id y devuelve el endpoint de control. */
export type ApiSlideState = {
  type: "slide"
  presentation_id: string
  /** Diapositiva actual, empezando en 1. */
  slide: number
  slide_count: number
  /** Ruta pública de la imagen de la diapositiva actual. */
  url: string
}

// ─── View models de la UI ───────────────────────────────────────────────────────────────────

/**
 * Rol de la UI. Sin autenticación en el backend todavía, se deduce de por dónde entra el
 * usuario (pestaña elegida, flujo de creación); la sesión real lo definirá cuando exista.
 */
export type Role = "host" | "guest"

export type Room = {
  id: number
  name: string
  url: string
  hasAccessKey: boolean
  createdAt: string
  updatedAt: string
}

export type Member = {
  id: number
  fullName: string
  roles: string[]
}

export type GroupMember = {
  id: number
  fullName: string
  /** true: la sesión actual es integrante (comparando con el userId de la sesión). */
  joined: boolean
}

export type Group = {
  id: number
  /** Cupo del grupo (viene del backend, ya no lo pasa el padre). */
  limit: number
  /** Prioridad de paso: 1 presenta primero. */
  priority: number
  members: GroupMember[]
}
