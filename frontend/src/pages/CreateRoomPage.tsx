import { useState, type FormEvent } from "react"
import { Link, useNavigate } from "react-router-dom"
import { SplitCard } from "../components/layout/SplitCard"
import { Button } from "../components/ui/Button"
import { Field } from "../components/ui/Field"
import { createRoom } from "../services/rooms"

// POST /api/rooms. Reglas del backend: nombre obligatorio (hasta 200 caracteres), URL opcional
// http(s), clave opcional de 4+ caracteres (se guarda hasheado; después ya no se puede leer).
export default function CreateRoomPage() {
  const navigate = useNavigate()
  const [name, setName] = useState("")
  const [url, setUrl] = useState("")
  const [accessKey, setAccessKey] = useState("")
  const [creating, setCreating] = useState(false)
  const [error, setError] = useState("")

  const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    setCreating(true)
    setError("")
    // El anfitrión de la sala es quien la acaba de crear: se lleva ese rol a la página de sala.
    createRoom({ name, url: url.trim() || undefined, accessKey: accessKey || undefined })
      .then((room) => navigate(`/salas/${room.id}`, { state: { role: "host" } }))
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : "No se pudo crear la sala.")
        setCreating(false)
      })
  }

  return (
    <SplitCard
      title="Creá una sala para presentar"
      text="Con el enlace de la sala y la clave, tus invitados entran a ver la presentación en vivo."
    >
      <form onSubmit={handleSubmit} className="flex flex-col gap-5">
        <h2 className="text-xl font-semibold text-brand-700">Nueva sala</h2>
        <Field label="Nombre de la sala" placeholder="Ej. Desarrollo Web" value={name}
          onChange={(event) => setName(event.target.value)} required maxLength={200} />
        <Field label="URL de clase (opcional)" type="url" placeholder="https://moodle.institucion.edu.ar/…" value={url}
          onChange={(event) => setUrl(event.target.value)} />
        <Field label="Clave de ingreso (opcional)" placeholder="Ej. A7K-29P" value={accessKey}
          onChange={(event) => setAccessKey(event.target.value)} minLength={4} maxLength={72} />
        <small className="-mt-3 text-ink/60">
          Cada persona que entre va a necesitar esta clave. Dejalo vacío para una sala abierta.
        </small>
        {error && <p role="alert" className="text-sm font-medium text-red-700">{error}</p>}
        <Button type="submit" disabled={creating}>{creating ? "Creando sala…" : "Crear sala"}</Button>
        <p className="text-sm text-ink/70">
          <Link to="/salas" className="font-semibold text-brand-500 hover:underline">Volver a mis salas</Link>
        </p>
      </form>
    </SplitCard>
  )
}
