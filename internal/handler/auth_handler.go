package handler

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/diegovhdev/yortale/internal/middleware"
	"github.com/diegovhdev/yortale/internal/service"
	"github.com/diegovhdev/yortale/web/pages"
)

type AuthHandler struct {
	auth          *service.AuthService
	secureCookies bool
}

func NewAuthHandler(auth *service.AuthService, secureCookies bool) *AuthHandler {
	return &AuthHandler{auth: auth, secureCookies: secureCookies}
}

const genericError = "Algo salió mal. Inténtalo de nuevo en un momento."

// ---------- Registro ----------

// GET /registro
func (h *AuthHandler) ShowRegister(w http.ResponseWriter, r *http.Request) {
	if middleware.CurrentUser(r.Context()) != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	render(w, r, http.StatusOK, pages.RegisterPage(pages.RegisterForm{}))
}

// POST /registro
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Formulario inválido", http.StatusBadRequest)
		return
	}
	// Los valores escritos se devuelven al formulario si hay errores (excepto contraseñas).
	form := pages.RegisterForm{
		Username:    r.PostFormValue("username"),
		Email:       r.PostFormValue("email"),
		AcceptTerms: r.PostFormValue("accept_terms") == "on",
	}

	user, err := h.auth.Register(r.Context(), service.RegisterInput{
		Username:        form.Username,
		Email:           form.Email,
		Password:        r.PostFormValue("password"),
		PasswordConfirm: r.PostFormValue("password_confirm"),
		AcceptTerms:     form.AcceptTerms,
	})

	var verrs service.ValidationErrors
	switch {
	case errors.As(err, &verrs):
		form.Errors = verrs
		h.renderRegister(w, r, http.StatusUnprocessableEntity, form)
		return
	case err != nil:
		slog.Error("registrar usuario", "err", err)
		form.General = genericError
		h.renderRegister(w, r, http.StatusInternalServerError, form)
		return
	}

	// Cuenta creada: se inicia sesión de inmediato.
	if err := h.startSession(w, r, user.ID); err != nil {
		slog.Error("iniciar sesión tras registro", "err", err)
		redirect(w, r, "/iniciar-sesion")
		return
	}
	redirect(w, r, "/")
}

// Con htmx se devuelve solo el formulario; sin JavaScript, la página completa.
func (h *AuthHandler) renderRegister(w http.ResponseWriter, r *http.Request, status int, form pages.RegisterForm) {
	if isHTMX(r) {
		render(w, r, status, pages.RegisterFormFragment(form))
		return
	}
	render(w, r, status, pages.RegisterPage(form))
}

// ---------- Inicio de sesión ----------

// GET /iniciar-sesion
func (h *AuthHandler) ShowLogin(w http.ResponseWriter, r *http.Request) {
	if middleware.CurrentUser(r.Context()) != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	render(w, r, http.StatusOK, pages.LoginPage(pages.LoginForm{}))
}

// POST /iniciar-sesion
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Formulario inválido", http.StatusBadRequest)
		return
	}
	form := pages.LoginForm{Login: r.PostFormValue("login")}

	user, err := h.auth.Authenticate(r.Context(), form.Login, r.PostFormValue("password"))
	switch {
	case errors.Is(err, service.ErrInvalidCredentials):
		form.General = "Usuario, correo o contraseña incorrectos."
		h.renderLogin(w, r, http.StatusUnprocessableEntity, form)
		return
	case err != nil:
		slog.Error("autenticar", "err", err)
		form.General = genericError
		h.renderLogin(w, r, http.StatusInternalServerError, form)
		return
	}

	if err := h.startSession(w, r, user.ID); err != nil {
		slog.Error("crear sesión", "err", err)
		form.General = genericError
		h.renderLogin(w, r, http.StatusInternalServerError, form)
		return
	}
	redirect(w, r, "/")
}

func (h *AuthHandler) renderLogin(w http.ResponseWriter, r *http.Request, status int, form pages.LoginForm) {
	if isHTMX(r) {
		render(w, r, status, pages.LoginFormFragment(form))
		return
	}
	render(w, r, status, pages.LoginPage(form))
}

// ---------- Cerrar sesión ----------

// POST /cerrar-sesion
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(middleware.SessionCookieName); err == nil && cookie.Value != "" {
		if err := h.auth.EndSession(r.Context(), cookie.Value); err != nil {
			slog.Error("cerrar sesión", "err", err)
		}
	}
	middleware.ClearSessionCookie(w, h.secureCookies)
	redirect(w, r, "/iniciar-sesion")
}

func (h *AuthHandler) startSession(w http.ResponseWriter, r *http.Request, userID int64) error {
	token, expires, err := h.auth.StartSession(r.Context(), userID)
	if err != nil {
		return err
	}
	middleware.SetSessionCookie(w, token, expires, h.secureCookies)
	return nil
}
