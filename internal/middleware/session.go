// Package middleware contiene funciones que envuelven a los handlers HTTP.
package middleware

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/diegovhdev/yortale/internal/models"
	"github.com/diegovhdev/yortale/internal/service"
)

const SessionCookieName = "yortale_session"

type ctxKey struct{}

// SetSessionCookie escribe la cookie de sesión.
//   - HttpOnly: JavaScript no puede leerla (protege contra robo por XSS).
//   - SameSite=Lax: el navegador no la envía en POST desde otros sitios (protege contra CSRF).
//   - Secure: solo viaja por HTTPS (se activa en producción).
func SetSessionCookie(w http.ResponseWriter, token string, expires time.Time, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

func ClearSessionCookie(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

// LoadUser lee la cookie de sesión en cada petición y, si es válida,
// guarda el usuario en el contexto para que los handlers lo usen.
func LoadUser(auth *service.AuthService, secure bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(SessionCookieName)
			if err != nil || cookie.Value == "" {
				next.ServeHTTP(w, r)
				return
			}

			user, err := auth.UserFromSession(r.Context(), cookie.Value)
			switch {
			case errors.Is(err, service.ErrNoSession):
				ClearSessionCookie(w, secure) // cookie vieja o falsa: la borramos
			case err != nil:
				slog.Error("cargar sesión", "err", err)
			default:
				r = r.WithContext(context.WithValue(r.Context(), ctxKey{}, &user))
			}
			next.ServeHTTP(w, r)
		})
	}
}

// CurrentUser devuelve el usuario con sesión iniciada, o nil si es un visitante.
func CurrentUser(ctx context.Context) *models.User {
	u, _ := ctx.Value(ctxKey{}).(*models.User)
	return u
}
