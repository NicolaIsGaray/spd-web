import { useRef, useState } from "react"
import type { Group } from "../../types"
import { initials } from "../../utils/initials"
import { Button } from "../ui/Button"

type Props = { group: Group; limit: number; onToggle: () => void }

export function GroupCard({ group, limit, onToggle }: Props) {
  const inputRef = useRef<HTMLInputElement>(null)
  const [fileName, setFileName] = useState("")
  const full = !group.joined && group.members.length >= limit

  return (
    <article className={`flex flex-col gap-4 rounded-lg border bg-surface p-5 ${group.joined ? "border-brand-700" : "border-border"}`}>
      <div className="flex items-baseline justify-between">
        <strong className="text-brand-700">Grupo {String(group.id).padStart(2, "0")}</strong>
        <small className="text-ink/60">{group.members.length}/{limit}</small>
      </div>
      <div className="flex min-h-16 flex-col gap-2">
        {group.members.length ? (
          group.members.map((member, index) => (
            <div key={member} className="flex items-center gap-2 text-sm">
              <span className="flex h-7 w-7 items-center justify-center rounded-full bg-brand-50 text-xs font-semibold text-brand-700">
                {initials(member)}
              </span>
              <span>{member}</span>
              {index === 0 && <small className="text-ink/60">· Encargado</small>}
            </div>
          ))
        ) : (
          <p className="text-sm text-ink/60">Todavía no hay integrantes</p>
        )}
      </div>
      <div className="flex flex-wrap gap-2">
        <Button variant={group.joined ? "ghost" : "secondary"} onClick={onToggle} disabled={full}>
          {group.joined ? "Abandonar" : full ? "Completo" : "Unirme"}
        </Button>
        {group.joined && (
          <>
            <input ref={inputRef} type="file" accept=".pdf,.pptx" className="hidden"
              onChange={(event) => setFileName(event.target.files?.[0]?.name ?? "")} />
            <Button variant="ghost" icon="upload" onClick={() => inputRef.current?.click()}>
              {fileName || "Cargar presentación"}
            </Button>
          </>
        )}
      </div>
    </article>
  )
}
