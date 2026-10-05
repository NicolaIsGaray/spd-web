package hub

type clientState uint8

const (
	clientNew clientState = iota
	clientActive
	clientClosed
)

// Client es un visor visto desde el Hub: solo un buzón de salida. El transporte (WebSocket)
// vive en otro paquete y se limita a consumir Messages(), así el Hub se prueba sin red.
type Client struct {
	// send tiene capacidad 1 y semántica "el último gana". La goroutine Run del Hub es su
	// ÚNICA emisora y la única que lo cierra: nunca puede haber un envío sobre canal cerrado.
	send chan []byte

	// Campos propiedad exclusiva de la goroutine Run.
	state     clientState
	sessionID string
}

// NewClient crea un visor listo para registrarse en el Hub (una única vez).
func NewClient() *Client {
	return &Client{send: make(chan []byte, 1)}
}

// Messages devuelve el buzón del visor: cada mensaje es un SlideState serializado en JSON.
// El canal se cierra cuando el visor se da de baja o el Hub se detiene.
func (c *Client) Messages() <-chan []byte { return c.send }

// offer deposita msg en el buzón SIN BLOQUEAR NUNCA al Hub, por lento que sea el visor.
//
// Si el buzón está lleno se descarta el mensaje pendiente: cada mensaje es el estado completo
// de la presentación, así que uno nuevo vuelve obsoleto al anterior y el visor no pierde nada.
// Como Run es la única emisora, tras vaciar el buzón el siguiente envío siempre cabe: el bucle
// termina como mucho en la segunda vuelta.
func (c *Client) offer(msg []byte) {
	for {
		select {
		case c.send <- msg:
			return
		default:
		}
		select {
		case <-c.send: // descartar el estado obsoleto
		default: // el visor acaba de consumirlo: ya hay hueco
		}
	}
}

func (c *Client) close() {
	c.state = clientClosed
	close(c.send)
}
