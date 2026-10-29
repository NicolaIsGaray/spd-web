import { useState } from "react"
import { Button } from "../ui/Button"
import { Counter } from "../ui/Counter"

type Props = { onConfirm: (count: number, limit: number) => void; busy?: boolean }

export function SetupGroups({ onConfirm, busy = false }: Props) {
  const [count, setCount] = useState(4)
  const [limit, setLimit] = useState(5)
  return (
    <div className="flex flex-col gap-4 rounded-lg border border-border bg-surface p-6">
      <h2 className="text-lg font-semibold text-brand-700">Armá los grupos</h2>
      <Counter label="Cantidad de grupos" hint="Equipos que van a exponer" value={count} min={2} max={8} onChange={setCount} />
      <Counter label="Límite por grupo" hint="Máximo de integrantes" value={limit} min={2} max={10} onChange={setLimit} />
      <Button onClick={() => onConfirm(count, limit)} icon="check" className="self-start" disabled={busy}>
        {busy ? "Creando grupos…" : "Confirmar ajustes"}
      </Button>
    </div>
  )
}
