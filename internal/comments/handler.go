package comments

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/a-h/templ"
	"github.com/diegovhdev/yortale/internal/auth"
	"github.com/diegovhdev/yortale/internal/platform/httpx"
	"github.com/diegovhdev/yortale/internal/stories"
)

type Handler struct {
	comments *Service
}

func NewHandler(comments *Service) *Handler {
	return &Handler{comments: comments}
}

const genericError = "Algo salió mal. Inténtalo de nuevo en un momento."

func pathID(r *http.Request, name string) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	return id, err == nil && id > 0
}

// fail responde a los errores que no son de validación: 404, 403 o 500.
func fail(w http.ResponseWriter, err error, msg string) {
	switch {
	case errors.Is(err, ErrNotFound):
		http.Error(w, "Comentario no encontrado", http.StatusNotFound)
	case errors.Is(err, ErrForbidden):
		http.Error(w, "No tienes permiso para esta acción", http.StatusForbidden)
	default:
		slog.Error(msg, "err", err)
		http.Error(w, "Error interno", http.StatusInternalServerError)
	}
}

// redirectToComment lleva al comentario dentro de su capítulo. Si no se puede calcular su
// página, va a la primera: es mejor que dar un error después de guardar.
func (h *Handler) redirectToComment(w http.ResponseWriter, r *http.Request, t Target) {
	link, err := h.comments.Permalink(r.Context(), t)
	if err != nil {
		slog.Error("buscar página del comentario", "err", err, "comment_id", t.Comment.ID)
		link = t.PageURL(1, t.Comment.Anchor())
	}
	httpx.Redirect(w, r, link)
}

// ChapterSection cumple stories.CommentSection: dibuja los comentarios bajo un capítulo.
// La página de comentarios va en ?comments=N, para no chocar con otros parámetros.
func (h *Handler) ChapterSection(r *http.Request, v stories.ChapterView) templ.Component {
	page, _ := strconv.Atoi(r.URL.Query().Get("comments"))
	sec, err := h.comments.ChapterSection(r.Context(), auth.CurrentUser(r.Context()), v.Story, v.Chapter, page)
	if err != nil {
		slog.Error("cargar comentarios", "err", err, "chapter_id", v.Chapter.ID)
		return SectionUnavailable()
	}
	return ChapterComments(sec)
}

// renderFormError vuelve a mostrar el formulario con el error: el fragmento con htmx, o la
// página completa que da page sin JS.
func renderFormError(w http.ResponseWriter, r *http.Request, err error, form Form, page func(Form) templ.Component, msg string) {
	status := http.StatusUnprocessableEntity
	var verrs auth.ValidationErrors
	switch {
	case errors.As(err, &verrs):
		form.Errors = verrs
		form.General = verrs["parent"]
	case errors.Is(err, ErrNotFound):
		fail(w, err, msg)
		return
	case errors.Is(err, ErrForbidden):
		form.General = "No puedes comentar en este capítulo."
		if form.IsEdit() {
			form.General = "No puedes editar este comentario."
		}
		status = http.StatusForbidden
	default:
		slog.Error(msg, "err", err)
		form.General = genericError
		status = http.StatusInternalServerError
	}
	if httpx.IsHTMX(r) {
		httpx.Render(w, r, status, FormFragment(form))
		return
	}
	httpx.Render(w, r, status, page(form))
}

// POST /chapters/{id}/comments
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	chapterID, ok := pathID(r, "id")
	if !ok {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Formulario inválido", http.StatusBadRequest)
		return
	}
	in := Input{Body: r.PostFormValue("body"), ParentID: r.PostFormValue("parent_id")}
	form := newForm(chapterID)
	if parentID, err := strconv.ParseInt(in.ParentID, 10, 64); err == nil && parentID > 0 {
		form = replyForm(chapterID, parentID)
	}

	scope, c, err := h.comments.Create(r.Context(), auth.CurrentUser(r.Context()), chapterID, in)
	if err != nil {
		form.Body = in.Body
		renderFormError(w, r, err, form, func(f Form) templ.Component { return FormPage(f, scope) }, "crear comentario")
		return
	}
	if !httpx.IsHTMX(r) {
		h.redirectToComment(w, r, Target{Scope: scope, Comment: c})
		return
	}
	// Con htmx: el formulario vuelve vacío y el comentario se añade a la lista.
	total, err := h.comments.Count(r.Context(), chapterID)
	if err != nil {
		slog.Error("contar comentarios", "err", err, "chapter_id", chapterID)
		total = -1 // no se actualiza el contador
	}
	httpx.Render(w, r, http.StatusOK, Created(form, scope, c, total))
}

