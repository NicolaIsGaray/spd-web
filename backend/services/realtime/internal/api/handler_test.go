package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/gin-gonic/gin"

	"spd.web/services/realtime/internal/hub"
)

const (
	pid     = "6f1c1a52-2a0e-4b6b-9a59-6a1d8f3f1c11"
	missing = "0b8e4c3e-7f4a-4f0e-8d55-2f8f0f6a9b22"
)

type fakeCatalog map[string]int

func (f fakeCatalog) SlideCount(_ context.Context, id string) (int, error) {
	if n, ok := f[id]; ok {
		return n, nil
	}
	return 0, hub.ErrPresentationNotFound
}

type testEnv struct {
	srv     *httptest.Server
	handler *Handler
	stopHub context.CancelFunc
}

func newTestEnv(t *testing.T, origins []string) *testEnv {
	t.Helper()
	gin.SetMode(gin.TestMode)
	h := hub.New(fakeCatalog{pid: 5}, hub.Options{})
	ctx, cancel := context.WithCancel(context.Background())
	go h.Run(ctx)

	handler := New(h, origins, slog.New(slog.DiscardHandler))
	engine := gin.New()
	handler.Register(engine)
	srv := httptest.NewServer(engine)
	t.Cleanup(func() {
		cancel()
		<-h.Done()
		handler.Shutdown(5 * time.Second)
		srv.Close()
	})
	return &testEnv{srv: srv, handler: handler, stopHub: cancel}
}

func (e *testEnv) dial(t *testing.T, id string, header http.Header) (*websocket.Conn, *http.Response, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	url := "ws" + strings.TrimPrefix(e.srv.URL, "http") + "/ws/presentation/" + id
	conn, resp, err := websocket.Dial(ctx, url, &websocket.DialOptions{HTTPHeader: header})
	if conn != nil {
		t.Cleanup(func() { _ = conn.CloseNow() })
	}
	return conn, resp, err
}

func (e *testEnv) viewer(t *testing.T) *websocket.Conn {
	t.Helper()
	conn, _, err := e.dial(t, pid, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	return conn
}

func (e *testEnv) control(t *testing.T, id, body string) (int, map[string]any) {
	t.Helper()
	resp, err := http.Post(e.srv.URL+"/api/presentations/"+id+"/control", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST control: %v", err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func readState(t *testing.T, conn *websocket.Conn) hub.SlideState {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	typ, data, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("leer del WebSocket: %v", err)
	}
	if typ != websocket.MessageText {
		t.Fatalf("tipo de mensaje = %v, se esperaba texto", typ)
	}
	var st hub.SlideState
	if err := json.Unmarshal(data, &st); err != nil {
		t.Fatalf("JSON inválido %q: %v", data, err)
	}
	return st
}

func readCloseStatus(t *testing.T, conn *websocket.Conn) websocket.StatusCode {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for {
		if _, _, err := conn.Read(ctx); err != nil {
			return websocket.CloseStatus(err)
		}
	}
}

func TestViewersFollowThePresenter(t *testing.T) {
	env := newTestEnv(t, nil)
	v1, v2 := env.viewer(t), env.viewer(t)

	for _, v := range []*websocket.Conn{v1, v2} {
		if st := readState(t, v); st.Slide != 1 || st.SlideCount != 5 || st.Type != "slide" {
			t.Fatalf("estado inicial = %+v", st)
		}
	}

	steps := []struct {
		body string
		want int
	}{
		{`{"action": "next"}`, 2},
		{`{"slide_index": 5}`, 5},
		{`{"action": "prev"}`, 4},
		{`{"action": "goto", "slide": 1}`, 1},
		{`{"slide": 3}`, 3},
	}
	for _, step := range steps {
		status, body := env.control(t, pid, step.body)
		if status != http.StatusOK || int(body["slide"].(float64)) != step.want {
			t.Fatalf("POST %s: %d %v, se esperaba slide %d", step.body, status, body, step.want)
		}
		for _, v := range []*websocket.Conn{v1, v2} {
			if st := readState(t, v); st.Slide != step.want {
				t.Fatalf("tras %s el visor recibió slide %d, se esperaba %d", step.body, st.Slide, step.want)
			}
		}
	}

	// Un visor que llega tarde recibe de inmediato la diapositiva actual.
	if st := readState(t, env.viewer(t)); st.Slide != 3 {
		t.Fatalf("visor tardío: slide %d, se esperaba 3", st.Slide)
	}
}

func TestControlValidation(t *testing.T) {
	env := newTestEnv(t, nil)
	cases := []struct {
		name, id, body string
		want           int
	}{
		{"JSON roto", pid, `{"action":`, http.StatusBadRequest},
		{"campo desconocido", pid, `{"slide_idx": 2}`, http.StatusBadRequest},
		{"acción desconocida", pid, `{"action": "jump"}`, http.StatusBadRequest},
		{"next con número", pid, `{"action": "next", "slide": 2}`, http.StatusBadRequest},
		{"slide y slide_index a la vez", pid, `{"slide": 1, "slide_index": 1}`, http.StatusBadRequest},
		{"cuerpo vacío", pid, `{}`, http.StatusBadRequest},
		{"goto sin número", pid, `{"action": "goto"}`, http.StatusBadRequest},
		{"fuera de rango", pid, `{"slide": 6}`, http.StatusBadRequest},
		{"cero", pid, `{"slide": 0}`, http.StatusBadRequest},
		{"id inválido", "no-es-un-uuid", `{"action": "next"}`, http.StatusBadRequest},
		{"presentación inexistente", missing, `{"action": "next"}`, http.StatusNotFound},
		{"válido", pid, `{"action": "NEXT"}`, http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, body := env.control(t, tc.id, tc.body)
			if status != tc.want {
				t.Fatalf("estado = %d (%v), se esperaba %d", status, body, tc.want)
			}
			if status != http.StatusOK && body["error"] == "" {
				t.Fatalf("falta el mensaje de error: %v", body)
			}
		})
	}
}

