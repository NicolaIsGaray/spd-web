package hub

import (
	"context"
	"encoding/json"
	"errors"
	"math/rand/v2"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

const (
	pid  = "6f1c1a52-2a0e-4b6b-9a59-6a1d8f3f1c11"
	pid2 = "0b8e4c3e-7f4a-4f0e-8d55-2f8f0f6a9b22"
)

// fakeCatalog simula el servicio de presentaciones y cuenta las consultas.
type fakeCatalog struct {
	mu     sync.Mutex
	counts map[string]int
	calls  map[string]int
}

func newFakeCatalog(counts map[string]int) *fakeCatalog {
	return &fakeCatalog{counts: counts, calls: map[string]int{}}
}

func (f *fakeCatalog) SlideCount(_ context.Context, id string) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls[id]++
	n, ok := f.counts[id]
	if !ok {
		return 0, ErrPresentationNotFound
	}
	return n, nil
}

func (f *fakeCatalog) callsFor(id string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[id]
}

func startHub(t *testing.T, cat Catalog, opts Options) *Hub {
	t.Helper()
	h := New(cat, opts)
	ctx, cancel := context.WithCancel(context.Background())
	go h.Run(ctx)
	t.Cleanup(func() {
		cancel()
		<-h.Done()
	})
	return h
}

func register(t *testing.T, h *Hub, id string) *Client {
	t.Helper()
	c := NewClient()
	if err := h.Register(context.Background(), id, c); err != nil {
		t.Fatalf("Register(%s): %v", id, err)
	}
	return c
}

func control(t *testing.T, h *Hub, id string, cmd Command) SlideState {
	t.Helper()
	st, err := h.Control(context.Background(), id, cmd)
	if err != nil {
		t.Fatalf("Control(%+v): %v", cmd, err)
	}
	return st
}

// recv lee el siguiente estado del buzón.
func recv(t *testing.T, c *Client) SlideState {
	t.Helper()
	select {
	case msg, ok := <-c.Messages():
		if !ok {
			t.Fatal("el buzón se cerró inesperadamente")
		}
		var st SlideState
		if err := json.Unmarshal(msg, &st); err != nil {
			t.Fatalf("mensaje inválido %q: %v", msg, err)
		}
		return st
	case <-time.After(2 * time.Second):
		t.Fatal("no llegó ningún mensaje")
		return SlideState{}
	}
}

// Control responde DESPUÉS de difundir, así que comprobar el buzón justo después es determinista.
func assertNoMessage(t *testing.T, c *Client) {
	t.Helper()
	select {
	case msg := <-c.Messages():
		t.Fatalf("mensaje inesperado: %s", msg)
	default:
	}
}

