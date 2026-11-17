package stories

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/diegovhdev/yortale/internal/auth"
	"github.com/diegovhdev/yortale/internal/platform/httpx"
)

// Las rutas de idiomas están envueltas con admin en routes.go; el servicio vuelve a comprobar
// que quien las usa es administrador.

// GET /admin/languages
func (h *Handler) Languages(w http.ResponseWriter, r *http.Request) {
	h.renderLanguages(w, r, http.StatusOK, newLanguageForm())
}

// POST /admin/languages
func (h *Handler) CreateLanguage(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Formulario inválido", http.StatusBadRequest)
		return
	}
	form := newLanguageForm()
	languageFormValues(r, &form)
	err := h.stories.CreateLanguage(r.Context(), auth.CurrentUser(r.Context()), form.Input())
	if err != nil {
		h.renderLanguageFormError(w, r, err, form, "crear idioma")
		return
	}
	httpx.Redirect(w, r, languagesURL)
}

// GET /admin/languages/{code}/edit
func (h *Handler) EditLanguage(w http.ResponseWriter, r *http.Request) {
	l, err := h.stories.Language(r.Context(), auth.CurrentUser(r.Context()), r.PathValue("code"))
	if err != nil {
		fail(w, err, "buscar idioma")
		return
	}
	httpx.Render(w, r, http.StatusOK, EditLanguagePage(editLanguageForm(l)))
}

// POST /admin/languages/{code}/edit
func (h *Handler) UpdateLanguage(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Formulario inválido", http.StatusBadRequest)
		return
	}
	user := auth.CurrentUser(r.Context())
	l, err := h.stories.Language(r.Context(), user, r.PathValue("code"))
	if err != nil {
		fail(w, err, "buscar idioma")
		return
	}
	form := editLanguageForm(l)
	languageFormValues(r, &form)
	if err := h.stories.UpdateLanguage(r.Context(), user, l.Code, form.Input()); err != nil {
		h.renderLanguageFormError(w, r, err, form, "actualizar idioma")
		return
	}
	httpx.Redirect(w, r, languagesURL)
}

// languageFormValues lee el formulario. El código solo se lee al crear: al editar se queda
// el de la ruta.
func languageFormValues(r *http.Request, f *LanguageForm) {
	if f.IsNew {
		f.Code = r.PostFormValue("code")
	}
	f.Name = r.PostFormValue("name")
	f.Active = r.PostFormValue("active") != ""
}

// renderLanguageFormError vuelve a mostrar el formulario: 422 con los errores por campo, o 500
// con un mensaje general. Un 404 (el idioma no existe) se responde como tal.
func (h *Handler) renderLanguageFormError(w http.ResponseWriter, r *http.Request, err error, form LanguageForm, msg string) {
	status := http.StatusUnprocessableEntity
	var verrs auth.ValidationErrors
	switch {
	case errors.As(err, &verrs):
		form.Errors = verrs
	case errors.Is(err, ErrNotFound):
		notFound(w)
		return
	default:
		slog.Error(msg, "err", err, "code", form.Code)
		status = http.StatusInternalServerError
		form.General = genericError
	}
	switch {
	case httpx.IsHTMX(r):
		httpx.Render(w, r, status, LanguageFormFragment(form))
	case form.IsNew:
		h.renderLanguages(w, r, status, form)
	default:
		httpx.Render(w, r, status, EditLanguagePage(form))
	}
}

// renderLanguages dibuja la lista de idiomas con el formulario para agregar uno.
func (h *Handler) renderLanguages(w http.ResponseWriter, r *http.Request, status int, form LanguageForm) {
	langs, err := h.stories.Languages(r.Context())
	if err != nil {
		fail(w, err, "listar idiomas")
		return
	}
	httpx.Render(w, r, status, LanguagesPage(langs, form))
}
