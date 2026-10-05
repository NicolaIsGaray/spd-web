package api

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"log/slog"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"spd.web/internal/platform"
	"spd.web/services/presentations/internal/presentation"
	"spd.web/services/presentations/internal/storage"
)

type noConverter struct{}

func (noConverter) ToImages(context.Context, string, string, int) ([]string, error) {
	return nil, presentation.ErrConverterUnavailable
}

func newServer(t *testing.T, maxUpload int64) *httptest.Server {
	t.Helper()
	gin.SetMode(gin.TestMode)
	log := slog.New(slog.DiscardHandler)
	store, err := storage.Open(t.TempDir(), log)
	if err != nil {
		t.Fatal(err)
	}
	limits := storage.Limits{MaxUploadBytes: maxUpload, MaxEntries: 100, MaxSlides: 20, MaxImageBytes: 1 << 20, MaxTotalBytes: 4 << 20}
	engine := platform.NewEngine(log)
	New(presentation.NewService(store, noConverter{}, limits, log), maxUpload, log).Register(engine)
	srv := httptest.NewServer(engine)
	t.Cleanup(srv.Close)
	return srv
}

func pngOf(t *testing.T, width int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewGray(image.Rect(0, 0, width, 2))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func zipOf(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, data := range files {
		w, _ := zw.Create(name)
		_, _ = w.Write(data)
	}
	_ = zw.Close()
	return buf.Bytes()
}

func upload(t *testing.T, srv *httptest.Server, field, filename string, data []byte) (*http.Response, map[string]any) {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	_ = mw.WriteField("titulo", "campo previo que debe ignorarse")
	fw, _ := mw.CreateFormFile(field, filename)
	_, _ = fw.Write(data)
	_ = mw.Close()

	resp, err := http.Post(srv.URL+"/api/presentaciones/upload", mw.FormDataContentType(), &body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp, out
}

func get(t *testing.T, url string) (*http.Response, []byte) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return resp, data
}

func TestUploadAndServeSlides(t *testing.T) {
	srv := newServer(t, 1<<20)
	slide1, slide2 := pngOf(t, 1), pngOf(t, 2)

	resp, body := upload(t, srv, "file", "deck.zip", zipOf(t, map[string][]byte{"s2.png": slide2, "s1.png": slide1}))
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("upload: %d %v", resp.StatusCode, body)
	}
	id, _ := body["presentation_id"].(string)
	if id == "" || body["slide_count"].(float64) != 2 {
		t.Fatalf("respuesta = %v", body)
	}
	if loc := resp.Header.Get("Location"); loc != "/api/presentaciones/"+id {
		t.Fatalf("Location = %q", loc)
	}

	meta, data := get(t, srv.URL+"/api/presentaciones/"+id)
	if meta.StatusCode != http.StatusOK || !bytes.Contains(data, []byte(`"file":"002.png"`)) {
		t.Fatalf("metadatos: %d %s", meta.StatusCode, data)
	}

	for _, ref := range []string{"1", "001.png"} {
		img, data := get(t, srv.URL+"/api/presentaciones/"+id+"/slides/"+ref)
		if img.StatusCode != http.StatusOK || !bytes.Equal(data, slide1) {
			t.Fatalf("slide %s: %d (%d bytes)", ref, img.StatusCode, len(data))
		}
		if ct := img.Header.Get("Content-Type"); ct != "image/png" {
			t.Fatalf("Content-Type = %q", ct)
		}
		if cc := img.Header.Get("Cache-Control"); cc != "public, max-age=31536000, immutable" {
			t.Fatalf("Cache-Control = %q", cc)
		}
	}
}

func TestErrorResponses(t *testing.T) {
	srv := newServer(t, 1<<20)
	resp, body := upload(t, srv, "file", "deck.zip", zipOf(t, map[string][]byte{"s1.png": pngOf(t, 1)}))
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("upload: %d %v", resp.StatusCode, body)
	}
	id := body["presentation_id"].(string)

	uploads := []struct {
		name, field, filename string
		data                  []byte
		want                  int
	}{
		{"sin campo file", "documento", "deck.zip", []byte("x"), http.StatusBadRequest},
		{"formato no soportado", "file", "deck.pdf", []byte("%PDF-1.7"), http.StatusUnsupportedMediaType},
		{"zip inválido", "file", "deck.zip", []byte("no soy un zip"), http.StatusBadRequest},
		{"demasiado grande", "file", "deck.zip", make([]byte, 3<<20), http.StatusRequestEntityTooLarge},
		{"pptx sin conversor", "file", "deck.pptx", zipOf(t, map[string][]byte{"ppt/presentation.xml": nil}), http.StatusServiceUnavailable},
	}
	for _, tc := range uploads {
		t.Run(tc.name, func(t *testing.T) {
			resp, body := upload(t, srv, tc.field, tc.filename, tc.data)
			if resp.StatusCode != tc.want || body["error"] == nil {
				t.Fatalf("estado = %d (%v), se esperaba %d con mensaje de error", resp.StatusCode, body, tc.want)
			}
		})
	}

	gets := map[string]int{
		"/api/presentaciones/no-es-uuid":                               http.StatusBadRequest,
		"/api/presentaciones/6f1c1a52-2a0e-4b6b-9a59-6a1d8f3f1c11":     http.StatusNotFound,
		"/api/presentaciones/" + id + "/slides/2":                      http.StatusNotFound,
		"/api/presentaciones/" + id + "/slides/..%2f..%2fetc%2fpasswd": http.StatusNotFound,
		"/api/presentaciones/" + id + "/slides/.staging":               http.StatusNotFound,
	}
	for path, want := range gets {
		if resp, data := get(t, srv.URL+path); resp.StatusCode != want {
			t.Fatalf("GET %s = %d (%s), se esperaba %d", path, resp.StatusCode, data, want)
		}
	}

	resp, err := http.Post(srv.URL+"/api/presentaciones/upload", "application/json", bytes.NewBufferString(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("upload sin multipart = %d, se esperaba 400", resp.StatusCode)
	}
}

// Una subida cortada a mitad (el cliente cierra antes de enviar todo el cuerpo) es un error del
// cliente: 400, no 500. Se usa una conexión TCP medio cerrada para poder leer la respuesta.
func TestTruncatedUploadIsAClientError(t *testing.T) {
	srv := newServer(t, 1<<20)

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("file", "deck.zip")
	_, _ = fw.Write(zipOf(t, map[string][]byte{"s1.png": pngOf(t, 1)}))
	_ = mw.Close()
	truncated := body.Bytes()[:body.Len()/2]

	conn, err := net.Dial("tcp", strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	fmt.Fprintf(conn, "POST /api/presentaciones/upload HTTP/1.1\r\nHost: test\r\nContent-Type: %s\r\nContent-Length: %d\r\n\r\n",
		mw.FormDataContentType(), body.Len())
	_, _ = conn.Write(truncated)
	_ = conn.(*net.TCPConn).CloseWrite()

	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		data, _ := io.ReadAll(resp.Body)
		t.Fatalf("estado = %d (%s), se esperaba 400", resp.StatusCode, data)
	}
}
