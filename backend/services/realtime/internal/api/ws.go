package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/coder/websocket"
	"github.com/gin-gonic/gin"

	"spd.web/internal/platform"
	"spd.web/services/realtime/internal/hub"
)

const (
	writeTimeout = 10 * time.Second // plazo para entregar un mensaje; un visor más lento se desconecta
	pingInterval = 30 * time.Second // heartbeat para detectar conexiones muertas
	pingTimeout  = 10 * time.Second // espera máxima del pong
	readLimit    = 512              // bytes por mensaje entrante: los visores no envían datos

	// StatusPresentationNotFound es un código de cierre propio (rango 4000-4999 de la RFC 6455):
	// la presentación no existe y el visor no debe reintentar.
	StatusPresentationNotFound websocket.StatusCode = 4404
)

// serveWS atiende GET /ws/presentation/:id.
func (h *Handler) serveWS(c *gin.Context) {
	id, ok := platform.CanonicalUUID(c.Param("id"))
	if !ok {
		platform.WriteError(c, http.StatusBadRequest, "id de presentación inválido")
		return
	}

	h.conns.Add(1)
	defer h.conns.Done()

	conn, err := websocket.Accept(c.Writer, c.Request, &websocket.AcceptOptions{
		OriginPatterns: h.originPatterns,
	})
	if err != nil {
		// Accept ya respondió al cliente (403 si el Origin no está permitido, 426 si no es WebSocket...).
		h.log.Warn("handshake WebSocket rechazado", "presentation_id", id, "err", err)
		return
	}
	// Red de seguridad: libera el socket en cualquier salida (no hace nada si ya se cerró).
	defer func() { _ = conn.CloseNow() }()

	// La vida de la conexión la gobiernan el Hub (al apagar, cierre ordenado 1001), el propio
	// socket y, como último recurso, Shutdown (cierre forzado). No se usa el contexto de la
	// petición HTTP: tras el hijack no refleja el estado de la conexión.
	ctx := h.connCtx

	// El registro va DESPUÉS del handshake para poder informar del error con un código de cierre:
	// un navegador no puede leer el estado HTTP de un handshake fallido, pero sí event.code.
	client := hub.NewClient()
	if err := h.hub.Register(ctx, id, client); err != nil {
		code, reason := closeStatus(err)
		h.log.Info("visor rechazado", "presentation_id", id, "err", err)
		_ = conn.Close(code, reason)
		return
	}
	defer h.hub.Unregister(client)

	h.log.Debug("visor conectado", "presentation_id", id)
	err = pump(ctx, conn, client.Messages())
	h.log.Debug("visor desconectado", "presentation_id", id, "motivo", err)
}

// pump mueve mensajes del Hub al socket hasta que la conexión termina:
//
//   - writeLoop, en su propia goroutine, es la ÚNICA que escribe mensajes de datos: serializa
//     las escrituras y aplica un plazo a cada una. El Hub nunca escribe en el socket, solo
//     deposita en un buzón no bloqueante, así que un visor lento jamás lo frena.
//   - readLoop, en la goroutine actual, lee sin parar: sin un lector activo la librería no
//     procesaría los frames de control (el pong de nuestros pings ni el close del visor).
//
// Cuando cualquiera de los dos termina cancela al otro, y pump no retorna hasta que ambos han
// terminado: no quedan goroutines huérfanas ni escrituras en curso tras el retorno.
func pump(ctx context.Context, conn *websocket.Conn, msgs <-chan []byte) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	conn.SetReadLimit(readLimit)

	writeDone := make(chan error, 1)
	go func() {
		err := writeLoop(ctx, conn, msgs)
		cancel() // si la escritura termina (fallo, visor lento, apagado), desbloquea readLoop
		writeDone <- err
	}()

	readErr := readLoop(ctx, conn)
	cancel()                // el visor cerró o el socket falló: detener writeLoop
	writeErr := <-writeDone // esperar a writeLoop antes de retornar

	if errors.Is(readErr, context.Canceled) && writeErr != nil {
		return writeErr // la lectura solo se interrumpió porque falló la escritura
	}
	return readErr
}

// writeLoop entrega al visor cada estado que el Hub deposita en su buzón y envía el heartbeat.
func writeLoop(ctx context.Context, conn *websocket.Conn, msgs <-chan []byte) error {
	heartbeat := time.NewTicker(pingInterval)
	defer heartbeat.Stop()

	for {
		select {
		case msg, ok := <-msgs:
			if !ok {
				// Dentro de pump, el buzón solo se cierra si el Hub se detiene (apagado del
				// servicio). Cierre ordenado con 1001 para que el visor sepa que debe reconectar.
				return conn.Close(websocket.StatusGoingAway, "servidor reiniciándose")
			}
			if err := writeMessage(ctx, conn, msg); err != nil {
				return err
			}
		case <-heartbeat.C:
			// Detecta visores caídos (TCP semiabierto) que nunca provocarían un error de lectura.
			// El pong lo recibe y procesa readLoop.
			if err := ping(ctx, conn); err != nil {
				return err
			}
		case <-ctx.Done():
			return nil // readLoop terminó primero: ya no hay a quién escribir
		}
	}
}

func writeMessage(ctx context.Context, conn *websocket.Conn, msg []byte) error {
	ctx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()
	return conn.Write(ctx, websocket.MessageText, msg)
}

func ping(ctx context.Context, conn *websocket.Conn) error {
	ctx, cancel := context.WithTimeout(ctx, pingTimeout)
	defer cancel()
	return conn.Ping(ctx)
}

// readLoop lee hasta que la conexión se cierra. Los visores son de solo lectura: cualquier
// mensaje de datos se descarta (y uno mayor que readLimit cierra la conexión con 1009).
func readLoop(ctx context.Context, conn *websocket.Conn) error {
	for {
		if _, _, err := conn.Read(ctx); err != nil {
			return err
		}
	}
}

// closeStatus traduce un error de registro a un código de cierre WebSocket que el frontend
// puede leer en event.code para decidir si reintenta.
func closeStatus(err error) (websocket.StatusCode, string) {
	switch {
	case errors.Is(err, hub.ErrPresentationNotFound):
		return StatusPresentationNotFound, "presentación no encontrada"
	case errors.Is(err, hub.ErrClosed):
		return websocket.StatusGoingAway, "servidor reiniciándose"
	default:
		return websocket.StatusTryAgainLater, "servicio no disponible, reintenta"
	}
}
