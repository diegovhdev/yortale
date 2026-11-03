// Package server conecta las rutas con sus handlers y aplica los middleware.
package server

import (
	"net/http"

	"github.com/diegovhdev/yortale/internal/auth"
	"github.com/diegovhdev/yortale/internal/home"
	"github.com/diegovhdev/yortale/internal/legal"
	"github.com/diegovhdev/yortale/internal/moderation"
	"github.com/diegovhdev/yortale/internal/platform/middleware"
	"github.com/diegovhdev/yortale/internal/stories"
	"github.com/diegovhdev/yortale/web"
)

type Deps struct {
	Auth          *auth.Service
	Stories       *stories.Service
	Moderation    *moderation.Service
	SecureCookies bool
}

func NewRouter(d Deps) http.Handler {
	mux := http.NewServeMux()
	authHandler := auth.NewHandler(d.Auth, d.SecureCookies)
	storyHandler := stories.NewHandler(d.Stories)
	modHandler := moderation.NewHandler(d.Moderation)

	// member envuelve las rutas que exigen sesión iniciada; staff, las del equipo de moderación,
	// y admin, las que solo pueden usar los administradores. El servicio de moderación vuelve
	// a comprobar los permisos sobre cada cuenta (un moderador no puede suspender a otro).
	member := func(h http.HandlerFunc) http.Handler { return auth.RequireUser(h) }
	staff := func(h http.HandlerFunc) http.Handler {
		return auth.RequireRole(auth.RoleModerator, auth.RoleAdmin)(h)
	}
	admin := func(h http.HandlerFunc) http.Handler { return auth.RequireRole(auth.RoleAdmin)(h) }

	mux.HandleFunc("GET /{$}", home.Home)

	mux.HandleFunc("GET /register", authHandler.ShowRegister)
	mux.HandleFunc("POST /register", authHandler.Register)
	mux.HandleFunc("GET /login", authHandler.ShowLogin)
	mux.HandleFunc("POST /login", authHandler.Login)
	mux.HandleFunc("POST /logout", authHandler.Logout)
	mux.Handle("GET /settings/profile", member(authHandler.ShowProfileSettings))
	mux.Handle("POST /settings/profile", member(authHandler.UpdateProfile))
	mux.HandleFunc("GET /users/{username}", storyHandler.AuthorProfile)

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

	// Tags. /tags/{slug} acepta cualquier forma del nombre y redirige a su slug.
	mux.HandleFunc("GET /tags/{slug}", storyHandler.ShowTag)
	mux.Handle("GET /api/tags", member(storyHandler.SuggestTags))

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

	// Reportes de los lectores. La historia y el capítulo van en la query (o en el formulario)
	// y no en la ruta, para no competir con /stories/{id}/{slug}.
	mux.Handle("GET /reports/new", member(modHandler.ShowReport))
	mux.Handle("POST /reports", member(modHandler.CreateReport))
	mux.Handle("GET /reports/sent", member(modHandler.ReportSent))

	// Panel de moderación.
	mux.Handle("GET /moderation", staff(modHandler.Queue))
	mux.Handle("GET /moderation/log", staff(modHandler.Log))
	mux.Handle("GET /moderation/stories/{id}", staff(modHandler.ShowStory))
	mux.Handle("POST /moderation/stories/{id}/hide", staff(modHandler.HideStory))
	mux.Handle("POST /moderation/stories/{id}/unhide", staff(modHandler.UnhideStory))
	mux.Handle("POST /moderation/stories/{id}/dismiss", staff(modHandler.DismissReports))
	mux.Handle("GET /moderation/users/{username}", staff(modHandler.ShowUser))
	mux.Handle("POST /moderation/users/{username}/suspend", staff(modHandler.Suspend))
	mux.Handle("POST /moderation/users/{username}/unsuspend", staff(modHandler.Unsuspend))
	mux.Handle("POST /moderation/users/{username}/role", admin(modHandler.SetRole))

	mux.HandleFunc("GET /terms", legal.Terms)
	mux.HandleFunc("GET /privacy", legal.Privacy)

	// Protección CSRF de la librería estándar: rechaza POST/PUT/PATCH/DELETE de otros sitios
	// según Sec-Fetch-Site u Origin. Junto con SameSite=Lax en la cookie, evita usar tokens.
	csrf := http.NewCrossOriginProtection()
	csrf.SetDenyHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Petición de origen cruzado rechazada", http.StatusForbidden)
	}))

	// El orden importa: Logger envuelve todo, luego la protección CSRF y luego la sesión.
	var app http.Handler = mux
	app = auth.LoadUser(d.Auth, d.SecureCookies)(app)
	app = csrf.Handler(app)

	// Los archivos estáticos (css, js, incluidos en el binario con go:embed) van aparte para no
	// pasar por LoadUser: si no, cada archivo costaría una consulta de sesión a la base de datos.
	root := http.NewServeMux()
	root.Handle("GET /static/", http.StripPrefix("/static/", web.StaticHandler()))
	root.Handle("/", app)
	return middleware.Logger(root)
}
