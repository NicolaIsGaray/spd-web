import { useRef, useState } from "react"
import type { Group } from "../../types"
import { initials } from "../../utils/initials"
import { Button } from "../ui/Button"

type Props = { group: Group; onToggle: () => void }

export function GroupCard({ group, onToggle }: Props) {
  const inputRef = useRef<HTMLInputElement>(null)
  const [fileName, setFileName] = useState("")
  // Integrante de la sesión: lo marca la carga de datos (user_id de la sesión dentro del grupo).
  const joined = group.members.some((member) => member.joined)
  const full = !joined && group.members.length >= group.limit

  return (
    <article className={`flex flex-col gap-4 rounded-lg border bg-surface p-5 ${joined ? "border-brand-700" : "border-border"}`}>
      <div className="flex items-baseline justify-between">
        <strong className="text-brand-700">Grupo {String(group.priority).padStart(2, "0")}</strong>
        <small className="text-ink/60">{group.members.length}/{group.limit}</small>
      </div>
      <div className="flex min-h-16 flex-col gap-2">
        {group.members.length ? (
          group.members.map((member, index) => (
            <div key={member.id} className="flex items-center gap-2 text-sm">
              <span className="flex h-7 w-7 items-center justify-center rounded-full bg-brand-50 text-xs font-semibold text-brand-700">
                {initials(member.fullName)}
              </span>
              <span>{member.fullName}</span>
              {index === 0 && <small className="text-ink/60">· Encargado</small>}
            </div>
          ))
        ) : (
          <p className="text-sm text-ink/60">Todavía no hay integrantes</p>
        )}
      </div>
      <div className="flex flex-wrap gap-2">
        <Button variant={joined ? "ghost" : "secondary"} onClick={onToggle} disabled={full}>
          {joined ? "Abandonar" : full ? "Completo" : "Unirme"}
        </Button>
        {joined && (
          <>
            <input ref={inputRef} type="file" accept=".pdf,.pptx" className="hidden"
              onChange={(event) => setFileName(event.target.files?.[0]?.name ?? "")} />
            <Button variant="ghost" icon="upload" onClick={() => inputRef.current?.click()}>
              {fileName || "Cargar presentación"}
            </Button>
          </>
        )}
      </div>
      {fileName && (
        <p className="text-sm text-ink/70">
          Listo para presentar <strong className="text-brand-700">{fileName}</strong>
          {/* TODO: subir al back y asignarla al grupo (POST /api/presentations/upload y luego
              POST /api/groups/:id/presentations) cuando el flujo de grupos se integre */}
        </p>
      )}
    </article>
  )
}
