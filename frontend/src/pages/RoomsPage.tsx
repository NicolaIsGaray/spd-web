import { useEffect, useState } from "react"
import { useNavigate } from "react-router-dom"
import { RoomCard } from "../components/rooms/RoomCard"
import { Button } from "../components/ui/Button"
import { getRooms } from "../services/rooms"
import type { Role, Room } from "../types"

const TABS: { role: Role; label: string; empty: string }[] = [
  { role: "host", label: "Anfitrión", empty: "Todavía no creaste ninguna sala." },
  { role: "guest", label: "Invitado", empty: "Todavía no te uniste a ninguna sala." },
]

export default function RoomsPage() {
  const navigate = useNavigate()
  const [role, setRole] = useState<Role>("host")
  const [rooms, setRooms] = useState<Room[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    let active = true
    setLoading(true)
    getRooms(role).then((result) => {
      if (!active) return
      setRooms(result)
      setLoading(false)
    })
    return () => {
      active = false
    }
  }, [role])

  const tab = TABS.find((item) => item.role === role)!

  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-wrap items-center justify-between gap-4">
        <h1 className="text-2xl font-semibold text-brand-700">Mis salas</h1>
        {role === "host" ? (
          <Button variant="accent" onClick={() => navigate("/salas/nueva")}>
            Crear sala
          </Button>
        ) : (
          <Button variant="accent" onClick={() => navigate("/unirse")}>
            Unirme a una sala
          </Button>
        )}
      </div>

      <div className="flex gap-6 border-b border-border" role="tablist">
        {TABS.map((item) => (
          <button
            key={item.role}
            role="tab"
            aria-selected={role === item.role}
            onClick={() => setRole(item.role)}
            className={`-mb-px border-b-2 px-1 pb-3 text-sm font-semibold transition-colors ${
              role === item.role
                ? "border-accent text-brand-700"
                : "border-transparent text-ink/60 hover:text-brand-700"
            }`}
          >
            {item.label}
          </button>
        ))}
      </div>

      {loading ? (
        <p className="text-ink/60">Cargando salas…</p>
      ) : rooms.length === 0 ? (
        <p className="rounded-lg border border-dashed border-border bg-surface p-8 text-center text-ink/70">
          {tab.empty}
        </p>
      ) : (
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {rooms.map((room) => (
            <RoomCard key={room.id} room={room} />
          ))}
        </div>
      )}
    </div>
  )
}
