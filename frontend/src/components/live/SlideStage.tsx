import { useRef, useState, type PointerEvent } from "react"
import { Icon } from "../ui/Icon"

type Props = { slide: number; total: number; canControl: boolean; onPrev: () => void; onNext: () => void }

const COLORS = [
  { name: "Azul", value: "#1B3A6B" },
  { name: "Dorado", value: "#C9A227" },
  { name: "Rojo", value: "#C62828" },
  { name: "Negro", value: "#1A2233" },
]
const W = 1280
const H = 720
const tool = "flex h-9 w-9 items-center justify-center rounded-md text-brand-700 hover:bg-brand-50"

// Las anotaciones son locales: el back de referencia solo difunde el número de diapositiva.
export function SlideStage({ slide, total, canControl, onPrev, onNext }: Props) {
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const drawing = useRef(false)
  const [eraser, setEraser] = useState(false)
  const [color, setColor] = useState(COLORS[0].value)

  const ctx = () => canvasRef.current?.getContext("2d") ?? null
  const clear = () => ctx()?.clearRect(0, 0, W, H)
  const point = (e: PointerEvent<HTMLCanvasElement>) => {
    const r = e.currentTarget.getBoundingClientRect()
    return { x: (e.clientX - r.left) * (W / r.width), y: (e.clientY - r.top) * (H / r.height) }
  }

  const start = (e: PointerEvent<HTMLCanvasElement>) => {
    const c = ctx()
    if (!canControl || !c) return
    drawing.current = true
    e.currentTarget.setPointerCapture(e.pointerId)
    const p = point(e)
    c.beginPath()
    c.moveTo(p.x, p.y)
  }
  const move = (e: PointerEvent<HTMLCanvasElement>) => {
    const c = ctx()
    if (!drawing.current || !c) return
    c.globalCompositeOperation = eraser ? "destination-out" : "source-over"
    c.strokeStyle = color
    c.lineWidth = eraser ? 28 : 5
    c.lineCap = "round"
    c.lineJoin = "round"
    const p = point(e)
    c.lineTo(p.x, p.y)
    c.stroke()
  }
  const go = (change: () => void) => {
    clear()
    change()
  }

  return (
    <div className="flex flex-col gap-4">
      <div className="relative aspect-video overflow-hidden rounded-lg border border-border bg-surface">
        {/* Acá va la imagen de la diapositiva que devuelve el back */}
        <div className="flex h-full flex-col items-center justify-center gap-1 text-center">
          <span className="text-ink/50">Diapositiva</span>
          <strong className="text-6xl text-brand-700">{slide}</strong>
          <span className="text-ink/60">de {total}</span>
        </div>
        <canvas ref={canvasRef} width={W} height={H}
          className={`absolute inset-0 h-full w-full touch-none ${canControl ? "cursor-crosshair" : "pointer-events-none"}`}
          onPointerDown={start} onPointerMove={move} onPointerUp={() => (drawing.current = false)} onPointerCancel={() => (drawing.current = false)} />
        {canControl && (
          <>
            <button aria-label="Diapositiva anterior" disabled={slide <= 1} onClick={() => go(onPrev)}
              className="absolute left-3 top-1/2 flex h-10 w-10 -translate-y-1/2 items-center justify-center rounded-full bg-brand-700 text-white hover:bg-brand-900 disabled:opacity-40">
              <Icon name="arrowLeft" />
            </button>
            <button aria-label="Diapositiva siguiente" disabled={slide >= total} onClick={() => go(onNext)}
              className="absolute right-3 top-1/2 flex h-10 w-10 -translate-y-1/2 items-center justify-center rounded-full bg-brand-700 text-white hover:bg-brand-900 disabled:opacity-40">
              <Icon name="arrowRight" />
            </button>
          </>
        )}
      </div>
      {canControl && (
        <div className="mx-auto flex flex-wrap items-center justify-center gap-1 rounded-lg border border-border bg-surface p-2">
          <button aria-label="Pincel" aria-pressed={!eraser} className={`${tool} ${!eraser ? "bg-brand-50" : ""}`} onClick={() => setEraser(false)}><Icon name="brush" /></button>
          <button aria-label="Borrador" aria-pressed={eraser} className={`${tool} ${eraser ? "bg-brand-50" : ""}`} onClick={() => setEraser(true)}><Icon name="eraser" /></button>
          <span className="mx-1 h-6 w-px bg-border" />
          {COLORS.map((item) => (
            <button key={item.value} aria-label={`Color ${item.name}`} aria-pressed={color === item.value}
              onClick={() => { setColor(item.value); setEraser(false) }}
              className={`flex h-9 w-9 items-center justify-center rounded-md hover:bg-brand-50 ${color === item.value && !eraser ? "bg-brand-50" : ""}`}>
              <span className="h-5 w-5 rounded-full border border-border" style={{ background: item.value }} />
            </button>
          ))}
          <span className="mx-1 h-6 w-px bg-border" />
          <button aria-label="Borrar todo" className={tool} onClick={clear}><Icon name="reset" /></button>
        </div>
      )}
    </div>
  )
}
