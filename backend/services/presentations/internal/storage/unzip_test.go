package storage

import (
	"archive/zip"
	"bytes"
	"compress/flate"
	"context"
	"errors"
	"hash/crc32"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func extract(t *testing.T, zipPath string, lim Limits) (string, []string, error) {
	t.Helper()
	dest := t.TempDir()
	names, err := ExtractImages(context.Background(), zipPath, dest, lim)
	return dest, names, err
}

func TestExtractImagesOrdersRenamesAndSkipsJunk(t *testing.T) {
	slide1, slide2, slide10 := pngOf(t, 1), jpegOf(t, 2), pngOf(t, 10)
	zipPath := writeZip(t,
		zipEntry{"deck/slide10.png", slide10},
		zipEntry{"deck/slide2.JPG", slide2},
		zipEntry{"deck/slide1.png", slide1},
		zipEntry{"__MACOSX/deck/._slide1.png", []byte("AppleDouble, no es una imagen")},
		zipEntry{".DS_Store", []byte("basura")},
		zipEntry{"deck/notas.txt", []byte("no es una diapositiva")},
	)

	dest, names, err := extract(t, zipPath, testLimits())
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"001.png", "002.jpg", "003.png"}; !slices.Equal(names, want) {
		t.Fatalf("nombres = %q, se esperaba %q", names, want)
	}
	for name, want := range map[string][]byte{"001.png": slide1, "002.jpg": slide2, "003.png": slide10} {
		got, err := os.ReadFile(filepath.Join(dest, name))
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("%s no tiene el contenido esperado (err=%v)", name, err)
		}
	}
	entries, _ := os.ReadDir(dest)
	if len(entries) != 3 {
		t.Fatalf("se escribieron %d archivos, se esperaban 3", len(entries))
	}
}

func TestExtractImagesUsesRealContentType(t *testing.T) {
	_, names, err := extract(t, writeZip(t, zipEntry{"foto.png", jpegOf(t, 3)}), testLimits())
	if err != nil {
		t.Fatal(err)
	}
	if names[0] != "001.jpg" {
		t.Fatalf("un JPEG con extensión .png debe guardarse como .jpg; nombre = %s", names[0])
	}
}

func TestExtractImagesRejectsZipSlip(t *testing.T) {
	parent := t.TempDir()
	dest := filepath.Join(parent, "dest")
	if err := os.Mkdir(dest, 0o750); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"../evil.png", "/etc/evil.png", `..\..\evil.png`, "a/../../evil.png"} {
		zipPath := writeZip(t, zipEntry{"ok.png", pngOf(t, 1)}, zipEntry{name, pngOf(t, 2)})
		_, err := ExtractImages(context.Background(), zipPath, dest, testLimits())
		if !errors.Is(err, ErrInvalidFile) {
			t.Fatalf("%q: err = %v, se esperaba ErrInvalidFile", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(parent, "evil.png")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("se escribió un archivo fuera del directorio de destino")
	}
}

func TestExtractImagesRejectsInvalidContent(t *testing.T) {
	cases := map[string]string{
		"imagen falsa": writeZip(t, zipEntry{"slide1.png", []byte("<html><script>alert(1)</script></html>")}),
		"sin imágenes": writeZip(t, zipEntry{"leeme.txt", []byte("hola")}),
		"zip vacío":    writeZip(t),
	}
	for name, zipPath := range cases {
		if _, _, err := extract(t, zipPath, testLimits()); !errors.Is(err, ErrInvalidFile) {
			t.Fatalf("%s: err = %v, se esperaba ErrInvalidFile", name, err)
		}
	}

	notZip := filepath.Join(t.TempDir(), "upload.zip")
	_ = os.WriteFile(notZip, []byte("esto no es un zip"), 0o600)
	if _, _, err := extract(t, notZip, testLimits()); !errors.Is(err, ErrInvalidFile) {
		t.Fatalf("no-zip: err = %v, se esperaba ErrInvalidFile", err)
	}
}

func TestExtractImagesEnforcesLimits(t *testing.T) {
	big := append(pngOf(t, 1), make([]byte, 2<<20)...) // PNG válido + 2 MiB de relleno (comprime muy bien)

	lim := testLimits()
	if _, _, err := extract(t, writeZip(t, zipEntry{"big.png", big}), lim); !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("imagen grande: err = %v, se esperaba ErrLimitExceeded", err)
	}

	lim = testLimits()
	lim.MaxImageBytes, lim.MaxTotalBytes = 3<<20, 3<<20
	zipPath := writeZip(t, zipEntry{"1.png", big}, zipEntry{"2.png", big})
	if _, _, err := extract(t, zipPath, lim); !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("total: err = %v, se esperaba ErrLimitExceeded", err)
	}

	lim = testLimits()
	lim.MaxSlides = 2
	zipPath = writeZip(t, zipEntry{"1.png", pngOf(t, 1)}, zipEntry{"2.png", pngOf(t, 2)}, zipEntry{"3.png", pngOf(t, 3)})
	if _, _, err := extract(t, zipPath, lim); !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("demasiadas imágenes: err = %v, se esperaba ErrLimitExceeded", err)
	}

	lim = testLimits()
	lim.MaxEntries = 2
	if _, _, err := extract(t, zipPath, lim); !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("demasiadas entradas: err = %v, se esperaba ErrLimitExceeded", err)
	}
}

