import { useEffect, useRef, useState, type PointerEvent } from "react"
import { Icon } from "../ui/Icon"

type Props = {
  slide: number
  total: number
  /** Imagen de la diapositiva actual (ruta del servicio presentations vía gateway). */
  url: string
  canControl: boolean
  onPrev: () => void
  onNext: () => void
}

const COLORS = [
  { name: "Azul", value: "#1B3A6B" },
  { name: "Dorado", value: "#C9A227" },
  { name: "Rojo", value: "#C62828" },
  { name: "Negro", value: "#1A2233" },
]
const W = 1280
const H = 720
const tool = "flex h-9 w-9 items-center justify-center rounded-md text-brand-700 hover:bg-brand-50"

// Las anotaciones son locales: solo difunde este cliente la diapositiva que ve el resto.
export function SlideStage({ slide, total, url, canControl, onPrev, onNext }: Props) {
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const drawing = useRef(false)
  const [eraser, setEraser] = useState(false)
  const [color, setColor] = useState(COLORS[0].value)
  const [broken, setBroken] = useState(false)

  // Cambio de diapositiva: resetear el dibujo local y atajar si la imagen no carga.
  useEffect(() => {
    canvasRef.current?.getContext("2d")?.clearRect(0, 0, W, H)
    setBroken(false)
  }, [url])

  // Pre-carga de la siguiente diapositiva: las imágenes son cacheables (immutable), así que el
  // fetch posterior sale del caché y el cambio es instantáneo.
  useEffect(() => {
    if (slide < total) {
      const next = new Image()
      next.src = url.replace(/(\/slides\/)\d+$/, `$1${slide + 1}`)
    }
  }, [url, slide, total])

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
        {url ? (
          broken ? (
            <div className="flex h-full flex-col items-center justify-center gap-2 p-8 text-center">
              <Icon name="file" size={32} />
              <strong className="text-brand-700">No se pudo cargar esta diapositiva</strong>
              <small className="text-ink/60">El archivo quizás no está más disponible; verificá que la sesión siga activa.</small>
            </div>
          ) : (
            <img
              src={url}
              alt={`Diapositiva ${slide} de ${total}`}
              className="h-full w-full object-contain"
              onError={() => setBroken(true)}
            />
          )
        ) : (
          <div className="flex h-full items-center justify-center text-ink/60">Cargando diapositiva…</div>
        )}
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
