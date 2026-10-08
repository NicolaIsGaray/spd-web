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
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"spd.web/internal/platform"
	"spd.web/services/presentations/internal/presentation"
	"spd.web/services/presentations/internal/storage"
)

const maxUpload = 1 << 20

// minimalPDF basta para pasar la comprobación de firma: el conversor de los tests es falso.
var minimalPDF = []byte("%PDF-1.7\n%%EOF\n")

// pageConverter simula el conversor: escribe las páginas indicadas como page-1.png, page-2.png...
type pageConverter struct{ pages [][]byte }

func (c pageConverter) ToImages(_ context.Context, _, outDir string, _ int) ([]string, error) {
	paths := make([]string, len(c.pages))
	for i, data := range c.pages {
		paths[i] = filepath.Join(outDir, "page-"+strconv.Itoa(i+1)+".png")
		if err := os.WriteFile(paths[i], data, 0o600); err != nil {
			return nil, err
		}
	}
	return paths, nil
}

// failingConverter simula un conversor que siempre falla con err.
type failingConverter struct{ err error }

func (c failingConverter) ToImages(context.Context, string, string, int) ([]string, error) {
	return nil, c.err
}

func newServer(t *testing.T, conv presentation.Converter) *httptest.Server {
	t.Helper()
	gin.SetMode(gin.TestMode)
	log := slog.New(slog.DiscardHandler)
	store, err := storage.Open(t.TempDir(), log)
	if err != nil {
		t.Fatal(err)
	}
	limits := storage.Limits{MaxUploadBytes: maxUpload, MaxEntries: 100, MaxSlides: 20, MaxTotalBytes: 4 << 20}
	engine := platform.NewEngine(log)
	New(presentation.NewService(store, conv, limits, log), maxUpload, log).Register(engine)
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

// pptxOf genera el PPTX mínimo que acepta el servicio: un paquete OOXML (un contenedor ZIP)
// con ppt/presentation.xml.
func pptxOf(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("ppt/presentation.xml")
	_, _ = w.Write([]byte("<p:presentation/>"))
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
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

	resp, err := http.Post(srv.URL+"/api/presentations/upload", mw.FormDataContentType(), &body)
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
	slide1, slide2 := pngOf(t, 1), pngOf(t, 2)
	srv := newServer(t, pageConverter{pages: [][]byte{slide1, slide2}})

	docs := []struct {
		filename string
		data     []byte
	}{
		{"informe.pdf", minimalPDF},
		{"deck.pptx", pptxOf(t)},
	}
	for _, doc := range docs {
		t.Run(doc.filename, func(t *testing.T) {
			resp, body := upload(t, srv, "file", doc.filename, doc.data)
			if resp.StatusCode != http.StatusCreated {
				t.Fatalf("upload: %d %v", resp.StatusCode, body)
			}
			id, _ := body["presentation_id"].(string)
			if id == "" || body["slide_count"].(float64) != 2 {
				t.Fatalf("respuesta = %v", body)
			}
			if loc := resp.Header.Get("Location"); loc != "/api/presentations/"+id {
				t.Fatalf("Location = %q", loc)
			}

			meta, data := get(t, srv.URL+"/api/presentations/"+id)
			if meta.StatusCode != http.StatusOK || !bytes.Contains(data, []byte(`"file":"002.png"`)) {
				t.Fatalf("metadatos: %d %s", meta.StatusCode, data)
			}

			for _, ref := range []string{"1", "001.png"} {
				img, data := get(t, srv.URL+"/api/presentations/"+id+"/slides/"+ref)
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
		})
	}
}

func TestErrorResponses(t *testing.T) {
	srv := newServer(t, pageConverter{pages: [][]byte{pngOf(t, 1)}})
	resp, body := upload(t, srv, "file", "deck.pdf", minimalPDF)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("upload: %d %v", resp.StatusCode, body)
	}
	id := body["presentation_id"].(string)

	uploads := []struct {
		name, field, filename string
		data                  []byte
		want                  int
	}{
		{"sin campo file", "documento", "deck.pdf", minimalPDF, http.StatusBadRequest},
		{"formato no soportado", "file", "deck.odp", []byte("odp"), http.StatusUnsupportedMediaType},
		{"pdf inválido", "file", "deck.pdf", []byte("no soy un pdf"), http.StatusBadRequest},
		{"pptx inválido", "file", "deck.pptx", minimalPDF, http.StatusBadRequest},
		{"demasiado grande", "file", "deck.pdf", make([]byte, 3<<20), http.StatusRequestEntityTooLarge},
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
		"/api/presentations/no-es-uuid":                               http.StatusBadRequest,
		"/api/presentations/6f1c1a52-2a0e-4b6b-9a59-6a1d8f3f1c11":     http.StatusNotFound,
		"/api/presentations/" + id + "/slides/2":                      http.StatusNotFound,
		"/api/presentations/" + id + "/slides/..%2f..%2fetc%2fpasswd": http.StatusNotFound,
		"/api/presentations/" + id + "/slides/.staging":               http.StatusNotFound,
	}
	for path, want := range gets {
		if resp, data := get(t, srv.URL+path); resp.StatusCode != want {
			t.Fatalf("GET %s = %d (%s), se esperaba %d", path, resp.StatusCode, data, want)
		}
	}

	resp, err := http.Post(srv.URL+"/api/presentations/upload", "application/json", bytes.NewBufferString(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("upload sin multipart = %d, se esperaba 400", resp.StatusCode)
	}
}

// Los fallos del conversor responden 503 (falta un binario) o 422 (documento que no se pudo
// convertir) con el mensaje genérico, sin filtrar rutas ni la salida de los procesos.
func TestConversionErrors(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
		msg  string
	}{
		{"sin conversor", fmt.Errorf(`%w (exec: "pdftoppm": executable file not found in $PATH)`, presentation.ErrConverterUnavailable),
			http.StatusServiceUnavailable, presentation.ErrConverterUnavailable.Error()},
		{"documento ilegible", fmt.Errorf("%w: pdftoppm: exit status 1: Syntax Error: /srv/uploads/.staging/x/source.pdf", presentation.ErrConversionFailed),
			http.StatusUnprocessableEntity, presentation.ErrConversionFailed.Error()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := newServer(t, failingConverter{err: tc.err})
			resp, body := upload(t, srv, "file", "deck.pdf", minimalPDF)
			if resp.StatusCode != tc.want || body["error"] != tc.msg {
				t.Fatalf("respuesta = %d %v, se esperaba %d {error: %q}", resp.StatusCode, body, tc.want, tc.msg)
			}
		})
	}
}

// Una subida cortada a mitad (el cliente cierra antes de enviar todo el cuerpo) es un error del
// cliente: 400, no 500. Se usa una conexión TCP medio cerrada para poder leer la respuesta.
func TestTruncatedUploadIsAClientError(t *testing.T) {
	srv := newServer(t, pageConverter{pages: [][]byte{pngOf(t, 1)}})

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("file", "deck.pdf")
	_, _ = fw.Write(append([]byte("%PDF-1.7\n"), bytes.Repeat([]byte("x"), 64<<10)...))
	_ = mw.Close()
	truncated := body.Bytes()[:body.Len()/2]

	conn, err := net.Dial("tcp", strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	fmt.Fprintf(conn, "POST /api/presentations/upload HTTP/1.1\r\nHost: test\r\nContent-Type: %s\r\nContent-Length: %d\r\n\r\n",
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
