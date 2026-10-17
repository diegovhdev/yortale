package stories

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/diegovhdev/yortale/internal/auth"
	"github.com/diegovhdev/yortale/internal/platform/httpx"
)

type Handler struct {
	stories *Service
}

func NewHandler(stories *Service) *Handler {
	return &Handler{stories: stories}
}

const genericError = "Algo salió mal. Inténtalo de nuevo en un momento."

// pathID lee un id numérico de la ruta, por ejemplo {id} en /stories/{id}/edit.
func pathID(r *http.Request, name string) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	return id, err == nil && id > 0
}

// fail responde a los errores que no son de validación: 404 si no existe (o no es visible),
// 500 en cualquier otro caso.
func fail(w http.ResponseWriter, err error, msg string) {
	if errors.Is(err, ErrNotFound) {
		http.Error(w, "Página no encontrada", http.StatusNotFound)
		return
	}
	slog.Error(msg, "err", err)
	http.Error(w, "Error interno", http.StatusInternalServerError)
}

func notFound(w http.ResponseWriter) {
	http.Error(w, "Página no encontrada", http.StatusNotFound)
}

// ---------- Explorar ----------

// GET /stories
func (h *Handler) Explore(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	view := ExploreView{
		Query:   strings.TrimSpace(q.Get("q")),
		GenreID: q.Get("genre"),
		Kind:    q.Get("kind"),
		Page:    1,
	}
	if runes := []rune(view.Query); len(runes) > 200 {
		view.Query = string(runes[:200])
	}
	if p, err := strconv.Atoi(q.Get("page")); err == nil && p > 1 {
		view.Page = p
	}

	if !validOption(Kinds, view.Kind) {
		view.Kind = ""
	}
	filter := ExploreFilter{Query: view.Query, Kind: view.Kind, Page: view.Page}
	if id, err := strconv.ParseInt(view.GenreID, 10, 16); err == nil {
		genreID := int16(id)
		filter.GenreID = &genreID
	} else {
		view.GenreID = ""
	}

	var err error
	if view.Genres, err = h.stories.Genres(r.Context()); err != nil {
		fail(w, err, "listar géneros")
		return
	}
	if view.Stories, view.HasNext, err = h.stories.Explore(r.Context(), filter); err != nil {
		fail(w, err, "explorar historias")
		return
	}
	httpx.Render(w, r, http.StatusOK, ExplorePage(view))
}

// GET /my/stories
func (h *Handler) MyStories(w http.ResponseWriter, r *http.Request) {
	user := auth.CurrentUser(r.Context())
	stories, err := h.stories.ByAuthor(r.Context(), user.ID)
	if err != nil {
		fail(w, err, "listar mis historias")
		return
	}
	httpx.Render(w, r, http.StatusOK, MyStoriesPage(stories))
}

// ---------- Ver ----------

// GET /stories/{id}  — sin slug: redirige a la URL canónica.
func (h *Handler) RedirectToStory(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		notFound(w)
		return
	}
	story, err := h.stories.Visible(r.Context(), auth.CurrentUser(r.Context()), id)
	if err != nil {
		fail(w, err, "buscar historia")
		return
	}
	http.Redirect(w, r, story.URL(), http.StatusMovedPermanently)
}

// GET /stories/{id}/{slug}
func (h *Handler) Show(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		notFound(w)
		return
	}
	view, err := h.stories.StoryPage(r.Context(), auth.CurrentUser(r.Context()), id)
	if err != nil {
		fail(w, err, "mostrar historia")
		return
	}
	// Se busca solo por id; si el slug no coincide (título cambiado, enlace mal copiado)
	// se redirige a la URL correcta.
	if r.PathValue("slug") != view.Story.Slug {
		http.Redirect(w, r, view.Story.URL(), http.StatusMovedPermanently)
		return
	}
	httpx.Render(w, r, http.StatusOK, StoryPage(view))
}

// ---------- Crear y editar ----------

func storyFormValues(r *http.Request, f *StoryForm) {
	f.Title = r.PostFormValue("title")
	f.Synopsis = r.PostFormValue("synopsis")
	f.Kind = r.PostFormValue("kind")
	f.GenreIDs = r.PostForm[genresField]
	f.Language = r.PostFormValue("language")
	f.ContentRating = r.PostFormValue("content_rating")
	f.Status = r.PostFormValue("status")
}

