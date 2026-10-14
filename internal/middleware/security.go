package middleware

import (
	"log/slog"
	"net/http"
	"net/url"
	"time"
)

// SameOrigin rechaza peticiones POST/PUT/PATCH/DELETE que vengan de otro sitio web.
// Junto con SameSite=Lax en la cookie, esto protege contra ataques CSRF sin
// necesidad de tokens en los formularios.
//
// Si usas Go 1.25 o superior puedes reemplazarlo por http.NewCrossOriginProtection(),
// que hace la misma comprobación y viene en la librería estándar.
func SameOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}

		// Navegadores modernos envían Sec-Fetch-Site.
		if site := r.Header.Get("Sec-Fetch-Site"); site != "" {
			if site != "same-origin" && site != "none" {
				http.Error(w, "Petición de origen cruzado rechazada", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
			return
		}
		// Navegadores antiguos: comparar el header Origin con el host.
		if origin := r.Header.Get("Origin"); origin != "" {
			if u, err := url.Parse(origin); err != nil || u.Host != r.Host {
				http.Error(w, "Petición de origen cruzado rechazada", http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// statusRecorder guarda el código de estado para poder registrarlo en el log.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// Logger registra cada petición con su método, ruta, estado y duración.
func Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		slog.Info("http", "method", r.Method, "path", r.URL.Path,
			"status", rec.status, "duration", time.Since(start).Round(time.Millisecond))
	})
}
