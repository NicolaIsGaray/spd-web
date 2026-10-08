package storage

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

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

func testLimits() Limits {
	return Limits{
		MaxUploadBytes: 10 << 20,
		MaxEntries:     100,
		MaxSlides:      50,
		MaxTotalBytes:  4 << 20,
	}
}
