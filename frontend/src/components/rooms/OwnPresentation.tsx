import { useRef, useState, type ChangeEvent } from "react"
import { Icon } from "../ui/Icon"
import { Button } from "../ui/Button"
import { uploadPresentation, MAX_UPLOAD_MB } from "../../services/live"

type Props = {
  /** Avisa el id que asignó el back (POST /api/presentations/upload). */
  onUploaded: (presentationId: string) => void
  onStart: () => void
}

export function OwnPresentation({ onUploaded, onStart }: Props) {
  const inputRef = useRef<HTMLInputElement>(null)
  const [file, setFile] = useState<File | null>(null)
  const [status, setStatus] = useState<"idle" | "uploading" | "done">("idle")
  const [error, setError] = useState("")

  const handleChange = (event: ChangeEvent<HTMLInputElement>) => {
    const selected = event.target.files?.[0]
    if (!selected) return
    const validType = /\.(pdf|pptx)$/i.test(selected.name)
    if (!validType) return setError("El archivo tiene que ser PDF o PPTX.")
    if (selected.size > MAX_UPLOAD_MB * 1024 * 1024) {
      return setError(`El archivo supera los ${MAX_UPLOAD_MB} MB.`)
    }
    setError("")
    setFile(selected)
    setStatus("uploading")
    // POST /api/presentations/upload: el back convierte a una imagen PNG por diapositiva
    // (LibreOffice + pdftoppm) y responde metadatos con el id para el modo en vivo.
    uploadPresentation(selected)
      .then((presentation) => {
        setStatus("done")
        onUploaded(presentation.presentation_id)
      })
      .catch((err: unknown) => {
        setStatus("idle")
        setError(err instanceof Error ? err.message : "No se pudo subir la presentación.")
      })
  }

  const uploading = status === "uploading"

  return (
    <div className="flex flex-col gap-5 rounded-lg border border-border bg-surface p-6">
      <h2 className="text-lg font-semibold text-brand-700">Preparar tu presentación</h2>
      <input ref={inputRef} type="file" accept=".pdf,.pptx" className="hidden" onChange={handleChange} />
      <button
        onClick={() => inputRef.current?.click()}
        disabled={uploading}
        className="flex flex-col items-center gap-2 rounded-lg border-2 border-dashed border-border bg-surface-muted p-10 text-center hover:border-brand-500 disabled:opacity-60"
      >
        <span className="flex h-12 w-12 items-center justify-center rounded-full bg-brand-700 text-white">
          <Icon name={status === "done" ? "check" : "upload"} />
        </span>
        <strong className="text-brand-700">
          {file ? file.name : "Subí tu presentación"}
        </strong>
        <small className="text-ink/60">
          {file === null
            ? `PDF o PPTX · máximo ${MAX_UPLOAD_MB} MB`
            : uploading
              ? "Convirtiendo el archivo en diapositivas…"
              : status === "done"
                ? `${(file.size / (1024 * 1024)).toFixed(1)} MB · lista para presentar`
                : "No se pudo convertir: elegí el archivo de vuelta"}
        </small>
      </button>
      {error && (
        <p role="alert" className="text-sm font-medium text-red-700">
          {error}
          {error.includes("convertidores") && " · Avisale al anfitrión que el servicio de conversión no está disponible."}
        </p>
      )}
      <p className="flex items-center gap-2 text-sm text-ink/70">
        <Icon name="people" /> Las personas que ingresen van a ver lo mismo que vos, en tiempo real.
      </p>
      <Button onClick={onStart} icon="play" disabled={status !== "done"} className="self-start">
        Iniciar presentación
      </Button>
    </div>
  )
}
