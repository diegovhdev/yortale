package web

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/diegovhdev/yortale/internal/platform/httpx"
)

// asset es un archivo estático preparado al arrancar: su contenido, su hash
// y, si comprimido ocupa menos, su versión gzip.
type asset struct {
	data        []byte
	gz          []byte
	hash        string
	contentType string
}

// assets se calcula una sola vez, la primera vez que se pide un archivo o una URL.
// El contenido está incluido en el binario, así que no cambia mientras el servidor corre.
var assets = sync.OnceValue(func() map[string]*asset {
	m := map[string]*asset{}
	err := fs.WalkDir(StaticFS, "static", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := StaticFS.ReadFile(p)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		a := &asset{
			data:        data,
			hash:        hex.EncodeToString(sum[:6]),
			contentType: mime.TypeByExtension(path.Ext(p)),
		}
		if strings.HasPrefix(a.contentType, "text/") || strings.Contains(a.contentType, "javascript") {
			var buf bytes.Buffer
			zw, _ := gzip.NewWriterLevel(&buf, gzip.BestCompression)
			_, _ = zw.Write(data)
			_ = zw.Close()
			if buf.Len() < len(data) {
				a.gz = buf.Bytes()
			}
		}
		m[strings.TrimPrefix(p, "static/")] = a
		return nil
	})
	if err != nil {
		// Solo puede fallar si el embed está mal hecho, y eso se ve en cuanto arranca.
		panic("leer archivos estáticos: " + err.Error())
	}
	return m
})

// Asset devuelve la URL de un archivo estático (ruta relativa a web/static) con el hash
// de su contenido en ?v=. Al cambiar el archivo cambia la URL, así que el navegador
// puede guardarlo en caché para siempre sin quedarse con una versión vieja.
func Asset(name string) string {
	if a, ok := assets()[name]; ok {
		return "/static/" + name + "?v=" + a.hash
	}
	return "/static/" + name
}

// StaticHandler sirve los archivos de web/static. Debe montarse con http.StripPrefix("/static/", …).
// Con el ?v= correcto se cachean un año; sin él, el navegador revalida con el ETag (304).
func StaticHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a, ok := assets()[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		h := w.Header()
		h.Set("Content-Type", a.contentType)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Add("Vary", "Accept-Encoding")
		if r.URL.Query().Get("v") == a.hash {
			h.Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			h.Set("Cache-Control", "no-cache")
		}

		data, etag := a.data, `"`+a.hash+`"`
		if a.gz != nil && httpx.AcceptsGzip(r) {
			data, etag = a.gz, `"`+a.hash+`-gz"`
			h.Set("Content-Encoding", "gzip")
		}
		h.Set("ETag", etag)
		// ServeContent responde 304 si If-None-Match coincide con el ETag.
		http.ServeContent(w, r, r.URL.Path, time.Time{}, bytes.NewReader(data))
	})
}
