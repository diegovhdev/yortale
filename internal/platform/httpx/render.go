// Package httpx contiene las utilidades HTTP que comparten los handlers de todas las features.
package httpx

import (
	"bytes"
	"compress/gzip"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/a-h/templ"
)

// minGzipBytes es el tamaño a partir del cual vale la pena comprimir: por debajo, la cabecera
// gzip y el trabajo de comprimir cuestan más de lo que se ahorra (fragmentos htmx pequeños).
const minGzipBytes = 1024

// gzipWriters reutiliza los compresores, que reservan bastante memoria al crearse.
var gzipWriters = sync.Pool{New: func() any { return gzip.NewWriter(io.Discard) }}

// Render dibuja un componente templ con el código de estado indicado.
// Se renderiza primero en un buffer para no enviar HTML a medias si algo falla.
// Si el cliente lo acepta y la página es grande, se envía comprimida con gzip.
func Render(w http.ResponseWriter, r *http.Request, status int, c templ.Component) {
	var buf bytes.Buffer
	if err := c.Render(r.Context(), &buf); err != nil {
		slog.Error("renderizar plantilla", "err", err)
		http.Error(w, "Error interno", http.StatusInternalServerError)
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	// La misma URL devuelve una página completa o solo un fragmento según HX-Request,
	// y comprimida o no según Accept-Encoding.
	h.Add("Vary", "HX-Request")
	h.Add("Vary", "Accept-Encoding")

	body := buf.Bytes()
	if len(body) >= minGzipBytes && AcceptsGzip(r) {
		var zbuf bytes.Buffer
		zw := gzipWriters.Get().(*gzip.Writer)
		zw.Reset(&zbuf)
		_, _ = zw.Write(body)
		_ = zw.Close()
		gzipWriters.Put(zw)
		body = zbuf.Bytes()
		h.Set("Content-Encoding", "gzip")
	}
	h.Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

// AcceptsGzip indica si el cliente acepta respuestas comprimidas con gzip.
func AcceptsGzip(r *http.Request) bool {
	for _, part := range strings.Split(r.Header.Get("Accept-Encoding"), ",") {
		enc, params, _ := strings.Cut(part, ";")
		if !strings.EqualFold(strings.TrimSpace(enc), "gzip") {
			continue
		}
		// "gzip;q=0" significa que el cliente lo rechaza explícitamente.
		return strings.ReplaceAll(params, " ", "") != "q=0"
	}
	return false
}

// IsHTMX indica si la petición la hizo htmx (y por tanto espera un fragmento).
func IsHTMX(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}

// Redirect envía al usuario a otra URL. Con htmx se usa el header HX-Redirect
// para que el navegador haga una navegación completa; sin htmx, un 303 normal.
func Redirect(w http.ResponseWriter, r *http.Request, url string) {
	if IsHTMX(r) {
		w.Header().Set("HX-Redirect", url)
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Redirect(w, r, url, http.StatusSeeOther)
}
