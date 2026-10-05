package convert

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// Tests de integración: usan LibreOffice y pdftoppm reales. Se omiten con -short o si los
// binarios no están instalados.

func requireBinaries(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("test de integración: omitido con -short")
	}
	for _, bin := range []string{"soffice", "pdftoppm"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s no está instalado", bin)
		}
	}
}

var (
	fixtureOnce sync.Once
	fixturePath string
	fixtureErr  error
)

// samplePPTX genera (una vez por ejecución) un PPTX real de 3 diapositivas: LibreOffice
// convierte un ODP plano (FODP) escrito a mano.
func samplePPTX(t *testing.T) string {
	t.Helper()
	fixtureOnce.Do(func() {
		dir, err := os.MkdirTemp("", "spd-pptx-fixture-*")
		if err != nil {
			fixtureErr = err
			return
		}
		src := filepath.Join(dir, "deck.fodp")
		if fixtureErr = os.WriteFile(src, []byte(fodp(3)), 0o600); fixtureErr != nil {
			return
		}
		cmd := exec.Command("soffice", "--headless", "--norestore", "--nolockcheck",
			"-env:UserInstallation="+fileURL(filepath.Join(dir, "profile")),
			"--convert-to", "pptx", "--outdir", dir, src)
		if out, err := cmd.CombinedOutput(); err != nil {
			fixtureErr = fmt.Errorf("generar PPTX: %v: %s", err, out)
			return
		}
		fixturePath = filepath.Join(dir, "deck.pptx")
	})
	if fixtureErr != nil {
		t.Fatal(fixtureErr)
	}
	return fixturePath
}

func fodp(slides int) string {
	var pages strings.Builder
	for i := 1; i <= slides; i++ {
		fmt.Fprintf(&pages, `<draw:page draw:name="s%d" draw:master-page-name="Default">`+
			`<draw:frame svg:x="2cm" svg:y="2cm" svg:width="20cm" svg:height="4cm">`+
			`<draw:text-box><text:p>Diapositiva %d</text:p></draw:text-box></draw:frame></draw:page>`, i, i)
	}
	return `<?xml version="1.0" encoding="UTF-8"?>
<office:document xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0"
 xmlns:style="urn:oasis:names:tc:opendocument:xmlns:style:1.0"
 xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0"
 xmlns:draw="urn:oasis:names:tc:opendocument:xmlns:drawing:1.0"
 xmlns:fo="urn:oasis:names:tc:opendocument:xmlns:xsl-fo-compatible:1.0"
 xmlns:svg="urn:oasis:names:tc:opendocument:xmlns:svg-compatible:1.0"
 office:version="1.3" office:mimetype="application/vnd.oasis.opendocument.presentation">
 <office:automatic-styles><style:page-layout style:name="PM1">
  <style:page-layout-properties fo:page-width="28cm" fo:page-height="15.75cm" style:print-orientation="landscape"/>
 </style:page-layout></office:automatic-styles>
 <office:master-styles><style:master-page style:name="Default" style:page-layout-name="PM1"/></office:master-styles>
 <office:body><office:presentation>` + pages.String() + `</office:presentation></office:body>
</office:document>`
}

func newConverter(t *testing.T, opts Options) *LibreOffice {
	t.Helper()
	if opts.SofficeBin == "" {
		opts.SofficeBin = "soffice"
	}
	if opts.PdftoppmBin == "" {
		opts.PdftoppmBin = "pdftoppm"
	}
	if opts.Timeout == 0 {
		opts.Timeout = 2 * time.Minute
	}
	if opts.MaxPixels == 0 {
		opts.MaxPixels = 640
	}
	lo, err := NewLibreOffice(opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lo.Close() })
	return lo
}

func copyFixture(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(samplePPTX(t))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "source.pptx")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestToImagesRendersEverySlide(t *testing.T) {
	requireBinaries(t)
	lo := newConverter(t, Options{MaxConcurrent: 1})
	src := copyFixture(t)

	pages, err := lo.ToImages(context.Background(), src, filepath.Dir(src), 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 3 {
		t.Fatalf("páginas = %d (%v), se esperaban 3", len(pages), pages)
	}
	for _, p := range pages {
		data, _ := os.ReadFile(p)
		cfg, err := png.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("%s no es un PNG válido: %v", p, err)
		}
		if max(cfg.Width, cfg.Height) != 640 {
			t.Fatalf("%s mide %dx%d; el lado mayor debería ser 640", p, cfg.Width, cfg.Height)
		}
	}
}

