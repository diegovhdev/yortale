// Package server conecta las rutas con sus handlers y aplica los middleware.
package server

import (
	"io/fs"
	"net/http"

	"github.com/diegovhdev/yortale/internal/auth"
	"github.com/diegovhdev/yortale/internal/home"
	"github.com/diegovhdev/yortale/internal/platform/middleware"
	"github.com/diegovhdev/yortale/web"
)

type Deps struct {
	Auth          *auth.Service
	SecureCookies bool
}

func NewRouter(d Deps) http.Handler {
	mux := http.NewServeMux()
	authHandler := auth.NewHandler(d.Auth, d.SecureCookies)

	// Archivos estáticos (css, js) incluidos dentro del binario con go:embed.
	static, _ := fs.Sub(web.StaticFS, "static")
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(static)))

	mux.HandleFunc("GET /{$}", home.Home)

	mux.HandleFunc("GET /registro", authHandler.ShowRegister)
	mux.HandleFunc("POST /registro", authHandler.Register)
	mux.HandleFunc("GET /iniciar-sesion", authHandler.ShowLogin)
	mux.HandleFunc("POST /iniciar-sesion", authHandler.Login)
	mux.HandleFunc("POST /cerrar-sesion", authHandler.Logout)

	// Protección CSRF de la librería estándar: rechaza POST/PUT/PATCH/DELETE de otros sitios
	// según Sec-Fetch-Site u Origin. Junto con SameSite=Lax en la cookie, evita usar tokens.
	csrf := http.NewCrossOriginProtection()
	csrf.SetDenyHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Petición de origen cruzado rechazada", http.StatusForbidden)
	}))

	// El orden importa: Logger envuelve todo, luego la protección CSRF y luego la sesión.
	var h http.Handler = mux
	h = auth.LoadUser(d.Auth, d.SecureCookies)(h)
	h = csrf.Handler(h)
	h = middleware.Logger(h)
	return h
}
