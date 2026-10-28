import type { Group, Role, Room } from "../types"

// Datos de prueba (tomados del boceto). Reemplazar por llamadas al gateway.
const KEY = "A7K-29P"
const HOST = "Mario Castañeda"
const MOCK_ROOMS: Room[] = [
  { id: "1", title: "Desarrollo Web", members: 32, createdAt: "21/06/2026", role: "host", accessKey: KEY, host: HOST },
  { id: "2", title: "Técnicas Laborales", members: 30, createdAt: "20/06/2026", role: "host", accessKey: KEY, host: HOST },
  { id: "3", title: "Análisis de Datos", members: 23, createdAt: "18/06/2026", role: "host", accessKey: KEY, host: HOST },
  { id: "4", title: "Arquitectura de Software", members: 15, createdAt: "25/06/2026", role: "guest", accessKey: KEY, host: HOST },
  { id: "5", title: "Base de Datos Avanzada", members: 20, createdAt: "22/06/2026", role: "guest", accessKey: KEY, host: HOST },
]

export const INITIAL_GROUPS: Group[] = [
  { id: 1, members: ["Valentina R.", "Lucas M."], joined: false },
  { id: 2, members: ["Camila D."], joined: false },
  { id: 3, members: [], joined: false },
  { id: 4, members: ["Mateo S.", "Emma C."], joined: false },
]

export const PARTICIPANTS = ["Valentina Ruiz", "Lucas Medina", "Camila Díaz", "Mateo Silva"]

const delay = () => new Promise((resolve) => setTimeout(resolve, 250))

export async function getRooms(role: Role): Promise<Room[]> {
  await delay()
  return MOCK_ROOMS.filter((room) => room.role === role)
}

export async function getRoom(id: string): Promise<Room | undefined> {
  await delay()
  if (id === "nueva") {
    return { id, title: "Nueva sala", members: 0, createdAt: "", role: "host", accessKey: KEY, host: "Vos" }
  }
  return MOCK_ROOMS.find((room) => room.id === id)
}
