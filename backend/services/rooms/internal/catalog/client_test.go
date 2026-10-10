package catalog

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"spd.web/services/rooms/internal/domain"
)

const pid = "6f1c1a52-2a0e-4b6b-9a59-6a1d8f3f1c11"

func TestSlideCount(t *testing.T) {
	cases := []struct {
		name    string
		status  int
		body    string
		want    int
		wantErr error
	}{
		{"ok", http.StatusOK, `{"presentation_id":"x","slide_count":7,"slides":[]}`, 7, nil},
		{"no existe", http.StatusNotFound, `{"error":"presentación no encontrada"}`, 0, domain.ErrPresentationUnknown},
		{"error interno", http.StatusInternalServerError, `{}`, 0, errAny},
		{"JSON inválido", http.StatusOK, `{"slide_count":`, 0, errAny},
		{"sin diapositivas", http.StatusOK, `{"slide_count":0}`, 0, errAny},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/presentations/"+pid {
					t.Errorf("ruta consultada = %s", r.URL.Path)
				}
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			base, _ := url.Parse(srv.URL)

			got, err := New(base, time.Second).SlideCount(context.Background(), pid)
			switch {
			case tc.wantErr == nil && (err != nil || got != tc.want):
				t.Fatalf("SlideCount = %d, %v; se esperaba %d", got, err, tc.want)
			case tc.wantErr == errAny && (err == nil || errors.Is(err, domain.ErrPresentationUnknown)):
				t.Fatalf("err = %v; se esperaba un error que no fuera ErrPresentationUnknown", err)
			case tc.wantErr != nil && tc.wantErr != errAny && !errors.Is(err, tc.wantErr):
				t.Fatalf("err = %v, se esperaba %v", err, tc.wantErr)
			}
		})
	}
}

func TestSlideCountUnreachable(t *testing.T) {
	base, _ := url.Parse("http://127.0.0.1:1") // puerto sin servicio
	if _, err := New(base, time.Second).SlideCount(context.Background(), pid); err == nil {
		t.Fatal("se esperaba un error de conexión")
	}
}

var errAny = errors.New("cualquier error")