func TestUnknownPresentationClosesWith4404(t *testing.T) {
	env := newTestEnv(t, nil)
	conn, _, err := env.dial(t, missing, nil)
	if err != nil {
		t.Fatalf("el handshake debería completarse para poder informar con un código de cierre: %v", err)
	}
	if code := readCloseStatus(t, conn); code != StatusPresentationNotFound {
		t.Fatalf("código de cierre = %d, se esperaba %d", code, StatusPresentationNotFound)
	}
}

func TestInvalidIDIsRejectedBeforeUpgrade(t *testing.T) {
	env := newTestEnv(t, nil)
	_, resp, err := env.dial(t, "../../etc", nil)
	if err == nil || resp == nil || resp.StatusCode != http.StatusBadRequest && resp.StatusCode != http.StatusNotFound {
		t.Fatalf("se esperaba 400/404 antes del upgrade; err=%v resp=%v", err, resp)
	}
}

func TestOriginCheck(t *testing.T) {
	env := newTestEnv(t, []string{"http://localhost:5173"})

	_, resp, err := env.dial(t, pid, http.Header{"Origin": {"http://evil.example"}})
	if err == nil || resp == nil || resp.StatusCode != http.StatusForbidden {
		t.Fatalf("origen no permitido: se esperaba 403; err=%v", err)
	}
	conn, _, err := env.dial(t, pid, http.Header{"Origin": {"http://localhost:5173"}})
	if err != nil {
		t.Fatalf("origen permitido rechazado: %v", err)
	}
	readState(t, conn)
}

func TestShutdownSendsGoingAway(t *testing.T) {
	env := newTestEnv(t, nil)
	conn := env.viewer(t)
	readState(t, conn)

	env.stopHub() // equivale a lo que hace main al recibir SIGTERM
	if code := readCloseStatus(t, conn); code != websocket.StatusGoingAway {
		t.Fatalf("código de cierre = %d, se esperaba 1001 (going away)", code)
	}
}

// Un visor que no responde al cierre ordenado (aquí: nunca lee, así que no devuelve el close)
// no debe retrasar el apagado más allá del plazo de gracia.
func TestShutdownForcesUnresponsiveViewers(t *testing.T) {
	env := newTestEnv(t, nil)
	env.viewer(t) // conectado, pero nunca lee

	env.stopHub()
	start := time.Now()
	if !env.handler.Shutdown(200 * time.Millisecond) {
		t.Fatal("quedaron conexiones abiertas tras forzar el cierre")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("el apagado tardó %v pese al plazo de gracia de 200ms", elapsed)
	}
}
