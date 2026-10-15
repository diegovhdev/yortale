package auth

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/diegovhdev/yortale/internal/platform/httpx"
)

type Handler struct {
	auth          *Service
	secureCookies bool
}

func NewHandler(auth *Service, secureCookies bool) *Handler {
	return &Handler{auth: auth, secureCookies: secureCookies}
}

const genericError = "Algo salió mal. Inténtalo de nuevo en un momento."

// ---------- Registro ----------

// GET /register
func (h *Handler) ShowRegister(w http.ResponseWriter, r *http.Request) {
	if CurrentUser(r.Context()) != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	httpx.Render(w, r, http.StatusOK, RegisterPage(RegisterForm{}))
}

// POST /register
func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Formulario inválido", http.StatusBadRequest)
		return
	}
	// Los valores escritos se devuelven al formulario si hay errores (excepto contraseñas).
	form := RegisterForm{
		Username:    r.PostFormValue("username"),
		Email:       r.PostFormValue("email"),
		AcceptTerms: r.PostFormValue("accept_terms") == "on",
	}

	user, err := h.auth.Register(r.Context(), RegisterInput{
		Username:        form.Username,
		Email:           form.Email,
		Password:        r.PostFormValue("password"),
		PasswordConfirm: r.PostFormValue("password_confirm"),
		AcceptTerms:     form.AcceptTerms,
	})

	var verrs ValidationErrors
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
		httpx.Redirect(w, r, "/login")
		return
	}
	httpx.Redirect(w, r, "/")
}

// Con htmx se devuelve solo el formulario; sin JavaScript, la página completa.
func (h *Handler) renderRegister(w http.ResponseWriter, r *http.Request, status int, form RegisterForm) {
	if httpx.IsHTMX(r) {
		httpx.Render(w, r, status, RegisterFormFragment(form))
		return
	}
	httpx.Render(w, r, status, RegisterPage(form))
}

// ---------- Inicio de sesión ----------

// GET /login
func (h *Handler) ShowLogin(w http.ResponseWriter, r *http.Request) {
	if CurrentUser(r.Context()) != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	httpx.Render(w, r, http.StatusOK, LoginPage(LoginForm{}))
}

// POST /login
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Formulario inválido", http.StatusBadRequest)
		return
	}
	form := LoginForm{Login: r.PostFormValue("login")}

	user, err := h.auth.Authenticate(r.Context(), form.Login, r.PostFormValue("password"))
	switch {
	case errors.Is(err, ErrInvalidCredentials):
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
	httpx.Redirect(w, r, "/")
}

func (h *Handler) renderLogin(w http.ResponseWriter, r *http.Request, status int, form LoginForm) {
	if httpx.IsHTMX(r) {
		httpx.Render(w, r, status, LoginFormFragment(form))
		return
	}
	httpx.Render(w, r, status, LoginPage(form))
}

// ---------- Cerrar sesión ----------

// POST /logout
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(SessionCookieName); err == nil && cookie.Value != "" {
		if err := h.auth.EndSession(r.Context(), cookie.Value); err != nil {
			slog.Error("cerrar sesión", "err", err)
		}
	}
	ClearSessionCookie(w, h.secureCookies)
	httpx.Redirect(w, r, "/login")
}

func (h *Handler) startSession(w http.ResponseWriter, r *http.Request, userID int64) error {
	token, expires, err := h.auth.StartSession(r.Context(), userID)
	if err != nil {
		return err
	}
	SetSessionCookie(w, token, expires, h.secureCookies)
	return nil
}