func assertClosed(t *testing.T, c *Client) {
	t.Helper()
	select {
	case _, ok := <-c.Messages():
		if ok {
			t.Fatal("se esperaba el buzón cerrado y llegó un mensaje")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("el buzón no se cerró")
	}
}

func TestRegisterReceivesCurrentSlide(t *testing.T) {
	h := startHub(t, newFakeCatalog(map[string]int{pid: 5}), Options{})

	control(t, h, pid, Command{Action: ActionGoto, Slide: 3})
	c := register(t, h, pid)

	want := SlideState{Type: "slide", PresentationID: pid, Slide: 3, SlideCount: 5, URL: "/api/presentaciones/" + pid + "/slides/3"}
	if got := recv(t, c); got != want {
		t.Fatalf("estado inicial = %+v, se esperaba %+v", got, want)
	}
}

func TestBroadcastReachesOnlyItsSession(t *testing.T) {
	h := startHub(t, newFakeCatalog(map[string]int{pid: 5, pid2: 2}), Options{})
	a1, a2, b := register(t, h, pid), register(t, h, pid), register(t, h, pid2)
	for _, c := range []*Client{a1, a2, b} {
		recv(t, c) // estado inicial
	}

	if st := control(t, h, pid, Command{Action: ActionNext}); st.Slide != 2 {
		t.Fatalf("respuesta de control: slide = %d, se esperaba 2", st.Slide)
	}
	for _, c := range []*Client{a1, a2} {
		if got := recv(t, c); got.Slide != 2 || got.PresentationID != pid {
			t.Fatalf("visor recibió %+v", got)
		}
	}
	assertNoMessage(t, b)
}

func TestNavigationRules(t *testing.T) {
	h := startHub(t, newFakeCatalog(map[string]int{pid: 3}), Options{})
	c := register(t, h, pid)
	recv(t, c)

	// prev en la primera diapositiva: no cambia y no difunde.
	if st := control(t, h, pid, Command{Action: ActionPrev}); st.Slide != 1 {
		t.Fatalf("prev en la primera: slide = %d", st.Slide)
	}
	assertNoMessage(t, c)

	// next se detiene en la última.
	for range 5 {
		control(t, h, pid, Command{Action: ActionNext})
	}
	if got := recv(t, c); got.Slide != 3 {
		t.Fatalf("tras varios next: slide = %d, se esperaba 3", got.Slide)
	}

	// goto fuera de rango: error y el estado no cambia.
	for _, n := range []int{0, -1, 4} {
		_, err := h.Control(context.Background(), pid, Command{Action: ActionGoto, Slide: n})
		if !errors.Is(err, ErrSlideOutOfRange) {
			t.Fatalf("goto %d: err = %v, se esperaba ErrSlideOutOfRange", n, err)
		}
	}
	if _, err := h.Control(context.Background(), pid, Command{Action: "jump"}); !errors.Is(err, ErrUnknownAction) {
		t.Fatalf("acción desconocida: err = %v", err)
	}
	if st := control(t, h, pid, Command{Action: ActionGoto, Slide: 1}); st.Slide != 1 {
		t.Fatalf("goto 1: slide = %d", st.Slide)
	}
	if got := recv(t, c); got.Slide != 1 {
		t.Fatalf("visor: slide = %d, se esperaba 1", got.Slide)
	}
}

func TestSlowViewerGetsLatestState(t *testing.T) {
	h := startHub(t, newFakeCatalog(map[string]int{pid: 10}), Options{})
	c := register(t, h, pid) // no se lee el estado inicial: el visor va "atrasado"

	for range 3 {
		control(t, h, pid, Command{Action: ActionNext})
	}
	if got := recv(t, c); got.Slide != 4 {
		t.Fatalf("visor lento recibió slide %d, se esperaba solo el último estado (4)", got.Slide)
	}
	assertNoMessage(t, c)
}

func TestUnknownPresentation(t *testing.T) {
	h := startHub(t, newFakeCatalog(nil), Options{})
	if err := h.Register(context.Background(), pid, NewClient()); !errors.Is(err, ErrPresentationNotFound) {
		t.Fatalf("Register: err = %v", err)
	}
	if _, err := h.Control(context.Background(), pid, Command{Action: ActionNext}); !errors.Is(err, ErrPresentationNotFound) {
		t.Fatalf("Control: err = %v", err)
	}
}

func TestCatalogQueriedOncePerSession(t *testing.T) {
	cat := newFakeCatalog(map[string]int{pid: 5})
	h := startHub(t, cat, Options{})
	for range 3 {
		register(t, h, pid)
		control(t, h, pid, Command{Action: ActionNext})
	}
	if n := cat.callsFor(pid); n != 1 {
		t.Fatalf("consultas al catálogo = %d, se esperaba 1", n)
	}
}

func TestUnregister(t *testing.T) {
	h := startHub(t, newFakeCatalog(map[string]int{pid: 5}), Options{})
	gone, stays := register(t, h, pid), register(t, h, pid)
	recv(t, gone)
	recv(t, stays)

	h.Unregister(gone)
	assertClosed(t, gone)
	h.Unregister(gone) // idempotente: no debe entrar en pánico (doble close)

	control(t, h, pid, Command{Action: ActionNext})
	if got := recv(t, stays); got.Slide != 2 {
		t.Fatalf("visor restante: slide = %d", got.Slide)
	}
	if err := h.Register(context.Background(), pid, gone); !errors.Is(err, ErrClientReused) {
		t.Fatalf("re-registrar un cliente cerrado: err = %v", err)
	}
}

func TestShutdownClosesEveryViewer(t *testing.T) {
	h := New(newFakeCatalog(map[string]int{pid: 5, pid2: 3}), Options{})
	ctx, cancel := context.WithCancel(context.Background())
	go h.Run(ctx)

	viewers := []*Client{register(t, h, pid), register(t, h, pid), register(t, h, pid2)}
	cancel()
	<-h.Done()

	for _, c := range viewers {
		recv(t, c) // el estado inicial sigue en el buzón...
		assertClosed(t, c)
	}
	if err := h.Register(context.Background(), pid, NewClient()); !errors.Is(err, ErrClosed) {
		t.Fatalf("Register tras detener: err = %v", err)
	}
	if _, err := h.Control(context.Background(), pid, Command{Action: ActionNext}); !errors.Is(err, ErrClosed) {
		t.Fatalf("Control tras detener: err = %v", err)
	}
	h.Unregister(viewers[0]) // no debe bloquear con el Hub detenido
}

func TestCanceledContextDoesNotBlock(t *testing.T) {
	h := New(newFakeCatalog(map[string]int{pid: 5}), Options{}) // Run nunca arranca
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := h.Register(ctx, pid, NewClient()); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, se esperaba context.Canceled", err)
	}
}

