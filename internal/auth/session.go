package auth

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/diegovhdev/yortale/internal/platform/httpx"
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
func LoadUser(auth *Service, secure bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(SessionCookieName)
			if err != nil || cookie.Value == "" {
				next.ServeHTTP(w, r)
				return
			}

			user, err := auth.UserFromSession(r.Context(), cookie.Value)
			switch {
			case errors.Is(err, ErrNoSession):
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
func CurrentUser(ctx context.Context) *User {
	u, _ := ctx.Value(ctxKey{}).(*User)
	return u
}

// RequireUser deja pasar solo a usuarios con sesión iniciada. A los visitantes los manda
// al login; si pedían una página (GET), el login los devuelve a ella con ?next=.
// Los handlers protegidos pueden dar por hecho que CurrentUser no es nil.
func RequireUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if CurrentUser(r.Context()) == nil {
			target := "/login"
			if r.Method == http.MethodGet {
				target += "?next=" + url.QueryEscape(r.URL.RequestURI())
			}
			httpx.Redirect(w, r, target)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// SafeNext devuelve next solo si es una ruta de este mismo sitio; si no, "/".
// Evita que /login?next=https://otro-sitio.com sirva para redirigir a cualquier parte.
func SafeNext(next string) string {
	if !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") || strings.HasPrefix(next, "/\\") {
		return "/"
	}
	if u, err := url.Parse(next); err != nil || u.Host != "" || u.Scheme != "" {
		return "/"
	}
	return next
}
