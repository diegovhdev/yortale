// Package server conecta las rutas con sus handlers y aplica los middleware.
package server

import (
	"io/fs"
	"net/http"

	"github.com/diegovhdev/yortale/internal/auth"
	"github.com/diegovhdev/yortale/internal/home"
	"github.com/diegovhdev/yortale/internal/legal"
	"github.com/diegovhdev/yortale/internal/platform/middleware"
	"github.com/diegovhdev/yortale/internal/stories"
	"github.com/diegovhdev/yortale/web"
)

type Deps struct {
	Auth          *auth.Service
	Stories       *stories.Service
	SecureCookies bool
}

func NewRouter(d Deps) http.Handler {
	mux := http.NewServeMux()
	authHandler := auth.NewHandler(d.Auth, d.SecureCookies)
	storyHandler := stories.NewHandler(d.Stories)

	// member envuelve las rutas que exigen sesión iniciada.
	member := func(h http.HandlerFunc) http.Handler { return auth.RequireUser(h) }

	// Archivos estáticos (css, js) incluidos dentro del binario con go:embed.
	static, _ := fs.Sub(web.StaticFS, "static")
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(static)))

	mux.HandleFunc("GET /{$}", home.Home)

	mux.HandleFunc("GET /register", authHandler.ShowRegister)
	mux.HandleFunc("POST /register", authHandler.Register)
	mux.HandleFunc("GET /login", authHandler.ShowLogin)
	mux.HandleFunc("POST /login", authHandler.Login)
	mux.HandleFunc("POST /logout", authHandler.Logout)

	// Historias. La URL canónica es /stories/{id}/{slug}; las rutas fijas como
	// /stories/{id}/edit tienen prioridad sobre {slug} porque son más específicas
	// (por eso Slugify nunca devuelve "edit" ni "delete").
	mux.HandleFunc("GET /stories", storyHandler.Explore)
	mux.Handle("GET /stories/new", member(storyHandler.New))
	mux.Handle("POST /stories", member(storyHandler.Create))
	mux.HandleFunc("GET /stories/{id}", storyHandler.RedirectToStory)
	mux.HandleFunc("GET /stories/{id}/{slug}", storyHandler.Show)
	mux.Handle("GET /stories/{id}/edit", member(storyHandler.Edit))
	mux.Handle("POST /stories/{id}/edit", member(storyHandler.Update))
	mux.Handle("GET /stories/{id}/delete", member(storyHandler.ConfirmDelete))
	mux.Handle("POST /stories/{id}/delete", member(storyHandler.Delete))
	mux.Handle("GET /my/stories", member(storyHandler.MyStories))

	// Capítulos. Se leen dentro de la URL de su historia; las acciones del autor
	// van bajo /chapters/{id} para no chocar con el patrón /stories/{id}/{slug}/...
	mux.HandleFunc("GET /stories/{id}/{slug}/chapters/{chapterID}", storyHandler.ReadChapter)
	mux.Handle("GET /stories/{id}/chapters/new", member(storyHandler.NewChapter))
	mux.Handle("POST /stories/{id}/chapters", member(storyHandler.CreateChapter))
	mux.Handle("GET /chapters/{id}/edit", member(storyHandler.EditChapter))
	mux.Handle("POST /chapters/{id}/edit", member(storyHandler.UpdateChapter))
	mux.Handle("GET /chapters/{id}/delete", member(storyHandler.ConfirmDeleteChapter))
	mux.Handle("POST /chapters/{id}/delete", member(storyHandler.DeleteChapter))
	mux.Handle("POST /chapters/{id}/move", member(storyHandler.MoveChapter))

	mux.HandleFunc("GET /terms", legal.Terms)
	mux.HandleFunc("GET /privacy", legal.Privacy)

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
