package storage

import (
	"archive/zip"
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

type zipEntry struct {
	name string
	data []byte
}

// writeZip crea un ZIP en un directorio temporal con las entradas indicadas.
func writeZip(t *testing.T, entries ...zipEntry) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "upload.zip")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for _, e := range entries {
		w, err := zw.Create(e.name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(e.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

// pngOf genera un PNG válido; el ancho permite distinguir imágenes entre sí.
func pngOf(t *testing.T, width int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, 4))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func jpegOf(t *testing.T, width int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, image.NewGray(image.Rect(0, 0, width, 4)), nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func testLimits() Limits {
	return Limits{
		MaxUploadBytes: 10 << 20,
		MaxEntries:     100,
		MaxSlides:      50,
		MaxImageBytes:  1 << 20,
		MaxTotalBytes:  4 << 20,
	}
}
