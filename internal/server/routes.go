// Package server conecta las rutas con sus handlers y aplica los middleware.
package server

import (
	"io/fs"
	"net/http"

	"github.com/diegovhdev/yortale/internal/handler"
	"github.com/diegovhdev/yortale/internal/middleware"
	"github.com/diegovhdev/yortale/internal/service"
	"github.com/diegovhdev/yortale/web"
)

type Deps struct {
	Auth          *service.AuthService
	SecureCookies bool
}

func NewRouter(d Deps) http.Handler {
	mux := http.NewServeMux()
	auth := handler.NewAuthHandler(d.Auth, d.SecureCookies)

	// Archivos estáticos (css, js) incluidos dentro del binario con go:embed.
	static, _ := fs.Sub(web.StaticFS, "static")
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(static)))

	mux.HandleFunc("GET /{$}", handler.Home)

	mux.HandleFunc("GET /registro", auth.ShowRegister)
	mux.HandleFunc("POST /registro", auth.Register)
	mux.HandleFunc("GET /iniciar-sesion", auth.ShowLogin)
	mux.HandleFunc("POST /iniciar-sesion", auth.Login)
	mux.HandleFunc("POST /cerrar-sesion", auth.Logout)

	// El orden importa: Logger envuelve todo, luego la protección CSRF y luego la sesión.
	var h http.Handler = mux
	h = middleware.LoadUser(d.Auth, d.SecureCookies)(h)
	h = middleware.SameOrigin(h)
	h = middleware.Logger(h)
	return h
}
