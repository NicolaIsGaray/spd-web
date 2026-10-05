package presentation

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"spd.web/services/presentations/internal/storage"
)

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
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// fakeConverter simula LibreOffice: genera `pages` PNG con el nombre que usaría pdftoppm.
type fakeConverter struct {
	pages int
	err   error
}

func (f fakeConverter) ToImages(_ context.Context, _, outDir string, _ int) ([]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	var out []string
	for i := 1; i <= f.pages; i++ {
		p := filepath.Join(outDir, "page-"+strconv.Itoa(i)+".png")
		if err := os.WriteFile(p, nil, 0o600); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

type fixture struct {
	svc  *Service
	root string
}

func newFixture(t *testing.T, conv Converter) fixture {
	t.Helper()
	log := slog.New(slog.DiscardHandler)
	store, err := storage.Open(t.TempDir(), log)
	if err != nil {
		t.Fatal(err)
	}
	limits := storage.Limits{MaxUploadBytes: 1 << 20, MaxEntries: 100, MaxSlides: 20, MaxImageBytes: 1 << 20, MaxTotalBytes: 4 << 20}
	return fixture{svc: NewService(store, conv, limits, log), root: store.Root()}
}

// assertNoLeftovers comprueba que no quedan directorios de trabajo ni publicaciones parciales.
func (f fixture) assertNoLeftovers(t *testing.T, published int) {
	t.Helper()
	staging, _ := os.ReadDir(filepath.Join(f.root, ".staging"))
	if len(staging) != 0 {
		t.Fatalf("quedaron %d directorios de trabajo", len(staging))
	}
	entries, _ := os.ReadDir(f.root)
	if got := len(entries) - 1; got != published { // -1: el propio .staging
		t.Fatalf("presentaciones publicadas = %d, se esperaban %d", got, published)
	}
}

func TestCreateFromZip(t *testing.T) {
	f := newFixture(t, fakeConverter{})
	upload := zipOf(t, map[string][]byte{"b/slide2.png": pngOf(t, 2), "b/slide1.png": pngOf(t, 1), "b/slide10.png": pngOf(t, 10)})

	p, err := f.svc.Create(context.Background(), "Mi Presentación.ZIP", bytes.NewReader(upload))
	if err != nil {
		t.Fatal(err)
	}
	if p.SlideCount != 3 || p.Slides[2].File != "003.png" || p.Slides[2].URL != "/api/presentaciones/"+p.ID+"/slides/3" {
		t.Fatalf("presentación = %+v", p)
	}
	f.assertNoLeftovers(t, 1)

	got, err := f.svc.Get(p.ID)
	if err != nil || got.SlideCount != 3 {
		t.Fatalf("Get = %+v, %v", got, err)
	}

	byNumber, err := f.svc.SlideFile(p.ID, "3")
	if err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(byNumber); !bytes.Equal(data, pngOf(t, 10)) {
		t.Fatal("la diapositiva 3 debería ser slide10.png (orden natural)")
	}
	if byName, err := f.svc.SlideFile(p.ID, "003.png"); err != nil || byName != byNumber {
		t.Fatalf("por nombre: %s, %v", byName, err)
	}
	for _, ref := range []string{"0", "4", "-1", "+1", "../003.png", "004.png", ""} {
		if _, err := f.svc.SlideFile(p.ID, ref); !errors.Is(err, ErrSlideNotFound) {
			t.Fatalf("SlideFile(%q): err = %v, se esperaba ErrSlideNotFound", ref, err)
		}
	}
	if _, err := f.svc.Get("6f1c1a52-2a0e-4b6b-9a59-6a1d8f3f1c11"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get inexistente: err = %v", err)
	}
}

func TestCreateFromPPTX(t *testing.T) {
	f := newFixture(t, fakeConverterWithPNG{t: t, pages: 3})
	pptx := zipOf(t, map[string][]byte{"[Content_Types].xml": []byte("<Types/>"), "ppt/presentation.xml": []byte("<p:presentation/>")})

	p, err := f.svc.Create(context.Background(), "deck.pptx", bytes.NewReader(pptx))
	if err != nil {
		t.Fatal(err)
	}
	if p.SlideCount != 3 || p.Slides[0].File != "001.png" {
		t.Fatalf("presentación = %+v", p)
	}
	f.assertNoLeftovers(t, 1)
}

// fakeConverterWithPNG genera imágenes reales (el servicio verifica el contenido).
type fakeConverterWithPNG struct {
	t     *testing.T
	pages int
}

func (f fakeConverterWithPNG) ToImages(_ context.Context, _, outDir string, _ int) ([]string, error) {
	var out []string
	for i := 1; i <= f.pages; i++ {
		p := filepath.Join(outDir, "page-"+strconv.Itoa(i)+".png")
		if err := os.WriteFile(p, pngOf(f.t, i), 0o600); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

func TestCreateFailuresLeaveNothingBehind(t *testing.T) {
	validPPTX := zipOf(t, map[string][]byte{"ppt/presentation.xml": []byte("<p/>")})
	cases := []struct {
		name     string
		conv     Converter
		filename string
		body     []byte
		want     error
	}{
		{"formato no soportado", fakeConverter{}, "deck.pdf", []byte("%PDF"), ErrUnsupportedFormat},
		{"zip corrupto", fakeConverter{}, "deck.zip", []byte("no soy un zip"), ErrInvalidFile},
		{"zip sin imágenes", fakeConverter{}, "deck.zip", zipOf(t, map[string][]byte{"a.txt": []byte("x")}), ErrInvalidFile},
		{"pptx falso", fakeConverter{}, "deck.pptx", zipOf(t, map[string][]byte{"word/document.xml": nil}), ErrInvalidFile},
		{"pptx que no es zip", fakeConverter{}, "deck.pptx", []byte("hola"), ErrInvalidFile},
		{"archivo demasiado grande", fakeConverter{}, "deck.zip", make([]byte, 2<<20), ErrLimitExceeded},
		{"conversión fallida", fakeConverter{err: ErrConversionFailed}, "deck.pptx", validPPTX, ErrConversionFailed},
		{"sin LibreOffice", fakeConverter{err: ErrConverterUnavailable}, "deck.pptx", validPPTX, ErrConverterUnavailable},
		{"demasiadas diapositivas", fakeConverterWithPNG{t: t, pages: 21}, "deck.pptx", validPPTX, ErrLimitExceeded},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, tc.conv)
			_, err := f.svc.Create(context.Background(), tc.filename, bytes.NewReader(tc.body))
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, se esperaba %v", err, tc.want)
			}
			f.assertNoLeftovers(t, 0)
		})
	}
}