// Dos conversiones simultáneas deben funcionar: cada una usa su propio perfil de LibreOffice.
func TestConcurrentConversionsUseIsolatedProfiles(t *testing.T) {
	requireBinaries(t)
	lo := newConverter(t, Options{MaxConcurrent: 2})

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for range 2 {
		src := copyFixture(t)
		wg.Go(func() {
			pages, err := lo.ToImages(context.Background(), src, filepath.Dir(src), 100)
			if err == nil && len(pages) != 3 {
				err = fmt.Errorf("páginas = %d, se esperaban 3", len(pages))
			}
			errs <- err
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestToImagesStopsAfterMaxPagesPlusOne(t *testing.T) {
	requireBinaries(t)
	lo := newConverter(t, Options{MaxConcurrent: 1})
	src := copyFixture(t)

	pages, err := lo.ToImages(context.Background(), src, filepath.Dir(src), 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 2 {
		t.Fatalf("con maxPages=1 se esperaban 2 páginas (para detectar el exceso), hay %d", len(pages))
	}
}

func TestMissingBinaries(t *testing.T) {
	lo := newConverter(t, Options{SofficeBin: "soffice-que-no-existe", MaxConcurrent: 1})
	if err := lo.Check(); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Check: err = %v, se esperaba ErrUnavailable", err)
	}
	_, err := lo.ToImages(context.Background(), "x.pptx", t.TempDir(), 10)
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("ToImages: err = %v, se esperaba ErrUnavailable", err)
	}
}

// Un timeout debe matar TODO el grupo de procesos, no solo al hijo directo: soffice es un
// script que lanza otros procesos. Se simula con un "soffice" falso que deja un nieto vivo,
// el caso exacto en que matar solo al hijo dejaría un huérfano. No necesita LibreOffice.
func TestTimeoutKillsTheWholeProcessGroup(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("la comprobación de procesos huérfanos usa /proc")
	}
	fake := filepath.Join(t.TempDir(), "fake-soffice")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\nsleep 300 &\nwait\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	lo := newConverter(t, Options{SofficeBin: fake, MaxConcurrent: 1, Timeout: 500 * time.Millisecond})

	start := time.Now()
	_, err := lo.ToImages(context.Background(), "source.pptx", t.TempDir(), 10)
	if !errors.Is(err, ErrFailed) || !strings.Contains(err.Error(), "tiempo agotado") {
		t.Fatalf("err = %v, se esperaba ErrFailed por tiempo agotado", err)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("ToImages tardó %v en volver tras el timeout", elapsed)
	}
	assertNoOrphans(t, lo.profileRoot)
}

// Lo mismo con LibreOffice real: un perfil nuevo tarda varios segundos en inicializarse, así
// que 1,5 s garantiza que el timeout llega a mitad de la conversión.
func TestTimeoutWithRealLibreOffice(t *testing.T) {
	requireBinaries(t)
	if runtime.GOOS != "linux" {
		t.Skip("la comprobación de procesos huérfanos usa /proc")
	}
	lo := newConverter(t, Options{MaxConcurrent: 1, Timeout: 1500 * time.Millisecond})
	src := copyFixture(t)

	_, err := lo.ToImages(context.Background(), src, filepath.Dir(src), 100)
	if !errors.Is(err, ErrFailed) || !strings.Contains(err.Error(), "tiempo agotado") {
		t.Fatalf("err = %v, se esperaba ErrFailed por tiempo agotado", err)
	}
	assertNoOrphans(t, lo.profileRoot)
}

// assertNoOrphans falla si queda vivo algún proceso lanzado por el conversor. Todos heredan
// HOME=<perfil> (ver LibreOffice.run), así que se buscan en /proc/*/environ.
func assertNoOrphans(t *testing.T, profileRoot string) {
	t.Helper()
	needle := []byte("HOME=" + profileRoot)
	deadline := time.Now().Add(3 * time.Second)
	for {
		var left []string
		entries, _ := os.ReadDir("/proc")
		for _, e := range entries {
			environ, err := os.ReadFile(filepath.Join("/proc", e.Name(), "environ"))
			if err != nil || !bytes.Contains(environ, needle) {
				continue
			}
			cmdline, _ := os.ReadFile(filepath.Join("/proc", e.Name(), "cmdline"))
			left = append(left, e.Name()+": "+string(bytes.ReplaceAll(cmdline, []byte{0}, []byte{' '})))
		}
		if len(left) == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("quedaron procesos huérfanos tras el timeout: %v", left)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
