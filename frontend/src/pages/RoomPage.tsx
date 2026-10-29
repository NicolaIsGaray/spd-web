import { useEffect, useState } from "react"
import { Link, useLocation, useNavigate, useParams } from "react-router-dom"
import { GroupCard } from "../components/rooms/GroupCard"
import { ModeCard } from "../components/rooms/ModeCard"
import { OwnPresentation } from "../components/rooms/OwnPresentation"
import { PrepareRoom } from "../components/rooms/PrepareRoom"
import { RoomHeader } from "../components/rooms/RoomHeader"
import { SetupGroups } from "../components/rooms/SetupGroups"
import { Button } from "../components/ui/Button"
import { useSession } from "../context/SessionContext"
import { createGroup, getRoom, getRoomGroups, getRoomMembers, joinGroup, leaveGroup } from "../services/rooms"
import { ApiError } from "../services/http"
import type { Group, Member, Role, Room } from "../types"
import { initials } from "../utils/initials"

type Format = "group" | "individual"

export default function RoomPage() {
  const { id = "" } = useParams()
  const navigate = useNavigate()
  const { session } = useSession()
  // Sin login en el back aún, el rol llega por la pantalla de la que venimos (pestaña elegida,
  // creación de sala). Cuando exista autenticación, acá va la sesión real.
  const role: Role = (useLocation().state as { role?: Role } | null)?.role ?? "guest"

  const [room, setRoom] = useState<Room | null | undefined>(undefined)
  const [members, setMembers] = useState<Member[]>([])
  const [groups, setGroups] = useState<Group[]>([])
  const [mode, setMode] = useState<"own" | "watch">("watch")
  const [format, setFormat] = useState<Format>("group")
  const [configured, setConfigured] = useState(false)
  const [settingUp, setSettingUp] = useState(false)
  const [notice, setNotice] = useState("")
  const [presentationId, setPresentationId] = useState<string | null>(null)
  const [preparing, setPreparing] = useState(false)

  // La sala existe y sus integrantes son de solo lectura: se cargan al entrar.
  useEffect(() => {
    let active = true
    setRoom(undefined)
    setGroups([])
    setNotice("")
    getRoom(id)
      .then((result) => {
        if (!active) return
        setRoom(result)
        return getRoomMembers(result.id)
      })
      .then((roomMembers) => {
        if (active && roomMembers) setMembers(roomMembers)
      })
      .catch(() => {
        if (active) setRoom(null)
      })
    return () => {
      active = false
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [id])

  // Los invitados (y los anfitriones ya configurados) ven los grupos que haya creados.
  useEffect(() => {
    if (room == null || role !== "guest") return
    let active = true
    getRoomGroups(room.id, session?.userId ?? null)
      .then((result) => {
        if (active) {
          setGroups(result)
          setConfigured(true) // los grupos ya fueron armados por el anfitrión
        }
      })
      .catch(() => setNotice("No se pudieron cargar los grupos de la sala."))
    return () => {
      active = false
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [room?.id, role])

  const reloadGroups = () => {
    if (!room) return
    getRoomGroups(room.id, session?.userId ?? null)
      .then(setGroups)
      .catch(() => setNotice("No se pudieron recargar los grupos."))
  }

  if (room === undefined) return <p className="text-ink/60">Cargando sala…</p>
  if (room === null) {
    return (
      <p className="text-ink/70">
        No encontramos esa sala.{" "}
        <Link to="/salas" className="font-semibold text-brand-500 hover:underline">Volver a mis salas</Link>
      </p>
    )
  }

  const isHost = role === "host"
  const activeMode = isHost ? mode : "watch"
  const showList = !isHost || configured
  const startLive = () =>
    navigate(`/salas/${room.id}/en-vivo`, { state: { role, presentationId } })
  const inRoom = format === "group" ? groups.reduce((total, g) => total + g.members.length, 0) : members.length

  // El anfitrión arma (n × POST /api/rooms/:id/groups) la grilla real de grupos de la sala.
  const confirmGroups = (count: number, limit: number) => {
    setSettingUp(true)
    setNotice("")
    const creates = Array.from({ length: count }, (_, index) =>
      createGroup(room.id, { limit, priority: index + 1 }),
    )
    Promise.all(creates)
      .then(() => reloadGroups())
      .then(() => setConfigured(true))
      .catch((err: unknown) =>
        setNotice(err instanceof Error ? err.message : "No se pudieron crear los grupos."),
      )
      .finally(() => setSettingUp(false))
  }

  // Integrarse a un grupo hace POST /api/groups/:id/members con el userId de la sesión; si no
  // hay sesión con usuario real, no se puede (y así lo decimos).
  const toggleGroup = (group: Group) => {
    const userId = session?.userId
    if (userId === undefined || !room) {
      setNotice("Necesitás una cuenta (Registrate) o entrar como invitado para unirte a un grupo.")
      return
    }
    setNotice("")
    const change = group.members.some((m) => m.joined)
      ? leaveGroup(group.id, userId)
      : joinGroup(group.id, userId)
    change
      .then(reloadGroups)
      .catch((err: unknown) => {
        if (err instanceof ApiError && err.status === 409) setNotice(err.message)
        else setNotice("No se pudo cambiar de grupo. Volvé a intentar.")
      })
  }

  return (
    <div className="flex flex-col gap-8">
      <RoomHeader title={room.name} hasAccessKey={room.hasAccessKey} members={members.length} />

      {notice && (
        <p role="alert" className="rounded-lg bg-brand-50 p-4 text-sm font-medium text-brand-700">
          {notice}
        </p>
      )}

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
        <OwnPresentation
          onUploaded={setPresentationId}
          onStart={startLive}
        />
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
                <SetupGroups onConfirm={confirmGroups} busy={settingUp} />
              ) : (
                <div className="flex flex-col gap-4 rounded-lg border border-border bg-surface p-6">
                  <h2 className="text-lg font-semibold text-brand-700">Formato individual</h2>
                  <p className="text-ink/70">
                    Cada participante carga su archivo y presenta. {members.length} {members.length === 1 ? "persona" : "personas"} en la sala.
                  </p>
                  {/* TODO: aplicar formato en la sala (el back no guarda el formato todavía) */}
                  <Button icon="check" className="self-start" onClick={() => setConfigured(true)}>Activar formato individual</Button>
                </div>
              )}
            </section>
          )}

          {showList && (
            <section className="flex flex-col gap-5">
              <div className="flex items-center justify-between gap-3">
                <h2 className="text-lg font-semibold text-brand-700">
                  {format === "group" ? `${groups.length} grupos: elegí tu equipo` : `Personas en la sala (${members.length})`}
                </h2>
                <span className="rounded-full bg-brand-50 px-3 py-1 text-sm font-semibold text-brand-700">{inRoom} en sala</span>
              </div>
              {format === "group" ? (
                <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
                  {groups.map((group) => (
                    <GroupCard key={group.id} group={group} onToggle={() => toggleGroup(group)} />
                  ))}
                </div>
              ) : (
                <ul className="flex flex-col gap-2">
                  {members.map((person, index) => (
                    <li key={person.id} className="flex items-center gap-3 rounded-lg border border-border bg-surface p-3">
                      <span className="flex h-8 w-8 items-center justify-center rounded-full bg-brand-50 text-xs font-semibold text-brand-700">{initials(person.fullName)}</span>
                      <strong className="flex-1 text-sm">{person.fullName}</strong>
                      <small className="text-ink/60">{index === 0 ? "Listo para presentar" : "En espera"}</small>
                    </li>
                  ))}
                </ul>
              )}
              {isHost && !preparing && <Button icon="sparkles" className="self-start" onClick={() => setPreparing(true)}>Preparar sala</Button>}
              {isHost && preparing && (
                <PrepareRoom
                  items={format === "individual" ? members.map((m) => m.fullName) : groups.map((g) => `Grupo ${g.id}`)}
                  individual={format === "individual"}
                  onStart={startLive}
                />
              )}
              {!isHost && (
                <div className="flex flex-col gap-3 rounded-lg bg-brand-50 p-4 text-sm text-brand-700">
                  <p><strong>Ya estás dentro.</strong> El anfitrión te va a avisar cuando sea momento de comenzar.</p>
                  {/* TODO: redirigir a la sesión en vivo activa de la sala (el back todavía
                      no tiene endpoint para consultarla) */}
                  {presentationId ? (
                    <Button icon="play" className="self-start" onClick={startLive}>Entrar a la presentación</Button>
                  ) : (
                    <Button icon="play" className="self-start" disabled onClick={startLive}>Entrar a la presentación</Button>
                  )}
                </div>
              )}
            </section>
          )}
        </>
      )}
    </div>
  )
}