// GET /stories/new
func (h *Handler) New(w http.ResponseWriter, r *http.Request) {
	genres, err := h.stories.Genres(r.Context())
	if err != nil {
		fail(w, err, "listar géneros")
		return
	}
	httpx.Render(w, r, http.StatusOK, NewStoryPage(newStoryForm(genres)))
}

// POST /stories
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Formulario inválido", http.StatusBadRequest)
		return
	}
	user := auth.CurrentUser(r.Context())
	genres, err := h.stories.Genres(r.Context())
	if err != nil {
		fail(w, err, "listar géneros")
		return
	}
	form := newStoryForm(genres)
	storyFormValues(r, &form)

	story, err := h.stories.Create(r.Context(), user.ID, form.Input())
	if err != nil {
		h.renderStoryFormError(w, r, err, form, "crear historia")
		return
	}
	httpx.Redirect(w, r, story.URL())
}

// GET /stories/{id}/edit
func (h *Handler) Edit(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		notFound(w)
		return
	}
	user := auth.CurrentUser(r.Context())
	story, err := h.stories.ForAuthor(r.Context(), user.ID, id)
	if err != nil {
		fail(w, err, "buscar historia")
		return
	}
	genres, err := h.stories.Genres(r.Context())
	if err != nil {
		fail(w, err, "listar géneros")
		return
	}
	httpx.Render(w, r, http.StatusOK, EditStoryPage(editStoryForm(story, genres)))
}

// POST /stories/{id}/edit
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		notFound(w)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Formulario inválido", http.StatusBadRequest)
		return
	}
	user := auth.CurrentUser(r.Context())
	story, err := h.stories.ForAuthor(r.Context(), user.ID, id)
	if err != nil {
		fail(w, err, "buscar historia")
		return
	}
	genres, err := h.stories.Genres(r.Context())
	if err != nil {
		fail(w, err, "listar géneros")
		return
	}
	form := editStoryForm(story, genres)
	storyFormValues(r, &form)

	story, err = h.stories.Update(r.Context(), user.ID, id, form.Input())
	if err != nil {
		h.renderStoryFormError(w, r, err, form, "actualizar historia")
		return
	}
	httpx.Redirect(w, r, story.URL())
}

// renderStoryFormError vuelve a mostrar el formulario: 422 con los errores por campo,
// o 500 con un mensaje general. Un 404 (historia ajena o borrada) se responde como tal.
func (h *Handler) renderStoryFormError(w http.ResponseWriter, r *http.Request, err error, form StoryForm, msg string) {
	status := http.StatusUnprocessableEntity
	var verrs auth.ValidationErrors
	switch {
	case errors.As(err, &verrs):
		form.Errors = verrs
	case errors.Is(err, ErrNotFound):
		notFound(w)
		return
	default:
		slog.Error(msg, "err", err)
		form.General = genericError
		status = http.StatusInternalServerError
	}

	switch {
	case httpx.IsHTMX(r):
		httpx.Render(w, r, status, StoryFormFragment(form))
	case form.IsNew:
		httpx.Render(w, r, status, NewStoryPage(form))
	default:
		httpx.Render(w, r, status, EditStoryPage(form))
	}
}

// ---------- Borrar ----------

// GET /stories/{id}/delete
func (h *Handler) ConfirmDelete(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		notFound(w)
		return
	}
	story, err := h.stories.ForAuthor(r.Context(), auth.CurrentUser(r.Context()).ID, id)
	if err != nil {
		fail(w, err, "buscar historia")
		return
	}
	httpx.Render(w, r, http.StatusOK, ConfirmDeletePage(confirmDeleteStory(story)))
}

// POST /stories/{id}/delete
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		notFound(w)
		return
	}
	if err := h.stories.Delete(r.Context(), auth.CurrentUser(r.Context()).ID, id); err != nil {
		fail(w, err, "borrar historia")
		return
	}
	httpx.Redirect(w, r, "/my/stories")
}
