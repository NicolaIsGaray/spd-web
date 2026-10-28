import { useState } from "react"
import { initials } from "../../utils/initials"
import { Button } from "../ui/Button"
import { Icon, type IconName } from "../ui/Icon"

type Props = { items: string[]; individual: boolean; onStart: () => void }
type OrderMode = "ordered" | "random"

const sequence = (length: number) => Array.from({ length }, (_, index) => index + 1)

function shuffle(values: number[]) {
  const result = [...values]
  for (let i = result.length - 1; i > 0; i--) {
    const j = Math.floor(Math.random() * (i + 1))
    ;[result[i], result[j]] = [result[j], result[i]]
  }
  return result
}

export function PrepareRoom({ items, individual, onStart }: Props) {
  const [orderMode, setOrderMode] = useState<OrderMode>("ordered")
  const [order, setOrder] = useState(() => sequence(items.length))

  const options: { mode: OrderMode; icon: IconName; title: string; text: string }[] = [
    { mode: "ordered", icon: "list", title: "En orden", text: "Vos decidís" },
    { mode: "random", icon: "dice", title: "Al azar", text: "Que decida la suerte" },
  ]

  return (
    <div className="flex flex-col gap-5 rounded-lg border border-border bg-surface p-6">
      <h2 className="text-lg font-semibold text-brand-700">Orden de presentación</h2>
      <div className="grid gap-3 sm:grid-cols-2">
        {options.map((option) => (
          <button key={option.mode} aria-pressed={orderMode === option.mode}
            onClick={() => {
              setOrderMode(option.mode)
              if (option.mode === "ordered") setOrder(sequence(items.length))
            }}
            className={`flex items-center gap-3 rounded-lg border p-4 text-left ${
              orderMode === option.mode ? "border-brand-700 bg-brand-50" : "border-border hover:border-brand-500"
            }`}>
            <Icon name={option.icon} />
            <span>
              <strong className="block text-brand-700">{option.title}</strong>
              <small className="text-ink/60">{option.text}</small>
            </span>
          </button>
        ))}
      </div>
      <ol className="flex flex-col gap-2">
        {items.map((item, index) => (
          <li key={item} className="flex items-center gap-3 rounded-md bg-surface-muted p-3 text-sm">
            <span className="flex h-8 w-8 items-center justify-center rounded-md bg-brand-700 text-xs font-semibold text-white">
              {individual ? initials(item) : `G${index + 1}`}
            </span>
            <strong className="flex-1">{individual ? item : `Grupo ${String(index + 1).padStart(2, "0")}`}</strong>
            <span className="font-semibold text-brand-700">{order[index]}°</span>
          </li>
        ))}
      </ol>
      <div className="flex flex-wrap gap-3">
        {orderMode === "random" && (
          <Button variant="secondary" icon="dice" onClick={() => setOrder((current) => shuffle(current))}>Sortear</Button>
        )}
        <Button variant="accent" icon="play" onClick={onStart}>Iniciar presentaciones</Button>
      </div>
    </div>
  )
}
