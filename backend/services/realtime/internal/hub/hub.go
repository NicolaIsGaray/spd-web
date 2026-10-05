// Package hub implementa el patrón Hub: mantiene las sesiones de presentación en vivo (la
// diapositiva actual de cada una), registra y da de baja a los visores, y les difunde cada
// cambio de diapositiva.
//
// # Modelo de concurrencia
//
// Todo el estado (mapa de sesiones, visores de cada sesión, diapositiva actual) es propiedad
// EXCLUSIVA de la goroutine que ejecuta Run. El resto del programa nunca lo toca: envía
// peticiones por canales (register, unregister, commands) y, si necesita respuesta, la recibe
// por un canal propio con buffer. Consecuencias:
//
//   - No hay memoria compartida: no hay data races ni mutex que olvidar.
//   - Los eventos se procesan de uno en uno, con orden total: un visor que se une recibe el
//     estado actual ANTES que cualquier cambio posterior.
//   - Run nunca hace I/O ni se bloquea: las consultas de red (catálogo) se hacen fuera del
//     bucle, y la entrega a cada visor es un depósito no bloqueante en su buzón (Client.offer).
package hub

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

var (
	// ErrPresentationNotFound indica que la presentación no existe en el catálogo.
	ErrPresentationNotFound = errors.New("presentación no encontrada")
	// ErrSlideOutOfRange indica que el número de diapositiva pedido no existe.
	ErrSlideOutOfRange = errors.New("diapositiva fuera de rango")
	// ErrUnknownAction indica un comando con una acción no soportada.
	ErrUnknownAction = errors.New("acción desconocida")
	// ErrClientReused indica que el Client ya se registró antes (cada Client se registra una vez).
	ErrClientReused = errors.New("el cliente ya fue registrado")
	// ErrClosed indica que el Hub se detuvo (apagado del servicio).
	ErrClosed = errors.New("hub detenido")

	// errNotLoaded (interno): la sesión no está en memoria y hay que consultar el catálogo.
	errNotLoaded = errors.New("sesión no cargada")
)

// Catalog da acceso a los metadatos de las presentaciones. Lo implementa el cliente HTTP del
// servicio de presentaciones y debe devolver ErrPresentationNotFound si el id no existe.
type Catalog interface {
	SlideCount(ctx context.Context, presentationID string) (int, error)
}

// Options configura el Hub.
type Options struct {
	// IdleTTL es el tiempo que una sesión SIN visores conserva su estado en memoria antes de
	// liberarse. 0 desactiva la liberación.
	IdleTTL time.Duration
	Logger  *slog.Logger
}

// Hub coordina las sesiones en vivo. Se crea con New y se arranca con Run en su propia goroutine.
type Hub struct {
	catalog Catalog
	idleTTL time.Duration
	log     *slog.Logger

	// Canales SIN buffer a propósito: un envío solo se completa cuando Run lo recibe, de modo
	// que si una goroutine hace Register y después Unregister, Run los procesa en ese orden.
	register   chan registration
	unregister chan *Client
	commands   chan commandRequest
	done       chan struct{} // se cierra cuando Run termina

	// sessions es propiedad EXCLUSIVA de la goroutine Run: ningún otro código lo lee ni escribe.
	sessions map[string]*session
}

type registration struct {
	presentationID string
	client         *Client
	slideCount     int        // > 0 si el llamador ya consultó el catálogo (permite crear la sesión)
	reply          chan error // buffer 1: Run responde sin bloquearse jamás
}

type commandRequest struct {
	presentationID string
	cmd            Command
	slideCount     int
	reply          chan commandResult // buffer 1
}

type commandResult struct {
	state SlideState
	err   error
}

// New crea un Hub. No procesa nada hasta que se llama a Run.
func New(catalog Catalog, opts Options) *Hub {
	log := opts.Logger
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &Hub{
		catalog:    catalog,
		idleTTL:    opts.IdleTTL,
		log:        log,
		register:   make(chan registration),
		unregister: make(chan *Client),
		commands:   make(chan commandRequest),
		done:       make(chan struct{}),
		sessions:   make(map[string]*session),
	}
}

// Run es el bucle del Hub: procesa de una en una las altas, bajas y comandos, y libera
// periódicamente las sesiones inactivas. Bloquea hasta que ctx se cancela; entonces cierra el
// buzón de todos los visores (que reciben un cierre ordenado) y retorna. Llamar una sola vez.
func (h *Hub) Run(ctx context.Context) {
	defer close(h.done)

	var sweep <-chan time.Time // nil (nunca dispara) si la liberación está desactivada
	if h.idleTTL > 0 {
		ticker := time.NewTicker(sweepInterval(h.idleTTL))
		defer ticker.Stop()
		sweep = ticker.C
	}

	for {
		select {
		case req := <-h.register:
			req.reply <- h.handleRegister(req)
		case c := <-h.unregister:
			h.handleUnregister(c)
		case req := <-h.commands:
			state, err := h.handleCommand(req)
			req.reply <- commandResult{state: state, err: err}
		case now := <-sweep:
			h.evictIdle(now)
		case <-ctx.Done():
			h.closeAll()
			return
		}
	}
}

// Done devuelve un canal que se cierra cuando Run ha terminado.
func (h *Hub) Done() <-chan struct{} { return h.done }