func TestIdleSessionEviction(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cat := newFakeCatalog(map[string]int{pid: 5})
		h := New(cat, Options{IdleTTL: time.Hour})
		ctx, cancel := context.WithCancel(context.Background())
		go h.Run(ctx)
		defer func() { cancel(); <-h.Done() }()

		c := register(t, h, pid)
		recv(t, c)
		control(t, h, pid, Command{Action: ActionGoto, Slide: 4})
		recv(t, c)

		// Con un visor conectado, la sesión se conserva aunque pase mucho tiempo sin actividad.
		time.Sleep(3 * time.Hour)
		synctest.Wait()
		if st := control(t, h, pid, Command{Action: ActionNext}); st.Slide != 5 || cat.callsFor(pid) != 1 {
			t.Fatalf("con visores: slide = %d, consultas = %d; se esperaba 5 y 1", st.Slide, cat.callsFor(pid))
		}
		recv(t, c)

		// Sin visores y sin actividad durante más de IdleTTL, se libera: el estado vuelve a la
		// diapositiva 1 y el catálogo se consulta de nuevo.
		h.Unregister(c)
		time.Sleep(2 * time.Hour)
		synctest.Wait()
		if st := control(t, h, pid, Command{Action: ActionNext}); st.Slide != 2 || cat.callsFor(pid) != 2 {
			t.Fatalf("tras liberar: slide = %d, consultas = %d; se esperaba 2 y 2", st.Slide, cat.callsFor(pid))
		}
	})
}

func TestOfferNeverBlocks(t *testing.T) {
	c := NewClient()
	for i := range 1000 {
		c.offer([]byte{byte(i)})
	}
	if got := <-c.Messages(); got[0] != byte(999%256) {
		t.Fatalf("buzón = %v, se esperaba solo el último mensaje", got)
	}
}

// TestConcurrentStress ejercita el Hub desde muchas goroutines a la vez. Ejecutar con -race.
// Invariantes: ningún visor recibe mensajes de otra sesión, la diapositiva siempre está en
// rango y, cuando el presentador solo avanza, ningún visor ve un estado anterior a otro ya
// recibido (el "último gana" puede saltarse estados, pero nunca los desordena).
func TestConcurrentStress(t *testing.T) {
	const (
		slides  = 300
		viewers = 25
	)
	ids := []string{pid, pid2, "3c0d7a8e-1111-4a2b-9c3d-4e5f6a7b8c9d", "9a8b7c6d-2222-4e3f-8a1b-2c3d4e5f6a7b"}
	counts := map[string]int{}
	for _, id := range ids {
		counts[id] = slides
	}
	h := startHub(t, newFakeCatalog(counts), Options{IdleTTL: time.Millisecond})
	ctx := context.Background()

	var (
		wg       sync.WaitGroup
		received atomic.Int64
	)
	for i, id := range ids {
		monotonic := i%2 == 0 // la mitad de las sesiones solo avanza; la otra recibe órdenes aleatorias

		for range viewers {
			wg.Go(func() {
				c := NewClient()
				if err := h.Register(ctx, id, c); err != nil {
					t.Errorf("Register: %v", err)
					return
				}
				last := 0
				check := func(msg []byte) {
					received.Add(1)
					var st SlideState
					if err := json.Unmarshal(msg, &st); err != nil {
						t.Errorf("mensaje inválido: %v", err)
						return
					}
					switch {
					case st.PresentationID != id:
						t.Errorf("visor de %s recibió un mensaje de %s", id, st.PresentationID)
					case st.Slide < 1 || st.Slide > slides:
						t.Errorf("slide fuera de rango: %d", st.Slide)
					case monotonic && st.Slide < last:
						t.Errorf("estado desordenado: %d después de %d", st.Slide, last)
					}
					last = st.Slide
				}
				for range 1 + rand.IntN(20) {
					select {
					case msg := <-c.Messages():
						check(msg)
					case <-time.After(50 * time.Millisecond):
					}
				}
				h.Unregister(c)
				for msg := range c.Messages() { // hasta que el Hub cierre el buzón
					check(msg)
				}
			})
		}

		wg.Go(func() {
			for range slides {
				cmd := Command{Action: ActionNext}
				if !monotonic {
					switch rand.IntN(3) {
					case 0:
						cmd = Command{Action: ActionPrev}
					case 1:
						cmd = Command{Action: ActionGoto, Slide: 1 + rand.IntN(slides)}
					}
				}
				if _, err := h.Control(ctx, id, cmd); err != nil {
					t.Errorf("Control: %v", err)
				}
			}
		})
	}
	wg.Wait()
	if received.Load() < int64(len(ids)*viewers) {
		t.Fatalf("solo se recibieron %d mensajes: el test no ejercitó la difusión", received.Load())
	}
	t.Logf("%d visores, %d comandos, %d mensajes entregados", len(ids)*viewers, len(ids)*slides, received.Load())
}
