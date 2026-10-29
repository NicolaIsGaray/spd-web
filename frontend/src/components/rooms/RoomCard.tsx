import { Link } from "react-router-dom"
import type { Role, Room } from "../../types"
import { formatDate } from "../../utils/formatDate"
import { Icon } from "../ui/Icon"

type Props = { room: Room; role?: Role }

export function RoomCard({ room, role }: Props) {
  return (
    <Link
      to={`/salas/${room.id}`}
      state={{ role: role ?? "guest" }}
      className="flex flex-col gap-3 rounded-lg border border-border bg-surface p-5 transition-colors hover:border-brand-500"
    >
      <div className="flex items-center justify-between gap-2">
        <strong className="text-lg text-brand-700">{room.name}</strong>
        {room.hasAccessKey ? (
          <span title="La sala pide clave de ingreso" className="inline-flex items-center gap-1 rounded-full bg-brand-50 px-2.5 py-0.5 text-xs font-semibold text-brand-700">
            <Icon name="lock" size={14} /> Con clave
          </span>
        ) : (
          <span className="rounded-full bg-brand-50 px-2.5 py-0.5 text-xs font-semibold text-brand-700">Abierta</span>
        )}
      </div>
      <div className="flex items-center justify-between text-sm text-ink/70">
        <span>Creada el {formatDate(room.createdAt)}</span>
        {role === "host" && (
          <span className="inline-flex items-center gap-1.5 font-semibold text-brand-500">
            Modificar <Icon name="edit" size={14} />
          </span>
        )}
      </div>
    </Link>
  )
}
