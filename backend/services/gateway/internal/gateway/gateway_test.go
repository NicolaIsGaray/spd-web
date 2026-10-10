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

func newGateway(t *testing.T, presentations, realtime, rooms string) *httptest.Server {
	t.Helper()
	gin.SetMode(gin.TestMode)
	log := slog.New(slog.DiscardHandler)
	p, _ := url.Parse(presentations)
	r, _ := url.Parse(realtime)
	rm, _ := url.Parse(rooms)
	engine := platform.NewEngine(log, CORS([]string{"http://localhost:5173"}))
	Register(engine, Upstreams{Presentations: p, Realtime: r, Rooms: rm}, log)
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
	gw := newGateway(t, fakeUpstream(t, "presentations").URL, fakeUpstream(t, "realtime").URL, fakeUpstream(t, "rooms").URL)
	gwHost := strings.TrimPrefix(gw.URL, "http://")

	routes := []struct{ method, path, service string }{
		{http.MethodPost, "/api/presentations/upload", "presentations"},
		{http.MethodGet, "/api/presentations/" + pid, "presentations"},
		{http.MethodGet, "/api/presentations/" + pid + "/slides/3", "presentations"},
		{http.MethodPost, "/api/presentations/" + pid + "/control", "realtime"},

		{http.MethodPost, "/api/users", "rooms"},
		{http.MethodGet, "/api/users", "rooms"},
		{http.MethodGet, "/api/users/1", "rooms"},
		{http.MethodPatch, "/api/users/1", "rooms"},
		{http.MethodDelete, "/api/users/1", "rooms"},
		{http.MethodGet, "/api/users/1/rooms", "rooms"},
		{http.MethodGet, "/api/users/1/groups", "rooms"},
		{http.MethodPost, "/api/rooms", "rooms"},
		{http.MethodGet, "/api/rooms", "rooms"},
		{http.MethodGet, "/api/rooms/1", "rooms"},
		{http.MethodPatch, "/api/rooms/1", "rooms"},
		{http.MethodDelete, "/api/rooms/1", "rooms"},
		{http.MethodGet, "/api/rooms/1/members", "rooms"},
		{http.MethodPost, "/api/rooms/1/members", "rooms"},
		{http.MethodDelete, "/api/rooms/1/members/2", "rooms"},
		{http.MethodGet, "/api/rooms/1/groups", "rooms"},
		{http.MethodPost, "/api/rooms/1/groups", "rooms"},
		{http.MethodGet, "/api/groups/1", "rooms"},
		{http.MethodPatch, "/api/groups/1", "rooms"},
		{http.MethodDelete, "/api/groups/1", "rooms"},
		{http.MethodGet, "/api/groups/1/members", "rooms"},
		{http.MethodPost, "/api/groups/1/members", "rooms"},
		{http.MethodDelete, "/api/groups/1/members/2", "rooms"},
		{http.MethodGet, "/api/groups/1/presentations", "rooms"},
		{http.MethodPost, "/api/groups/1/presentations", "rooms"},
		{http.MethodGet, "/api/groups/1/presentations/" + pid, "rooms"},
		{http.MethodPatch, "/api/groups/1/presentations/" + pid, "rooms"},
		{http.MethodDelete, "/api/groups/1/presentations/" + pid, "rooms"},
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

	for _, path := range []string{"/api/otra-cosa", "/api/salas"} {
		if resp, _ := call(t, http.MethodGet, gw.URL+path, nil); resp.StatusCode != http.StatusNotFound {
			t.Fatalf("ruta desconocida %s = %d, se esperaba 404", path, resp.StatusCode)
		}
	}
}

func TestWebSocketThroughGateway(t *testing.T) {
	gw := newGateway(t, fakeUpstream(t, "presentations").URL, fakeUpstream(t, "realtime").URL, fakeUpstream(t, "rooms").URL)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Mismo origen que el gateway: el servicio de destino debe aceptarlo porque el gateway
	// conserva el Host público.
	header := http.Header{"Origin": {gw.URL}}
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(gw.URL, "http")+"/ws/presentation/"+pid, &websocket.DialOptions{HTTPHeader: header})
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
	gw := newGateway(t, fakeUpstream(t, "presentations").URL, fakeUpstream(t, "realtime").URL, fakeUpstream(t, "rooms").URL)
	control := gw.URL + "/api/presentations/" + pid + "/control"

	preflight := http.Header{"Origin": {"http://localhost:5173"}, "Access-Control-Request-Method": {"POST"}, "Access-Control-Request-Headers": {"content-type"}}
	resp, _ := call(t, http.MethodOptions, control, preflight)
	if resp.StatusCode != http.StatusNoContent || resp.Header.Get("Access-Control-Allow-Origin") != "http://localhost:5173" ||
		!strings.Contains(resp.Header.Get("Access-Control-Allow-Headers"), "Content-Type") {
		t.Fatalf("preflight permitido: %d %v", resp.StatusCode, resp.Header)
	}

	// El servicio rooms usa PATCH y DELETE: el preflight tiene que permitirlos.
	preflight["Access-Control-Request-Method"] = []string{"DELETE"}
	resp, _ = call(t, http.MethodOptions, gw.URL+"/api/rooms/1", preflight)
	if methods := resp.Header.Get("Access-Control-Allow-Methods"); !strings.Contains(methods, "PATCH") || !strings.Contains(methods, "DELETE") {
		t.Fatalf("Access-Control-Allow-Methods = %q", methods)
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

	gw := newGateway(t, downURL, downURL, downURL)
	resp, body := call(t, http.MethodGet, gw.URL+"/api/presentations/"+pid, nil)
	if resp.StatusCode != http.StatusBadGateway || body["error"] == "" {
		t.Fatalf("servicio caído: %d %v; se esperaba 502 con mensaje", resp.StatusCode, body)
	}
}
