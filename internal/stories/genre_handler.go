package stories

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/diegovhdev/yortale/internal/auth"
	"github.com/diegovhdev/yortale/internal/platform/httpx"
)

// Las rutas de géneros están envueltas con admin en routes.go; el servicio vuelve a comprobar
// que quien las usa es administrador.

// GET /admin/genres
func (h *Handler) AdminGenres(w http.ResponseWriter, r *http.Request) {
	h.renderGenres(w, r, http.StatusOK, newGenreForm())
}

// POST /admin/genres
func (h *Handler) CreateGenre(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Formulario inválido", http.StatusBadRequest)
		return
	}
	form := newGenreForm()
	genreFormValues(r, &form)
	if err := h.stories.CreateGenre(r.Context(), auth.CurrentUser(r.Context()), form.Input()); err != nil {
		h.renderGenreFormError(w, r, err, form, "crear género")
		return
	}
	httpx.Redirect(w, r, genresURL)
}

// GET /admin/genres/{id}/edit
func (h *Handler) EditGenre(w http.ResponseWriter, r *http.Request) {
	g, ok := h.pathGenre(w, r)
	if !ok {
		return
	}
	httpx.Render(w, r, http.StatusOK, EditGenrePage(editGenreForm(g)))
}

// POST /admin/genres/{id}/edit
func (h *Handler) UpdateGenre(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Formulario inválido", http.StatusBadRequest)
		return
	}
	g, ok := h.pathGenre(w, r)
	if !ok {
		return
	}
	form := editGenreForm(g)
	genreFormValues(r, &form)
	if err := h.stories.UpdateGenre(r.Context(), auth.CurrentUser(r.Context()), g.ID, form.Input()); err != nil {
		h.renderGenreFormError(w, r, err, form, "actualizar género")
		return
	}
	httpx.Redirect(w, r, genresURL)
}

// pathGenre busca el género de {id}; si no existe, ya respondió con 404.
func (h *Handler) pathGenre(w http.ResponseWriter, r *http.Request) (Genre, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 16)
	if err != nil {
		notFound(w)
		return Genre{}, false
	}
	g, err := h.stories.Genre(r.Context(), auth.CurrentUser(r.Context()), int16(id))
	if err != nil {
		fail(w, err, "buscar género")
		return Genre{}, false
	}
	return g, true
}

func genreFormValues(r *http.Request, f *GenreForm) {
	f.Name = r.PostFormValue("name")
	f.Active = r.PostFormValue("active") != ""
}

// renderGenreFormError vuelve a mostrar el formulario: 422 con los errores por campo, o 500
// con un mensaje general. Un 404 (el género no existe) se responde como tal.
func (h *Handler) renderGenreFormError(w http.ResponseWriter, r *http.Request, err error, form GenreForm, msg string) {
	status := http.StatusUnprocessableEntity
	var verrs auth.ValidationErrors
	switch {
	case errors.As(err, &verrs):
		form.Errors = verrs
	case errors.Is(err, ErrNotFound):
		notFound(w)
		return
	default:
		slog.Error(msg, "err", err, "name", form.Name)
		status = http.StatusInternalServerError
		form.General = genericError
	}
	switch {
	case httpx.IsHTMX(r):
		httpx.Render(w, r, status, GenreFormFragment(form))
	case form.IsNew:
		h.renderGenres(w, r, status, form)
	default:
		httpx.Render(w, r, status, EditGenrePage(form))
	}
}

// renderGenres dibuja la lista de géneros con el formulario para agregar uno.
func (h *Handler) renderGenres(w http.ResponseWriter, r *http.Request, status int, form GenreForm) {
	genres, err := h.stories.Genres(r.Context())
	if err != nil {
		fail(w, err, "listar géneros")
		return
	}
	httpx.Render(w, r, status, GenresPage(genres, form))
}
