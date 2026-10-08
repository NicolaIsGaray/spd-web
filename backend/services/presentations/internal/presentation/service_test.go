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

// minimalPDF basta para pasar la comprobación de firma: el conversor de los tests es falso.
var minimalPDF = []byte("%PDF-1.7\n%%EOF\n")

func pngOf(t *testing.T, width int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewGray(image.Rect(0, 0, width, 2))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// pngPages genera n páginas distintas entre sí: la página i mide i píxeles de ancho.
func pngPages(t *testing.T, n int) [][]byte {
	t.Helper()
	pages := make([][]byte, n)
	for i := range pages {
		pages[i] = pngOf(t, i+1)
	}
	return pages
}

// ooxmlOf empaqueta un documento OOXML: un contenedor ZIP con las partes indicadas.
func ooxmlOf(t *testing.T, parts map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, data := range parts {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// pptxOf genera el PPTX mínimo que acepta checkPPTX.
func pptxOf(t *testing.T) []byte {
	t.Helper()
	return ooxmlOf(t, map[string][]byte{"[Content_Types].xml": []byte("<Types/>"), "ppt/presentation.xml": []byte("<p:presentation/>")})
}

// fakeConverter simula LibreOffice y pdftoppm: escribe las páginas indicadas con los nombres
// que usa pdftoppm (page-1.png, page-2.png...) y las devuelve en el orden de filepath.Glob, que
// no es el de las páginas (page-10 va antes que page-2), o falla con err. Recuerda qué
// documento recibió: el conversor real elige el camino (PPTX o PDF) por su extensión.
type fakeConverter struct {
	pages [][]byte
	err   error

	calls  int
	gotExt string
	gotDoc []byte
}

func (f *fakeConverter) ToImages(_ context.Context, src, outDir string, _ int) ([]string, error) {
	f.calls++
	f.gotExt = filepath.Ext(src)
	f.gotDoc, _ = os.ReadFile(src)
	if f.err != nil {
		return nil, f.err
	}
	for i, data := range f.pages {
		if err := os.WriteFile(filepath.Join(outDir, "page-"+strconv.Itoa(i+1)+".png"), data, 0o600); err != nil {
			return nil, err
		}
	}
	return filepath.Glob(filepath.Join(outDir, "page-*.png"))
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
	limits := storage.Limits{MaxUploadBytes: 1 << 20, MaxEntries: 100, MaxSlides: 20, MaxTotalBytes: 4 << 20}
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

func TestCreateFromPDF(t *testing.T) {
	conv := &fakeConverter{pages: pngPages(t, 12)}
	f := newFixture(t, conv)

	p, err := f.svc.Create(context.Background(), "Informe Anual.PDF", bytes.NewReader(minimalPDF))
	if err != nil {
		t.Fatal(err)
	}
	if conv.gotExt != ".pdf" || !bytes.Equal(conv.gotDoc, minimalPDF) {
		t.Fatalf("el conversor recibió %q (%q), se esperaba el PDF subido", conv.gotExt, conv.gotDoc)
	}
	if p.SlideCount != 12 || p.Slides[11].File != "012.png" || p.Slides[11].URL != "/api/presentations/"+p.ID+"/slides/12" {
		t.Fatalf("presentación = %+v", p)
	}
	f.assertNoLeftovers(t, 1)

	got, err := f.svc.Get(p.ID)
	if err != nil || got.SlideCount != 12 {
		t.Fatalf("Get = %+v, %v", got, err)
	}

	byNumber, err := f.svc.SlideFile(p.ID, "10")
	if err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(byNumber); !bytes.Equal(data, pngOf(t, 10)) {
		t.Fatal("la diapositiva 10 debería ser page-10.png (orden natural, no el del conversor)")
	}
	if byName, err := f.svc.SlideFile(p.ID, "010.png"); err != nil || byName != byNumber {
		t.Fatalf("por nombre: %s, %v", byName, err)
	}
	for _, ref := range []string{"0", "13", "-1", "+1", "../010.png", "013.png", ""} {
		if _, err := f.svc.SlideFile(p.ID, ref); !errors.Is(err, ErrSlideNotFound) {
			t.Fatalf("SlideFile(%q): err = %v, se esperaba ErrSlideNotFound", ref, err)
		}
	}
	if _, err := f.svc.Get("6f1c1a52-2a0e-4b6b-9a59-6a1d8f3f1c11"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get inexistente: err = %v", err)
	}
}

func TestCreateFromPPTX(t *testing.T) {
	conv := &fakeConverter{pages: pngPages(t, 3)}
	f := newFixture(t, conv)
	pptx := pptxOf(t)

	p, err := f.svc.Create(context.Background(), "deck.pptx", bytes.NewReader(pptx))
	if err != nil {
		t.Fatal(err)
	}
	if conv.gotExt != ".pptx" || !bytes.Equal(conv.gotDoc, pptx) {
		t.Fatalf("el conversor recibió %q, se esperaba el PPTX subido", conv.gotExt)
	}
	if p.SlideCount != 3 || p.Slides[0].File != "001.png" {
		t.Fatalf("presentación = %+v", p)
	}
	f.assertNoLeftovers(t, 1)
}

// Solo se admiten .pptx y .pdf. El formato lo decide la extensión: un PDF real con otra
// extensión también se rechaza.
func TestCreateRejectsOtherFormats(t *testing.T) {
	conv := &fakeConverter{pages: pngPages(t, 1)}
	f := newFixture(t, conv)
	for _, name := range []string{"diapositivas.zip", "deck.ppt", "deck.odp", "slide.png", "deck.pdf.exe", "pdf", ""} {
		if _, err := f.svc.Create(context.Background(), name, bytes.NewReader(minimalPDF)); !errors.Is(err, ErrUnsupportedFormat) {
			t.Fatalf("Create(%q): err = %v, se esperaba ErrUnsupportedFormat", name, err)
		}
	}
	if conv.calls != 0 {
		t.Fatal("un formato no admitido no debe llegar al conversor")
	}
	f.assertNoLeftovers(t, 0)
}

func TestCreateFailuresLeaveNothingBehind(t *testing.T) {
	pptx := pptxOf(t)
	manyParts := map[string][]byte{"ppt/presentation.xml": nil}
	for i := range 100 {
		manyParts["ppt/slides/slide"+strconv.Itoa(i+1)+".xml"] = nil
	}
	cases := []struct {
		name     string
		conv     *fakeConverter // nil: el archivo no debe llegar al conversor
		filename string
		body     []byte
		want     error
	}{
		{"pdf vacío", nil, "deck.pdf", nil, ErrInvalidFile},
		{"pptx renombrado a .pdf", nil, "deck.pdf", pptx, ErrInvalidFile},
		{"pdf renombrado a .pptx", nil, "deck.pptx", minimalPDF, ErrInvalidFile},
		{"documento de Word renombrado a .pptx", nil, "deck.pptx", ooxmlOf(t, map[string][]byte{"word/document.xml": nil}), ErrInvalidFile},
		{"pptx con demasiadas partes", nil, "deck.pptx", ooxmlOf(t, manyParts), ErrLimitExceeded},
		{"pptx que se descomprime en exceso", nil, "deck.pptx", ooxmlOf(t, map[string][]byte{"ppt/presentation.xml": nil, "ppt/media/fondo.png": make([]byte, 5<<20)}), ErrLimitExceeded},
		{"archivo demasiado grande", nil, "deck.pdf", append([]byte("%PDF-1.7\n"), make([]byte, 2<<20)...), ErrLimitExceeded},
		{"conversión fallida", &fakeConverter{err: ErrConversionFailed}, "deck.pptx", pptx, ErrConversionFailed},
		{"sin pdftoppm", &fakeConverter{err: ErrConverterUnavailable}, "deck.pdf", minimalPDF, ErrConverterUnavailable},
		{"demasiadas diapositivas", &fakeConverter{pages: pngPages(t, 21)}, "deck.pptx", pptx, ErrLimitExceeded},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			conv := tc.conv
			if conv == nil {
				conv = &fakeConverter{}
			}
			f := newFixture(t, conv)
			_, err := f.svc.Create(context.Background(), tc.filename, bytes.NewReader(tc.body))
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, se esperaba %v", err, tc.want)
			}
			if tc.conv == nil && conv.calls != 0 {
				t.Fatal("el archivo no debía llegar al conversor")
			}
			f.assertNoLeftovers(t, 0)
		})
	}
}
