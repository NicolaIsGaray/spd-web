import { useState } from "react"
import { Icon } from "../ui/Icon"

type Copied = "link" | null

type Props = {
  title: string
  /** true si la sala pide clave de ingreso (el hash nunca sale del backend). */
  hasAccessKey: boolean
  /** Cuántos integrantes tiene la sala, si ya se conocen. */
  members?: number
}

export function RoomHeader({ title, hasAccessKey, members }: Props) {
  const [copied, setCopied] = useState<Copied>(null)
  // El enlace de invitación apunta al flujo de invitado: ahí se entra con la clave de la sala.
  const invitationLink = `${window.location.origin}/unirse`

  const copy = async (value: string) => {
    try {
      await navigator.clipboard.writeText(value)
    } catch {
      // sin permiso de portapapeles: igual mostramos el estado
    }
    setCopied("link")
    window.setTimeout(() => setCopied(null), 2000)
  }

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold text-brand-700">{title}</h1>
          <p className="text-sm text-ink/60">
            {members !== undefined && <>
              {members} {members === 1 ? "integrante" : "integrantes"}
            </>}
          </p>
        </div>
        <span className="rounded-full bg-brand-50 px-3 py-1 text-sm font-semibold text-brand-700">Activa</span>
      </div>
      <div className="grid gap-3 sm:grid-cols-2">
        <button
          onClick={() => copy(invitationLink)}
          className="flex items-center justify-between gap-3 rounded-lg border border-border bg-surface p-4 text-left hover:border-brand-500"
        >
          <span className="min-w-0">
            <small className="block text-ink/60">Enlace de invitación</small>
            <strong className="block truncate text-brand-700">
              {copied ? "¡Copiado!" : invitationLink}
            </strong>
          </span>
          <Icon name={copied ? "check" : "link"} />
        </button>
        <div className="flex items-center justify-between gap-3 rounded-lg border border-border bg-surface p-4">
          <span className="min-w-0">
            <small className="block text-ink/60">Clave de sala</small>
            <strong className="block truncate text-brand-700">
              {hasAccessKey ? "Protegida con clave de ingreso" : "Abierta: no pide clave"}
            </strong>
          </span>
          <Icon name={hasAccessKey ? "lock" : "people"} />
        </div>
      </div>
    </div>
  )
}
