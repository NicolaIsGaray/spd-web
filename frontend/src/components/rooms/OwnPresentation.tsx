import { useRef, useState, type ChangeEvent } from "react"
import { Button } from "../ui/Button"
import { Icon } from "../ui/Icon"

// Alinear con el límite que configure el back (variable MAX_UPLOAD_MB).
const MAX_UPLOAD_MB = 100

export function OwnPresentation({ onStart }: { onStart: () => void }) {
  const inputRef = useRef<HTMLInputElement>(null)
  const [file, setFile] = useState<File | null>(null)
  const [error, setError] = useState("")

  const handleChange = (event: ChangeEvent<HTMLInputElement>) => {
    const selected = event.target.files?.[0]
    if (!selected) return
    const validType = /\.(pdf|pptx)$/i.test(selected.name)
    if (!validType) return setError("El archivo tiene que ser PDF o PPTX.")
    if (selected.size > MAX_UPLOAD_MB * 1024 * 1024) return setError(`El archivo supera los ${MAX_UPLOAD_MB} MB.`)
    setError("")
    setFile(selected) // TODO: subir al back cuando se integre
  }

  return (
    <div className="flex flex-col gap-5 rounded-lg border border-border bg-surface p-6">
      <h2 className="text-lg font-semibold text-brand-700">Preparar tu presentación</h2>
      <input ref={inputRef} type="file" accept=".pdf,.pptx" className="hidden" onChange={handleChange} />
      <button onClick={() => inputRef.current?.click()}
        className="flex flex-col items-center gap-2 rounded-lg border-2 border-dashed border-border bg-surface-muted p-10 text-center hover:border-brand-500">
        <span className="flex h-12 w-12 items-center justify-center rounded-full bg-brand-700 text-white">
          <Icon name={file ? "check" : "upload"} />
        </span>
        <strong className="text-brand-700">{file ? file.name : "Subí tu presentación"}</strong>
        <small className="text-ink/60">
          {file ? `${(file.size / 1024 / 1024).toFixed(1)} MB · listo` : `PDF o PPTX · máximo ${MAX_UPLOAD_MB} MB`}
        </small>
      </button>
      {error && <p role="alert" className="text-sm font-medium text-red-700">{error}</p>}
      <p className="flex items-center gap-2 text-sm text-ink/70">
        <Icon name="people" /> Las personas que ingresen van a aparecer acá.
      </p>
      <Button onClick={onStart} icon="play" disabled={!file} className="self-start">Iniciar presentación</Button>
    </div>
  )
}
