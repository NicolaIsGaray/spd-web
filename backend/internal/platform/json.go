package platform

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// BindJSON decodifica en dst el cuerpo JSON de la petición, de como mucho maxBytes, y, si no
// puede, responde 400 (o 413 si el cuerpo es demasiado grande). Rechaza los campos desconocidos
// (un "fullname" mal escrito no debe ignorarse en silencio) y los datos después del objeto.
func BindJSON(c *gin.Context, dst any, maxBytes int64) bool {
	dec := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes))
	dec.DisallowUnknownFields()
	err := dec.Decode(dst)
	if err == nil && dec.More() {
		err = errors.New("hay datos después del objeto JSON")
	}
	if err == nil {
		return true
	}
	var tooBig *http.MaxBytesError
	if errors.As(err, &tooBig) {
		WriteError(c, http.StatusRequestEntityTooLarge, "el cuerpo supera "+FormatBytes(maxBytes))
	} else {
		WriteError(c, http.StatusBadRequest, "cuerpo JSON inválido: "+jsonError(err))
	}
	return false
}

// jsonError describe en español los errores habituales de encoding/json, sin nombres de tipos
// internos de Go.
func jsonError(err error) string {
	var typeErr *json.UnmarshalTypeError
	var syntaxErr *json.SyntaxError
	switch {
	case errors.Is(err, io.EOF):
		return "falta el cuerpo"
	case errors.As(err, &typeErr) && typeErr.Field == "":
		return "el cuerpo debe ser un objeto JSON"
	case errors.As(err, &typeErr):
		return fmt.Sprintf("el campo %s debe ser de tipo %s", typeErr.Field, typeErr.Type)
	case errors.As(err, &syntaxErr), errors.Is(err, io.ErrUnexpectedEOF):
		return "JSON mal formado"
	case strings.HasPrefix(err.Error(), "json: unknown field "):
		return "campo desconocido " + strings.TrimPrefix(err.Error(), "json: unknown field ")
	}
	return err.Error()
}

// FormatBytes formatea un tamaño para mensajes de error ("100 MB", "1 KB").
func FormatBytes(n int64) string {
	const kb, mb = 1 << 10, 1 << 20
	switch {
	case n >= mb && n%mb == 0:
		return fmt.Sprintf("%d MB", n/mb)
	case n >= kb && n%kb == 0:
		return fmt.Sprintf("%d KB", n/kb)
	}
	return fmt.Sprintf("%d bytes", n)
}
