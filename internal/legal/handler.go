// Package legal contiene las páginas legales: términos de uso y política de privacidad.
package legal

import (
	"net/http"

	"github.com/diegovhdev/yortale/internal/platform/httpx"
)

const (
	// operatorName es el responsable del servicio y del tratamiento de datos.
	operatorName = "Alcachofas'News"
	// contactEmail recibe consultas legales, denuncias de derechos de autor y solicitudes de privacidad.
	contactEmail = "diego.valencia.dev@gmail.com"
	// effectiveDate es la fecha en que entra en vigor la versión actual (auth.CurrentTermsVersion).
	effectiveDate = "14 de octubre de 2026"
)

// GET /terms
func Terms(w http.ResponseWriter, r *http.Request) {
	httpx.Render(w, r, http.StatusOK, TermsPage())
}

// GET /privacy
func Privacy(w http.ResponseWriter, r *http.Request) {
	httpx.Render(w, r, http.StatusOK, PrivacyPage())
}
