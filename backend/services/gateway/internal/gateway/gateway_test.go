package gateway

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/gin-gonic/gin"

	"spd.web/internal/platform"
)

const pid = "6f1c1a52-2a0e-4b6b-9a59-6a1d8f3f1c11"

// fakeUpstream responde con el nombre del servicio y lo que recibió, y acepta WebSockets.
func fakeUpstream(t *testing.T, name string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if platform.IsWebSocketUpgrade(r) {
			conn, err := websocket.Accept(w, r, nil) // verificación de Origin por defecto (mismo host)
			if err != nil {
				return
			}
			defer conn.CloseNow()
			_ = conn.Write(r.Context(), websocket.MessageText, []byte("hola desde "+name))
			_ = conn.Close(websocket.StatusNormalClosure, "")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"service": name,
			"method":  r.Method,
			"path":    r.URL.Path,
			"host":    r.Host,
			"xff":     r.Header.Get("X-Forwarded-For"),
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newGateway(t *testing.T, presentations, realtime string) *httptest.Server {
	t.Helper()
	gin.SetMode(gin.TestMode)
	log := slog.New(slog.DiscardHandler)
	p, _ := url.Parse(presentations)
	r, _ := url.Parse(realtime)
	engine := platform.NewEngine(log, CORS([]string{"http://localhost:5173"}))
	Register(engine, Upstreams{Presentations: p, Realtime: r}, log)
	srv := httptest.NewServer(engine)
	t.Cleanup(srv.Close)
	return srv
}

func call(t *testing.T, method, url string, header http.Header) (*http.Response, map[string]string) {
	t.Helper()
	req, _ := http.NewRequest(method, url, strings.NewReader(`{"action":"next"}`))
	for k, v := range header {
		req.Header[k] = v
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var out map[string]string
	_ = json.Unmarshal(body, &out)
	return resp, out
}

func TestRoutesReachTheOwningService(t *testing.T) {
	gw := newGateway(t, fakeUpstream(t, "presentations").URL, fakeUpstream(t, "realtime").URL)
	gwHost := strings.TrimPrefix(gw.URL, "http://")

	routes := []struct{ method, path, service string }{
		{http.MethodPost, "/api/presentaciones/upload", "presentations"},
		{http.MethodGet, "/api/presentaciones/" + pid, "presentations"},
		{http.MethodGet, "/api/presentaciones/" + pid + "/slides/3", "presentations"},
		{http.MethodPost, "/api/presentaciones/" + pid + "/control", "realtime"},
	}
	for _, rt := range routes {
		resp, body := call(t, rt.method, gw.URL+rt.path, nil)
		if resp.StatusCode != http.StatusOK || body["service"] != rt.service || body["path"] != rt.path {
			t.Fatalf("%s %s → %d %v; se esperaba %s", rt.method, rt.path, resp.StatusCode, body, rt.service)
		}
		if body["host"] != gwHost {
			t.Fatalf("Host recibido por el servicio = %q; debe conservarse el público %q", body["host"], gwHost)
		}
		if body["xff"] == "" {
			t.Fatal("falta X-Forwarded-For")
		}
	}

	if resp, _ := call(t, http.MethodGet, gw.URL+"/api/otra-cosa", nil); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("ruta desconocida = %d, se esperaba 404", resp.StatusCode)
	}
}

func TestWebSocketThroughGateway(t *testing.T) {
	gw := newGateway(t, fakeUpstream(t, "presentations").URL, fakeUpstream(t, "realtime").URL)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Mismo origen que el gateway: el servicio de destino debe aceptarlo porque el gateway
	// conserva el Host público.
	header := http.Header{"Origin": {gw.URL}}
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(gw.URL, "http")+"/ws/presentacion/"+pid, &websocket.DialOptions{HTTPHeader: header})
	if err != nil {
		t.Fatalf("dial a través del gateway: %v", err)
	}
	defer conn.CloseNow()
	_, msg, err := conn.Read(ctx)
	if err != nil || string(msg) != "hola desde realtime" {
		t.Fatalf("mensaje = %q, %v", msg, err)
	}
}

func TestCORS(t *testing.T) {
	gw := newGateway(t, fakeUpstream(t, "presentations").URL, fakeUpstream(t, "realtime").URL)
	control := gw.URL + "/api/presentaciones/" + pid + "/control"

	preflight := http.Header{"Origin": {"http://localhost:5173"}, "Access-Control-Request-Method": {"POST"}, "Access-Control-Request-Headers": {"content-type"}}
	resp, _ := call(t, http.MethodOptions, control, preflight)
	if resp.StatusCode != http.StatusNoContent || resp.Header.Get("Access-Control-Allow-Origin") != "http://localhost:5173" ||
		!strings.Contains(resp.Header.Get("Access-Control-Allow-Headers"), "Content-Type") {
		t.Fatalf("preflight permitido: %d %v", resp.StatusCode, resp.Header)
	}

	resp, body := call(t, http.MethodPost, control, http.Header{"Origin": {"http://localhost:5173"}})
	if resp.Header.Get("Access-Control-Allow-Origin") != "http://localhost:5173" || body["service"] != "realtime" {
		t.Fatalf("petición CORS permitida: %v %v", resp.Header, body)
	}

	resp, _ = call(t, http.MethodPost, control, http.Header{"Origin": {"http://evil.example"}})
	if resp.Header.Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("un origen no permitido no debe recibir Access-Control-Allow-Origin")
	}
}

func TestUnavailableUpstream(t *testing.T) {
	down := httptest.NewServer(http.NotFoundHandler())
	downURL := down.URL
	down.Close() // puerto sin servicio

	gw := newGateway(t, downURL, downURL)
	resp, body := call(t, http.MethodGet, gw.URL+"/api/presentaciones/"+pid, nil)
	if resp.StatusCode != http.StatusBadGateway || body["error"] == "" {
		t.Fatalf("servicio caído: %d %v; se esperaba 502 con mensaje", resp.StatusCode, body)
	}
}
