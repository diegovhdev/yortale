package stories

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"slices"
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
		Query: strings.TrimSpace(q.Get("q")),
		Kind:  q.Get("kind"),
		Page:  1,
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
	// Es un filtro y no un formulario que se guarda, así que los valores raros se ignoran en vez
	// de dar error: ids que no son números, repetidos y los que pasan de maxGenres.
	for _, raw := range q["genre"] {
		id, err := strconv.ParseInt(raw, 10, 16)
		if err != nil || slices.Contains(filter.GenreIDs, int16(id)) || len(filter.GenreIDs) == maxGenres {
			continue
		}
		filter.GenreIDs = append(filter.GenreIDs, int16(id))
		view.GenreIDs = append(view.GenreIDs, strconv.Itoa(int(id)))
	}
	for _, w := range excludableWarnings() {
		if slices.Contains(q["exclude"], w.Value) {
			filter.ExcludeWarnings = append(filter.ExcludeWarnings, w.Value)
		}
	}
	view.ExcludeWarnings = filter.ExcludeWarnings
	view.Tags = strings.TrimSpace(q.Get("tags"))
	view.ExcludeTags = strings.TrimSpace(q.Get("exclude_tags"))
	filter.Tags = tagFilterSlugs(view.Tags)
	filter.ExcludeTags = tagFilterSlugs(view.ExcludeTags)

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

// ---------- Tags ----------

// GET /tags/{slug}
func (h *Handler) ShowTag(w http.ResponseWriter, r *http.Request) {
	// Se acepta cualquier forma del tag (/tags/Época Victoriana) y se redirige a su slug.
	slug := r.PathValue("slug")
	if canonical := TagSlug(slug); canonical != slug {
		if canonical == "" {
			notFound(w)
			return
		}
		http.Redirect(w, r, Tag{Slug: canonical}.URL(), http.StatusMovedPermanently)
		return
	}
	page := 1
	if p, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil && p > 1 {
		page = p
	}
	view, err := h.stories.TagPage(r.Context(), slug, page)
	if err != nil {
		fail(w, err, "mostrar tag")
		return
	}
	httpx.Render(w, r, http.StatusOK, TagPage(view))
}

// GET /api/tags?q=…
// Sugerencias para el campo de tags del formulario de historia, en JSON: [{name, slug, uses}].
func (h *Handler) SuggestTags(w http.ResponseWriter, r *http.Request) {
	tags, err := h.stories.SuggestTags(r.Context(), r.URL.Query().Get("q"))
	if err != nil {
		fail(w, err, "sugerir tags")
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(tags); err != nil {
		slog.Error("escribir sugerencias de tags", "err", err)
	}
}

// GET /my/stories
func (h *Handler) MyStories(w http.ResponseWriter, r *http.Request) {
	user := auth.CurrentUser(r.Context())
	stories, err := h.stories.ByAuthor(r.Context(), user.ID)
	if err != nil {
		fail(w, err, "listar mis historias")
		return
	}
	httpx.Render(w, r, http.StatusOK, MyStoriesPage(user, stories))
}

// ---------- Perfil del autor ----------

// GET /users/{username}
func (h *Handler) AuthorProfile(w http.ResponseWriter, r *http.Request) {
	view, err := h.stories.AuthorPage(r.Context(), auth.CurrentUser(r.Context()), r.PathValue("username"))
	if err != nil {
		fail(w, err, "mostrar perfil del autor")
		return
	}
	// El username no distingue mayúsculas (citext); si se escribió distinto, se redirige
	// a la forma guardada para que cada perfil tenga una sola URL.
	if r.PathValue("username") != view.Author.Username {
		http.Redirect(w, r, view.Author.ProfileURL(), http.StatusMovedPermanently)
		return
	}
	httpx.Render(w, r, http.StatusOK, AuthorPage(view))
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
	f.Warnings = r.PostForm[warningsField]
	f.Tags = r.PostForm[tagsField]
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
