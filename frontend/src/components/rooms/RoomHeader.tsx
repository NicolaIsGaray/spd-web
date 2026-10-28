import { useState } from "react"
import { Icon } from "../ui/Icon"

type Copied = "link" | "key" | null

export function RoomHeader({ title, accessKey, host }: { title: string; accessKey: string; host: string }) {
  const [copied, setCopied] = useState<Copied>(null)

  const copy = async (value: string, which: Exclude<Copied, null>) => {
    try {
      await navigator.clipboard.writeText(value)
    } catch {
      // sin permiso de portapapeles: igual mostramos el estado
    }
    setCopied(which)
    window.setTimeout(() => setCopied(null), 2000)
  }

  const items = [
    { which: "link" as const, label: "Enlace de invitación", value: `${window.location.origin}/unirse` },
    { which: "key" as const, label: "Clave de sala", value: accessKey },
  ]

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold text-brand-700">{title}</h1>
          <p className="text-sm text-ink/60">Organizada por <strong className="text-ink">{host}</strong></p>
        </div>
        <span className="rounded-full bg-brand-50 px-3 py-1 text-sm font-semibold text-brand-700">Activa</span>
      </div>
      <div className="grid gap-3 sm:grid-cols-2">
        {items.map((item) => (
          <button key={item.which} onClick={() => copy(item.value, item.which)}
            className="flex items-center justify-between gap-3 rounded-lg border border-border bg-surface p-4 text-left hover:border-brand-500">
            <span className="min-w-0">
              <small className="block text-ink/60">{item.label}</small>
              <strong className="block truncate text-brand-700">
                {copied === item.which ? "¡Copiado!" : item.value}
              </strong>
            </span>
            <Icon name={copied === item.which ? "check" : "copy"} />
          </button>
        ))}
      </div>
    </div>
  )
}
