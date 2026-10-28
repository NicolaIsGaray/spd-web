export type Role = "host" | "guest"

export type Room = {
  id: string
  title: string
  members: number
  createdAt: string
  role: Role
  accessKey: string
  host: string
}

export type Group = {
  id: number
  members: string[]
  joined: boolean
}
