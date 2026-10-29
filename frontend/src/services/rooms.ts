import type {
  ApiGroup,
  ApiGroupFields,
  ApiGroupMember,
  ApiMember,
  ApiNewRoom,
  ApiRoom,
  ApiRoomJoin,
  ApiRoomMember,
  ApiPresentation,
  ApiUser,
  Group,
  Member,
  Room,
} from "../types"
import { apiGet, apiSend, apiSendWithStatus } from "./http"

// Dominio "salas" del gateway: usuarios, salas y sus miembros, grupos y sus integrantes, y las
// presentaciones asignadas a cada grupo. Los errores llegan como ApiError (ver http.ts), con el
// mensaje del backend y el código HTTP: 409 conflicto (sala con clave equivocada, grupo lleno...),
// 404 no existe, 422 presentación desconocida, etc.

// ─── Salas ──────────────────────────────────────────────────────────────────────────────────

const toRoom = (api: ApiRoom): Room => ({
  id: api.id,
  name: api.name,
  url: api.url,
  hasAccessKey: api.has_access_key,
  createdAt: api.created_at,
  updatedAt: api.updated_at,
})

const toMember = (api: ApiMember): Member => ({ id: api.id, fullName: api.full_name, roles: api.roles })

/** Todas las salas, ordenadas por id. GET /api/rooms */
export async function getRooms(): Promise<Room[]> {
  const rooms = await apiGet<ApiRoom[]>("/rooms")
  return rooms.map(toRoom)
}

/** Salas en las que es miembro un usuario. GET /api/users/:id/rooms */
export async function getUserRooms(userId: number): Promise<Room[]> {
  const rooms = await apiGet<ApiRoom[]>(`/users/${userId}/rooms`)
  return rooms.map(toRoom)
}

/** Una sala. GET /api/rooms/:id. Rechaza con ApiError si no existe. */
export function getRoom(id: number | string): Promise<Room> {
  return apiGet<ApiRoom>(`/rooms/${id}`).then(toRoom)
}

/** Crea una sala: sin clave de ingreso si accessKey queda vacía. POST /api/rooms */
export function createRoom(input: { name: string; url?: string; accessKey?: string }): Promise<Room> {
  const body: ApiNewRoom = {
    name: input.name,
    ...(input.url ? { url: input.url } : {}),
    ...(input.accessKey ? { access_key: input.accessKey } : {}),
  }
  return apiSend<ApiRoom>("/rooms", "POST", body).then(toRoom)
}

/** Integrantes de una sala. GET /api/rooms/:id/members */
export async function getRoomMembers(roomId: number): Promise<Member[]> {
  const members = await apiGet<ApiMember[]>(`/rooms/${roomId}/members`)
  return members.map(toMember)
}

/** Entra a una sala. Devuelve true si es nuevo miembro (201), false si ya estaba (200).
 * POST /api/rooms/:id/members */
export async function joinRoom(roomId: number, input: { userId: number; accessKey?: string }): Promise<boolean> {
  const body: ApiRoomJoin = { user_id: input.userId, ...(input.accessKey ? { access_key: input.accessKey } : {}) }
  const { status } = await apiSendWithStatus<ApiRoomMember>(`/rooms/${roomId}/members`, "POST", body)
  return status === 201
}

// ─── Grupos ─────────────────────────────────────────────────────────────────────────────────

/** Grupos de una sala con sus integrantes (una llamada por grupo más una global con los
 * nombres). GET /api/rooms/:id/groups */
export async function getRoomGroups(roomId: number, sessionUserId: number | null): Promise<Group[]> {
  // Los integrantes de grupo llegan solo con user_id: los nombres salen de los miembros de la sala.
  const groups = await apiGet<ApiGroup[]>(`/rooms/${roomId}/groups`)
  if (groups.length === 0) return []
  const members = await getRoomMembers(roomId)
  const names = new Map(members.map((m) => [m.id, m.fullName]))
  return Promise.all(
    groups.map(async (group) => {
      const groupMembers = await getGroupMembers(group.id)
      return {
        id: group.id,
        limit: group.limit,
        priority: group.priority,
        members: groupMembers.map((m) => ({
          id: m.user_id,
          fullName: names.get(m.user_id) ?? `Usuario ${m.user_id}`,
          joined: sessionUserId !== null && m.user_id === sessionUserId,
        })),
      }
    }),
  )
}

/** Crea un grupo en la sala. POST /api/rooms/:id/groups */
export function createGroup(roomId: number, fields: { limit?: number; priority?: number }): Promise<ApiGroup> {
  const body: ApiGroupFields = {
    ...(fields.limit !== undefined ? { limit: fields.limit } : {}),
    ...(fields.priority !== undefined ? { priority: fields.priority } : {}),
  }
  return apiSend<ApiGroup>(`/rooms/${roomId}/groups`, "POST", body)
}

/** Integrantes de un grupo. GET /api/groups/:id/members */
export function getGroupMembers(groupId: number): Promise<ApiGroupMember[]> {
  return apiGet<ApiGroupMember[]>(`/groups/${groupId}/members`)
}

/** Entra a un grupo. Devuelve true si se sumó ahora (201), false si ya estaba (200).
 * POST /api/groups/:id/members */
export async function joinGroup(groupId: number, userId: number): Promise<boolean> {
  const { status } = await apiSendWithStatus<ApiGroupMember>(`/groups/${groupId}/members`, "POST", { user_id: userId })
  return status === 201
}

/** Sale de un grupo. DELETE /api/groups/:id/members/:user_id */
export function leaveGroup(groupId: number, userId: number): Promise<void> {
  return apiSend<void>(`/groups/${groupId}/members/${userId}`, "DELETE")
}

/** Presentaciones asignadas a un grupo. GET /api/groups/:id/presentations */
export function getGroupPresentations(groupId: number): Promise<ApiPresentation[]> {
  return apiGet<ApiPresentation[]>(`/groups/${groupId}/presentations`)
}

// ─── Usuarios ───────────────────────────────────────────────────────────────────────────────

/** Crea un usuario. POST /api/users. 409 si el email ya está registrado. */
export function createUser(input: {
  fullName: string
  email: string
  password: string
  roles?: string[]
}): Promise<ApiUser> {
  return apiSend<ApiUser>("/users", "POST", {
    full_name: input.fullName,
    email: input.email,
    password: input.password,
    ...(input.roles ? { roles: input.roles } : {}),
  })
}

/** Un usuario. GET /api/users/:id */
export function getUser(id: number): Promise<ApiUser> {
  return apiGet<ApiUser>(`/users/${id}`)
}