// GET /comments/{id}
// Sin htmx lleva al comentario en su capítulo; con htmx devuelve solo el comentario
// (lo usa «Cancelar» al editar en el sitio).
func (h *Handler) Show(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		http.NotFound(w, r)
		return
	}
	t, err := h.comments.Find(r.Context(), auth.CurrentUser(r.Context()), id)
	if err != nil {
		fail(w, err, "buscar comentario")
		return
	}
	if httpx.IsHTMX(r) {
		httpx.Render(w, r, http.StatusOK, Article(t.Scope, t.Comment))
		return
	}
	link, err := h.comments.Permalink(r.Context(), t)
	if err != nil {
		fail(w, err, "buscar página del comentario")
		return
	}
	http.Redirect(w, r, link, http.StatusFound)
}

// GET /comments/{id}/edit
func (h *Handler) Edit(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		http.NotFound(w, r)
		return
	}
	t, err := h.comments.Find(r.Context(), auth.CurrentUser(r.Context()), id)
	if err == nil && t.Comment.Deleted {
		err = ErrNotFound
	} else if err == nil && !t.CanEdit(t.Comment) {
		err = ErrForbidden
	}
	if err != nil {
		fail(w, err, "buscar comentario")
		return
	}
	if httpx.IsHTMX(r) {
		httpx.Render(w, r, http.StatusOK, FormFragment(editForm(t, true)))
		return
	}
	httpx.Render(w, r, http.StatusOK, EditPage(editForm(t, false), t))
}

// POST /comments/{id}/edit
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Formulario inválido", http.StatusBadRequest)
		return
	}
	body := r.PostFormValue("body")
	inline := r.PostFormValue("inline") == "1" && httpx.IsHTMX(r)

	t, err := h.comments.Update(r.Context(), auth.CurrentUser(r.Context()), id, body)
	if err != nil {
		form := editForm(t, inline)
		form.Body = body // lo que escribió, no lo guardado
		renderFormError(w, r, err, form, func(f Form) templ.Component { return EditPage(f, t) }, "editar comentario")
		return
	}
	if inline {
		httpx.Render(w, r, http.StatusOK, Article(t.Scope, t.Comment))
		return
	}
	h.redirectToComment(w, r, t)
}

// GET /comments/{id}/delete
// La página de confirmación es para quien no tiene JS; con htmx, el enlace «Borrar» pregunta
// con hx-confirm y envía el POST directamente.
func (h *Handler) ConfirmDelete(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		http.NotFound(w, r)
		return
	}
	t, err := h.comments.Find(r.Context(), auth.CurrentUser(r.Context()), id)
	if err == nil && t.Comment.Deleted {
		err = ErrNotFound
	} else if err == nil && !t.CanDelete(t.Comment) {
		err = ErrForbidden
	}
	if err != nil {
		fail(w, err, "buscar comentario")
		return
	}
	httpx.Render(w, r, http.StatusOK, DeletePage(t))
}

// POST /comments/{id}/delete
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		http.NotFound(w, r)
		return
	}
	t, err := h.comments.Delete(r.Context(), auth.CurrentUser(r.Context()), id)
	if err != nil {
		fail(w, err, "borrar comentario")
		return
	}
	if httpx.IsHTMX(r) {
		// Se sustituye por el aviso de borrado; al recargar desaparece si no tiene respuestas.
		httpx.Render(w, r, http.StatusOK, Article(t.Scope, t.Comment))
		return
	}
	h.redirectToComment(w, r, t)
}