// Register une un visor a la sesión de presentationID; si la sesión no está en memoria, la
// crea consultando el catálogo. Nada más registrarse, el visor recibe en su buzón la
// diapositiva actual. Si devuelve nil, el llamador debe llamar a Unregister al terminar.
func (h *Hub) Register(ctx context.Context, presentationID string, c *Client) error {
	return h.withCatalog(ctx, presentationID, func(slideCount int) error {
		req := registration{
			presentationID: presentationID,
			client:         c,
			slideCount:     slideCount,
			reply:          make(chan error, 1),
		}
		if err := send(ctx, h, h.register, req); err != nil {
			return err
		}
		// Run procesa cada petición recibida en el acto y siempre responde, así que esta espera
		// es breve. No se escucha ctx a propósito: abandonar la espera después de un registro ya
		// aplicado dejaría un visor huérfano, sin su Unregister.
		return <-req.reply
	})
}

// Unregister da de baja al visor y cierra su buzón. Es idempotente y no bloquea si el Hub ya
// se detuvo (en ese caso el buzón ya está cerrado).
func (h *Hub) Unregister(c *Client) {
	select {
	case h.unregister <- c:
	case <-h.done:
	}
}

// Control aplica un comando del presentador y, si la diapositiva cambia, la difunde a todos
// los visores de esa presentación. Devuelve el estado resultante.
func (h *Hub) Control(ctx context.Context, presentationID string, cmd Command) (SlideState, error) {
	var res commandResult
	err := h.withCatalog(ctx, presentationID, func(slideCount int) error {
		req := commandRequest{
			presentationID: presentationID,
			cmd:            cmd,
			slideCount:     slideCount,
			reply:          make(chan commandResult, 1),
		}
		if err := send(ctx, h, h.commands, req); err != nil {
			return err
		}
		res = <-req.reply // ver el comentario equivalente en Register
		return res.err
	})
	return res.state, err
}

// withCatalog ejecuta try contra el bucle del Hub. Si la sesión aún no está en memoria
// (errNotLoaded), consulta el catálogo FUERA del bucle (es I/O de red: hacerlo dentro de Run
// congelaría todas las sesiones) y reintenta una única vez con el número de diapositivas para
// que Run pueda crearla. Si otra goroutine la creó entretanto, Run reutiliza la existente.
func (h *Hub) withCatalog(ctx context.Context, presentationID string, try func(slideCount int) error) error {
	err := try(0)
	if !errors.Is(err, errNotLoaded) {
		return err
	}
	n, err := h.catalog.SlideCount(ctx, presentationID)
	if err != nil {
		return err
	}
	if n < 1 {
		return fmt.Errorf("catálogo: la presentación %s no tiene diapositivas", presentationID)
	}
	return try(n)
}

// send entrega una petición al bucle Run sin quedarse bloqueado para siempre: se rinde si el
// llamador cancela ctx o si el Hub ya se detuvo.
func send[T any](ctx context.Context, h *Hub, ch chan<- T, req T) error {
	select {
	case ch <- req:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-h.done:
		return ErrClosed
	}
}

// ---------------------------------------------------------------------------------------
// A partir de aquí, todo se ejecuta EXCLUSIVAMENTE en la goroutine Run.
// ---------------------------------------------------------------------------------------

// sessionFor devuelve la sesión en memoria, o la crea si el llamador aportó slideCount.
func (h *Hub) sessionFor(id string, slideCount int) (*session, error) {
	if s, ok := h.sessions[id]; ok {
		return s, nil
	}
	if slideCount < 1 {
		return nil, errNotLoaded
	}
	s := newSession(id, slideCount, time.Now())
	h.sessions[id] = s
	h.log.Debug("sesión creada", "presentation_id", id, "slide_count", slideCount)
	return s, nil
}

func (h *Hub) handleRegister(req registration) error {
	if req.client.state != clientNew {
		return ErrClientReused
	}
	s, err := h.sessionFor(req.presentationID, req.slideCount)
	if err != nil {
		return err
	}
	req.client.state = clientActive
	req.client.sessionID = s.id
	s.clients[req.client] = struct{}{}
	s.lastActive = time.Now()

	// Estado inicial: el visor recibe la diapositiva actual nada más unirse. Al encolarlo aquí,
	// dentro de Run, ningún broadcast posterior puede adelantarse a este mensaje.
	req.client.offer(s.payload)
	return nil
}

func (h *Hub) handleUnregister(c *Client) {
	if c.state != clientActive {
		return // nunca registrado o ya cerrado: Unregister es idempotente
	}
	if s, ok := h.sessions[c.sessionID]; ok {
		delete(s.clients, c)
		s.lastActive = time.Now()
	}
	c.close()
}

func (h *Hub) handleCommand(req commandRequest) (SlideState, error) {
	s, err := h.sessionFor(req.presentationID, req.slideCount)
	if err != nil {
		return SlideState{}, err
	}
	s.lastActive = time.Now()
	changed, err := s.apply(req.cmd)
	if err != nil {
		return SlideState{}, err
	}
	if changed {
		s.broadcast()
	}
	return s.state(), nil
}

// evictIdle libera las sesiones sin visores que llevan más de idleTTL sin actividad. Mientras
// quede un solo visor, la sesión se conserva aunque el presentador no haga nada.
func (h *Hub) evictIdle(now time.Time) {
	for id, s := range h.sessions {
		if len(s.clients) == 0 && now.Sub(s.lastActive) >= h.idleTTL {
			delete(h.sessions, id)
			h.log.Debug("sesión inactiva liberada", "presentation_id", id)
		}
	}
}

// closeAll cierra el buzón de todos los visores al apagar el Hub.
func (h *Hub) closeAll() {
	viewers := 0
	for id, s := range h.sessions {
		for c := range s.clients {
			c.close()
			viewers++
		}
		delete(h.sessions, id)
	}
	h.log.Info("hub detenido", "visores_desconectados", viewers)
}

func sweepInterval(ttl time.Duration) time.Duration {
	return min(max(ttl/2, time.Second), time.Minute)
}
