import { Link } from "react-router-dom"
import type { Room } from "../../types"
import { Icon } from "../ui/Icon"

export function RoomCard({ room }: { room: Room }) {
  return (
    <Link
      to={`/salas/${room.id}`}
      className="flex flex-col gap-3 rounded-lg border border-border bg-surface p-5 transition-colors hover:border-brand-500"
    >
      <div className="flex items-center justify-between gap-2">
        <strong className="text-lg text-brand-700">{room.title}</strong>
        <span className="rounded-full bg-brand-50 px-2.5 py-0.5 text-xs font-semibold text-brand-700">Activa</span>
      </div>
      <div className="flex items-center justify-between text-sm text-ink/70">
        <span>Creada el {room.createdAt}</span>
        <span className="inline-flex items-center gap-1.5">
          <Icon name="people" size={16} />
          {room.members} integrantes
        </span>
      </div>
      {room.role === "host" && (
        <span className="inline-flex items-center gap-1.5 self-end text-sm font-semibold text-brand-500">
          Modificar <Icon name="edit" size={14} />
        </span>
      )}
    </Link>
  )
}
