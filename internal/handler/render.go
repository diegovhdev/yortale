// Package handler recibe las peticiones HTTP, llama a los servicios y responde con HTML.
package handler

import (
	"bytes"
	"log/slog"
	"net/http"

	"github.com/a-h/templ"
)

// render dibuja un componente templ con el código de estado indicado.
// Se renderiza primero en un buffer para no enviar HTML a medias si algo falla.
func render(w http.ResponseWriter, r *http.Request, status int, c templ.Component) {
	var buf bytes.Buffer
	if err := c.Render(r.Context(), &buf); err != nil {
		slog.Error("renderizar plantilla", "err", err)
		http.Error(w, "Error interno", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// La misma URL devuelve una página completa o solo un fragmento según HX-Request.
	w.Header().Add("Vary", "HX-Request")
	w.WriteHeader(status)
	_, _ = buf.WriteTo(w)
}

// isHTMX indica si la petición la hizo htmx (y por tanto espera un fragmento).
func isHTMX(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}

// redirect envía al usuario a otra URL. Con htmx se usa el header HX-Redirect
// para que el navegador haga una navegación completa; sin htmx, un 303 normal.
func redirect(w http.ResponseWriter, r *http.Request, url string) {
	if isHTMX(r) {
		w.Header().Set("HX-Redirect", url)
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Redirect(w, r, url, http.StatusSeeOther)
}
