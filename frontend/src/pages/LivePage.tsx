import { useEffect, useRef, useState } from "react"
import { Link, useNavigate, useParams } from "react-router-dom"
import { SlideStage } from "../components/live/SlideStage"
import { Button } from "../components/ui/Button"
import { Icon } from "../components/ui/Icon"
import { getRoom } from "../services/rooms"
import type { Room } from "../types"

const TOTAL_SLIDES = 12 // dato de prueba
const SPECTATORS = 24 // dato de prueba

export default function LivePage() {
  const { id = "" } = useParams()
  const navigate = useNavigate()
  const stageRef = useRef<HTMLDivElement>(null)
  const [room, setRoom] = useState<Room | null | undefined>(undefined)
  const [slide, setSlide] = useState(1)

  useEffect(() => {
    let active = true
    getRoom(id).then((result) => {
      if (active) setRoom(result ?? null)
    })
    return () => {
      active = false
    }
  }, [id])

  if (room === undefined) return <p className="text-ink/60">Cargando…</p>
  if (room === null) {
    return (
      <p className="text-ink/70">
        No encontramos esa sala.{" "}
        <Link to="/salas" className="font-semibold text-brand-500 hover:underline">Volver a mis salas</Link>
      </p>
    )
  }

  const isHost = room.role === "host"
  const leave = () => navigate("/salas")

  return (
    <div className="flex flex-col gap-5">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-3">
          <h1 className="text-2xl font-semibold text-brand-700">{room.title}</h1>
          <span className="rounded-full bg-accent px-3 py-1 text-xs font-bold text-brand-900">EN VIVO</span>
        </div>
        <span className="inline-flex items-center gap-2 text-sm text-ink/70">
          <Icon name="eye" size={16} /> {SPECTATORS} espectadores
        </span>
      </div>

      <div ref={stageRef} className="bg-surface-muted">
        <SlideStage slide={slide} total={TOTAL_SLIDES} canControl={isHost}
          onPrev={() => setSlide((s) => Math.max(1, s - 1))}
          onNext={() => setSlide((s) => Math.min(TOTAL_SLIDES, s + 1))} />
      </div>

      {!isHost && (
        <p className="flex items-center gap-2 rounded-lg bg-brand-50 p-4 text-sm text-brand-700">
          <Icon name="eye" />
          <span><strong>Modo espectador.</strong> Estás viendo la presentación en tiempo real.</span>
        </p>
      )}

      <div className="flex flex-wrap gap-3">
        <Button variant="secondary" icon="expand" onClick={() => stageRef.current?.requestFullscreen?.()}>Pantalla completa</Button>
        <Button variant="secondary" icon="logout" onClick={leave}>Abandonar</Button>
        {isHost && <Button icon="check" onClick={leave}>Finalizar presentación</Button>}
      </div>
    </div>
  )
}
