package stories

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/diegovhdev/yortale/internal/auth"
	"github.com/diegovhdev/yortale/internal/platform/httpx"
)

// GET /stories/{id}/{slug}/chapters/{chapterID}
func (h *Handler) ReadChapter(w http.ResponseWriter, r *http.Request) {
	storyID, ok1 := pathID(r, "id")
	chapterID, ok2 := pathID(r, "chapterID")
	if !ok1 || !ok2 {
		notFound(w)
		return
	}
	view, err := h.stories.ReadChapter(r.Context(), auth.CurrentUser(r.Context()), storyID, chapterID)
	if err != nil {
		fail(w, err, "leer capítulo")
		return
	}
	if r.PathValue("slug") != view.Story.Slug {
		http.Redirect(w, r, view.Chapter.URL(view.Story), http.StatusMovedPermanently)
		return
	}
	httpx.Render(w, r, http.StatusOK, ChapterPage(view))
}

func chapterFormValues(r *http.Request, f *ChapterForm) {
	f.Title = r.PostFormValue("title")
	f.Content = r.PostFormValue("content")
	f.Publish = r.PostFormValue("publish") == "on"
}

// GET /stories/{id}/chapters/new
func (h *Handler) NewChapter(w http.ResponseWriter, r *http.Request) {
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
	httpx.Render(w, r, http.StatusOK, ChapterFormPage(newChapterForm(story)))
}

// POST /stories/{id}/chapters
func (h *Handler) CreateChapter(w http.ResponseWriter, r *http.Request) {
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
	story, c, err := h.stories.CreateChapter(r.Context(), user.ID, id, ChapterInput{
		Title:   r.PostFormValue("title"),
		Content: r.PostFormValue("content"),
		Publish: r.PostFormValue("publish") == "on",
	})
	if err != nil {
		form := newChapterForm(story)
		chapterFormValues(r, &form)
		h.renderChapterFormError(w, r, err, form, "crear capítulo")
		return
	}
	httpx.Redirect(w, r, c.URL(story))
}

// GET /chapters/{id}/edit
func (h *Handler) EditChapter(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		notFound(w)
		return
	}
	story, c, err := h.stories.ChapterForAuthor(r.Context(), auth.CurrentUser(r.Context()).ID, id)
	if err != nil {
		fail(w, err, "buscar capítulo")
		return
	}
	httpx.Render(w, r, http.StatusOK, ChapterFormPage(editChapterForm(story, c)))
}

// POST /chapters/{id}/edit
func (h *Handler) UpdateChapter(w http.ResponseWriter, r *http.Request) {
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
	form := ChapterForm{}
	chapterFormValues(r, &form)

	story, c, err := h.stories.UpdateChapter(r.Context(), user.ID, id, form.Input())
	if err != nil {
		// Se conserva lo que el autor escribió, no lo que hay guardado.
		edit := editChapterForm(story, c)
		edit.Title, edit.Content, edit.Publish = form.Title, form.Content, form.Publish
		edit.Action = Chapter{ID: id}.EditURL()
		h.renderChapterFormError(w, r, err, edit, "actualizar capítulo")
		return
	}
	httpx.Redirect(w, r, c.URL(story))
}

func (h *Handler) renderChapterFormError(w http.ResponseWriter, r *http.Request, err error, form ChapterForm, msg string) {
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
	if httpx.IsHTMX(r) {
		httpx.Render(w, r, status, ChapterFormFragment(form))
		return
	}
	httpx.Render(w, r, status, ChapterFormPage(form))
}

// GET /chapters/{id}/delete
func (h *Handler) ConfirmDeleteChapter(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		notFound(w)
		return
	}
	story, c, err := h.stories.ChapterForAuthor(r.Context(), auth.CurrentUser(r.Context()).ID, id)
	if err != nil {
		fail(w, err, "buscar capítulo")
		return
	}
	httpx.Render(w, r, http.StatusOK, ConfirmDeletePage(confirmDeleteChapter(story, c)))
}

// POST /chapters/{id}/delete
func (h *Handler) DeleteChapter(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		notFound(w)
		return
	}
	story, err := h.stories.DeleteChapter(r.Context(), auth.CurrentUser(r.Context()).ID, id)
	if err != nil {
		fail(w, err, "borrar capítulo")
		return
	}
	httpx.Redirect(w, r, story.URL())
}

// POST /chapters/{id}/move  (direction=up|down)
func (h *Handler) MoveChapter(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		notFound(w)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Formulario inválido", http.StatusBadRequest)
		return
	}
	direction := r.PostFormValue("direction")
	if direction != "up" && direction != "down" {
		http.Error(w, "Dirección inválida", http.StatusBadRequest)
		return
	}
	story, err := h.stories.MoveChapter(r.Context(), auth.CurrentUser(r.Context()).ID, id, direction == "up")
	if err != nil {
		fail(w, err, "mover capítulo")
		return
	}
	httpx.Redirect(w, r, story.URL())
}
