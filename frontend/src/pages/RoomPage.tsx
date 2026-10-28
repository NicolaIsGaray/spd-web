import { useEffect, useState } from "react"
import { Link, useNavigate, useParams } from "react-router-dom"
import { GroupCard } from "../components/rooms/GroupCard"
import { ModeCard } from "../components/rooms/ModeCard"
import { OwnPresentation } from "../components/rooms/OwnPresentation"
import { PrepareRoom } from "../components/rooms/PrepareRoom"
import { RoomHeader } from "../components/rooms/RoomHeader"
import { SetupGroups } from "../components/rooms/SetupGroups"
import { Button } from "../components/ui/Button"
import { getRoom, INITIAL_GROUPS, PARTICIPANTS } from "../services/rooms"
import type { Group, Room } from "../types"
import { initials } from "../utils/initials"

type Format = "group" | "individual"

export default function RoomPage() {
  const { id = "" } = useParams()
  const navigate = useNavigate()
  const [room, setRoom] = useState<Room | null | undefined>(undefined)
  const [mode, setMode] = useState<"own" | "watch">("watch")
  const [format, setFormat] = useState<Format>("group")
  const [configured, setConfigured] = useState(false)
  const [groups, setGroups] = useState<Group[]>(INITIAL_GROUPS)
  const [groupLimit, setGroupLimit] = useState(5)
  const [preparing, setPreparing] = useState(false)

  useEffect(() => {
    let active = true
    getRoom(id).then((result) => {
      if (active) setRoom(result ?? null)
    })
    return () => {
      active = false
    }
  }, [id])

  if (room === undefined) return <p className="text-ink/60">Cargando sala…</p>
  if (room === null) {
    return (
      <p className="text-ink/70">
        No encontramos esa sala.{" "}
        <Link to="/salas" className="font-semibold text-brand-500 hover:underline">Volver a mis salas</Link>
      </p>
    )
  }

  const isHost = room.role === "host"
  const activeMode = isHost ? mode : "watch"
  const showList = !isHost || configured
  const startLive = () => navigate(`/salas/${room.id}/en-vivo`)
  const inRoom = format === "group" ? groups.reduce((total, g) => total + g.members.length, 0) : PARTICIPANTS.length

  const toggleGroup = (groupId: number) =>
    setGroups((items) =>
      items.map((item) =>
        item.id !== groupId
          ? item
          : {
              ...item,
              joined: !item.joined,
              members: item.joined ? item.members.filter((m) => m !== "Tú") : [...item.members, "Tú"],
            },
      ),
    )

  const confirmGroups = (count: number, limit: number) => {
    setGroupLimit(limit)
    setGroups(Array.from({ length: count }, (_, i) => INITIAL_GROUPS[i] ?? { id: i + 1, members: [], joined: false }))
    setConfigured(true)
  }

  return (
    <div className="flex flex-col gap-8">
      <RoomHeader title={room.title} accessKey={room.accessKey} host={room.host} />

      {isHost && (
        <section className="flex flex-col gap-3">
          <h2 className="font-semibold text-brand-700">Modo de presentación</h2>
          <div className="grid gap-3 sm:grid-cols-2">
            <ModeCard active={mode === "own"} icon="file" title="Presentación propia" text="Exponé tu archivo" onClick={() => setMode("own")} />
            <ModeCard active={mode === "watch"} icon="people" title="Ver presentaciones" text="Organizá a tu clase" onClick={() => setMode("watch")} />
          </div>
        </section>
      )}

      {activeMode === "own" ? (
        <OwnPresentation onStart={startLive} />
      ) : (
        <>
          {isHost && !configured && (
            <section className="flex flex-col gap-4">
              <div className="flex gap-6 border-b border-border" role="tablist">
                {(["individual", "group"] as const).map((item) => (
                  <button key={item} role="tab" aria-selected={format === item} onClick={() => setFormat(item)}
                    className={`-mb-px border-b-2 px-1 pb-3 text-sm font-semibold ${
                      format === item ? "border-accent text-brand-700" : "border-transparent text-ink/60 hover:text-brand-700"
                    }`}>
                    {item === "group" ? "En grupo" : "Individual"}
                  </button>
                ))}
              </div>
              {format === "group" ? (
                <SetupGroups onConfirm={confirmGroups} />
              ) : (
                <div className="flex flex-col gap-4 rounded-lg border border-border bg-surface p-6">
                  <h2 className="text-lg font-semibold text-brand-700">Formato individual</h2>
                  <p className="text-ink/70">Cada participante va a poder cargar y presentar su propio archivo.</p>
                  <Button icon="check" className="self-start" onClick={() => setConfigured(true)}>Crear sala individual</Button>
                </div>
              )}
            </section>
          )}

          {showList && (
            <section className="flex flex-col gap-5">
              <div className="flex items-center justify-between gap-3">
                <h2 className="text-lg font-semibold text-brand-700">
                  {format === "group" ? `${groups.length} grupos: elegí tu equipo` : "Personas en la sala"}
                </h2>
                <span className="rounded-full bg-brand-50 px-3 py-1 text-sm font-semibold text-brand-700">{inRoom} en sala</span>
              </div>
              {format === "group" ? (
                <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
                  {groups.map((group) => (
                    <GroupCard key={group.id} group={group} limit={groupLimit} onToggle={() => toggleGroup(group.id)} />
                  ))}
                </div>
              ) : (
                <ul className="flex flex-col gap-2">
                  {PARTICIPANTS.map((person, index) => (
                    <li key={person} className="flex items-center gap-3 rounded-lg border border-border bg-surface p-3">
                      <span className="flex h-8 w-8 items-center justify-center rounded-full bg-brand-50 text-xs font-semibold text-brand-700">{initials(person)}</span>
                      <strong className="flex-1 text-sm">{person}</strong>
                      <small className="text-ink/60">{index === 0 ? "Listo para presentar" : "En espera"}</small>
                    </li>
                  ))}
                </ul>
              )}
              {isHost && !preparing && <Button icon="sparkles" className="self-start" onClick={() => setPreparing(true)}>Preparar sala</Button>}
              {isHost && preparing && (
                <PrepareRoom
                  items={format === "individual" ? PARTICIPANTS : groups.map((g) => `Grupo ${g.id}`)}
                  individual={format === "individual"}
                  onStart={startLive}
                />
              )}
              {!isHost && (
                <div className="flex flex-col gap-3 rounded-lg bg-brand-50 p-4 text-sm text-brand-700">
                  <p><strong>Ya estás dentro.</strong> El anfitrión te va a avisar cuando sea momento de comenzar.</p>
                  {/* TODO: entrar automáticamente cuando el back avise que arrancó la presentación */}
                  <Button icon="play" className="self-start" onClick={startLive}>Entrar a la presentación</Button>
                </div>
              )}
            </section>
          )}
        </>
      )}
    </div>
  )
}
