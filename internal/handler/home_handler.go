package handler

import (
	"net/http"

	"github.com/diegovhdev/yortale/internal/middleware"
	"github.com/diegovhdev/yortale/web/pages"
)

// GET /  — página temporal para comprobar que la sesión funciona.
func Home(w http.ResponseWriter, r *http.Request) {
	render(w, r, http.StatusOK, pages.HomePage(middleware.CurrentUser(r.Context())))

}