// Una cabecera que MIENTE (declara pocos bytes y contiene muchos) no debe permitir escribir
// más allá del límite: se corta durante la copia.
func TestExtractImagesLyingHeader(t *testing.T) {
	payload := append(pngOf(t, 1), make([]byte, 4<<20)...)
	var compressed bytes.Buffer
	fw, _ := flate.NewWriter(&compressed, flate.BestCompression)
	_, _ = fw.Write(payload)
	_ = fw.Close()

	zipPath := filepath.Join(t.TempDir(), "bomb.zip")
	f, _ := os.Create(zipPath)
	zw := zip.NewWriter(f)
	w, err := zw.CreateRaw(&zip.FileHeader{
		Name:               "bomb.png",
		Method:             zip.Deflate,
		CRC32:              crc32.ChecksumIEEE(payload),
		CompressedSize64:   uint64(compressed.Len()),
		UncompressedSize64: 1024, // mentira
	})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.Write(compressed.Bytes())
	_ = zw.Close()
	_ = f.Close()

	dest, _, err := extract(t, zipPath, testLimits())
	if err == nil {
		t.Fatal("se esperaba un error con una cabecera que miente")
	}
	t.Logf("error obtenido: %v", err)
	var written int64
	entries, _ := os.ReadDir(dest)
	for _, e := range entries {
		info, _ := e.Info()
		written += info.Size()
	}
	if written > testLimits().MaxImageBytes+1 {
		t.Fatalf("se escribieron %d bytes pese al límite", written)
	}
}

func TestImportImagesSortsNaturallyAndMoves(t *testing.T) {
	src := t.TempDir()
	for name, width := range map[string]int{"page-1.png": 1, "page-2.png": 2, "page-10.png": 10} {
		if err := os.WriteFile(filepath.Join(src, name), pngOf(t, width), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	paths, _ := filepath.Glob(filepath.Join(src, "*.png"))
	dest := t.TempDir()

	names, err := ImportImages(paths, dest, testLimits())
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"001.png", "002.png", "003.png"}; !slices.Equal(names, want) {
		t.Fatalf("nombres = %q", names)
	}
	got, _ := os.ReadFile(filepath.Join(dest, "003.png"))
	if !bytes.Equal(got, pngOf(t, 10)) {
		t.Fatal("page-10 debería ser la tercera diapositiva")
	}

	lim := testLimits()
	lim.MaxSlides = 2
	if _, err := ImportImages([]string{"a", "b", "c"}, dest, lim); !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("err = %v, se esperaba ErrLimitExceeded", err)
	}
}

func TestSlideBaseNameKeepsLexicographicOrder(t *testing.T) {
	if got := slideBaseName(0, 12); got != "001" {
		t.Fatalf("slideBaseName(0, 12) = %s", got)
	}
	if got := slideBaseName(41, 1200); got != "0042" {
		t.Fatalf("slideBaseName(41, 1200) = %s: con más de 999 diapositivas hacen falta 4 dígitos", got)
	}
}
