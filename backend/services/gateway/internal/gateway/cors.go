package gateway

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// CORS permite que un frontend servido desde otro origen (p. ej. Vite en
// http://localhost:5173) use la API. Responde él mismo a las peticiones preflight (OPTIONS).
// El WebSocket no usa CORS: su origen lo verifica el servicio realtime en el handshake.
func CORS(allowedOrigins []string) gin.HandlerFunc {
	allowed := make(map[string]bool, len(allowedOrigins))
	for _, o := range allowedOrigins {
		allowed[strings.TrimRight(o, "/")] = true
	}

	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin == "" {
			c.Next()
			return
		}
		h := c.Writer.Header()
		h.Add("Vary", "Origin")
		if !allowed[origin] && !allowed["*"] {
			c.Next() // sin cabeceras CORS: el navegador bloqueará la respuesta
			return
		}
		h.Set("Access-Control-Allow-Origin", origin)
		if c.Request.Method == http.MethodOptions && c.GetHeader("Access-Control-Request-Method") != "" {
			h.Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			h.Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			h.Set("Access-Control-Max-Age", "600")
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
