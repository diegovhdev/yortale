// Package home contiene la página de inicio.
package home

import (
	"net/http"

	"github.com/diegovhdev/yortale/internal/auth"
	"github.com/diegovhdev/yortale/internal/platform/httpx"
)

// GET /  — página temporal para comprobar que la sesión funciona.
func Home(w http.ResponseWriter, r *http.Request) {
	httpx.Render(w, r, http.StatusOK, HomePage(auth.CurrentUser(r.Context())))
}
