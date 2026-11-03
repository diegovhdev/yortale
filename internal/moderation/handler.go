package moderation

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/diegovhdev/yortale/internal/auth"
	"github.com/diegovhdev/yortale/internal/platform/httpx"
	"github.com/diegovhdev/yortale/internal/stories"
)

type Handler struct {
	mod *Service
}

func NewHandler(mod *Service) *Handler {
	return &Handler{mod: mod}
}

const genericError = "Algo salió mal. Inténtalo de nuevo en un momento."

// parseID lee un id positivo. Un texto vacío también es inválido.
func parseID(raw string) (int64, bool) {
	id, err := strconv.ParseInt(raw, 10, 64)
	return id, err == nil && id > 0
}

// parseOptionalID es como parseID, pero un texto vacío es válido y da 0 (sin capítulo).
func parseOptionalID(raw string) (int64, bool) {
	if raw == "" {
		return 0, true
	}
	return parseID(raw)
}

// fail responde a los errores que no son de validación: 404, 403 o 500.
func fail(w http.ResponseWriter, err error, msg string) {
	switch {
	case errors.Is(err, ErrNotFound):
		http.Error(w, "Página no encontrada", http.StatusNotFound)
	case errors.Is(err, ErrForbidden):
		http.Error(w, "No tienes permiso para esta acción", http.StatusForbidden)
	default:
		slog.Error(msg, "err", err)
		http.Error(w, "Error interno", http.StatusInternalServerError)
	}
}

func notFound(w http.ResponseWriter) {
	http.Error(w, "Página no encontrada", http.StatusNotFound)
}

// ---------- Reportar ----------

// GET /reports/new?story={id}[&chapter={id}]
func (h *Handler) ShowReport(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	storyID, ok1 := parseID(q.Get("story"))
	chapterID, ok2 := parseOptionalID(q.Get("chapter"))
	if !ok1 || !ok2 {
		notFound(w)
		return
	}
	t, err := h.mod.ReportTarget(r.Context(), auth.CurrentUser(r.Context()), storyID, chapterID)
	form := ReportForm{Target: t, Own: errors.Is(err, ErrOwnContent)}
	if err != nil && !form.Own {
		fail(w, err, "buscar contenido para reportar")
		return
	}
	httpx.Render(w, r, http.StatusOK, ReportPage(form))
}

// POST /reports
func (h *Handler) CreateReport(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Formulario inválido", http.StatusBadRequest)
		return
	}
	storyID, ok1 := parseID(r.PostFormValue("story"))
	chapterID, ok2 := parseOptionalID(r.PostFormValue("chapter"))
	if !ok1 || !ok2 {
		notFound(w)
		return
	}
	form := ReportForm{Reason: r.PostFormValue("reason"), Details: r.PostFormValue("details")}

	t, err := h.mod.Report(r.Context(), auth.CurrentUser(r.Context()), ReportInput{
		StoryID: storyID, ChapterID: chapterID, Reason: form.Reason, Details: form.Details,
	})
	form.Target = t
	status := http.StatusUnprocessableEntity
	var verrs auth.ValidationErrors
	switch {
	case err == nil:
		httpx.Redirect(w, r, "/reports/sent?story="+strconv.FormatInt(storyID, 10))
		return
	case errors.As(err, &verrs):
		form.Errors = verrs
	case errors.Is(err, ErrOwnContent):
		form.Own = true
		status = http.StatusForbidden
	case errors.Is(err, ErrNotFound):
		notFound(w)
		return
	default:
		slog.Error("crear reporte", "err", err)
		form.General = genericError
		status = http.StatusInternalServerError
	}
	if httpx.IsHTMX(r) {
		httpx.Render(w, r, status, ReportFormFragment(form))
		return
	}
	httpx.Render(w, r, status, ReportPage(form))
}

// GET /reports/sent?story={id}
func (h *Handler) ReportSent(w http.ResponseWriter, r *http.Request) {
	storyID, _ := parseID(r.URL.Query().Get("story"))
	httpx.Render(w, r, http.StatusOK, ReportSentPage(storyID))
}

// ---------- Cola y registro ----------

// GET /moderation
func (h *Handler) Queue(w http.ResponseWriter, r *http.Request) {
	cases, err := h.mod.Cases(r.Context())
	if err != nil {
		fail(w, err, "listar casos de moderación")
		return
	}
	httpx.Render(w, r, http.StatusOK, QueuePage(cases))
}

// GET /moderation/log
func (h *Handler) Log(w http.ResponseWriter, r *http.Request) {
	v := LogView{Page: 1}
	if p, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil && p > 1 {
		v.Page = p
	}
	var err error
	if v.Actions, v.HasNext, err = h.mod.Log(r.Context(), v.Page); err != nil {
		fail(w, err, "listar registro de moderación")
		return
	}
	httpx.Render(w, r, http.StatusOK, LogPage(v))
}

// ---------- Historias ----------

// GET /moderation/stories/{id}
func (h *Handler) ShowStory(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r.PathValue("id"))
	if !ok {
		notFound(w)
		return
	}
	v, err := h.mod.StoryCase(r.Context(), id)
	if err != nil {
		fail(w, err, "mostrar caso de moderación")
		return
	}
	httpx.Render(w, r, http.StatusOK, StoryCasePage(v))
}

// POST /moderation/stories/{id}/hide
func (h *Handler) HideStory(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r.PathValue("id"))
	if !ok {
		notFound(w)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Formulario inválido", http.StatusBadRequest)
		return
	}
	form := HideForm{Action: hideURL(id), Reason: r.PostFormValue("reason")}
	err := h.mod.HideStory(r.Context(), auth.CurrentUser(r.Context()), id, form.Reason)

	status := http.StatusUnprocessableEntity
	var verrs auth.ValidationErrors
	switch {
	case err == nil || errors.Is(err, ErrNoChange): // ErrNoChange: otro moderador se adelantó
		httpx.Redirect(w, r, stories.Story{ID: id}.ModerationURL())
		return
	case errors.As(err, &verrs):
		form.Errors = verrs
	case errors.Is(err, ErrNotFound):
		notFound(w)
		return
	default:
		slog.Error("ocultar historia", "err", err, "story_id", id)
		form.General = genericError
		status = http.StatusInternalServerError
	}
	if httpx.IsHTMX(r) {
		httpx.Render(w, r, status, HideFormFragment(form))
		return
	}
	v, err := h.mod.StoryCase(r.Context(), id)
	if err != nil {
		fail(w, err, "mostrar caso de moderación")
		return
	}
	v.Hide = form
	httpx.Render(w, r, status, StoryCasePage(v))
}

// POST /moderation/stories/{id}/unhide
func (h *Handler) UnhideStory(w http.ResponseWriter, r *http.Request) {
	h.storyAction(w, r, "mostrar historia", h.mod.UnhideStory)
}

// POST /moderation/stories/{id}/dismiss
func (h *Handler) DismissReports(w http.ResponseWriter, r *http.Request) {
	h.storyAction(w, r, "descartar reportes", h.mod.DismissReports)
}

// storyAction resuelve las acciones sobre una historia que no llevan formulario: hacerla y
// volver a su página de moderación.
func (h *Handler) storyAction(w http.ResponseWriter, r *http.Request, msg string,
	action func(ctx context.Context, actor *auth.User, storyID int64) error) {
	id, ok := parseID(r.PathValue("id"))
	if !ok {
		notFound(w)
		return
	}
	err := action(r.Context(), auth.CurrentUser(r.Context()), id)
	if err != nil && !errors.Is(err, ErrNoChange) {
		fail(w, err, msg)
		return
	}
	httpx.Redirect(w, r, stories.Story{ID: id}.ModerationURL())
}

// ---------- Cuentas ----------

// GET /moderation/users/{username}
func (h *Handler) ShowUser(w http.ResponseWriter, r *http.Request) {
	v, err := h.mod.UserPage(r.Context(), auth.CurrentUser(r.Context()), r.PathValue("username"))
	if err != nil {
		fail(w, err, "mostrar cuenta en moderación")
		return
	}
	httpx.Render(w, r, http.StatusOK, UserPage(v))
}

// POST /moderation/users/{username}/suspend
func (h *Handler) Suspend(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Formulario inválido", http.StatusBadRequest)
		return
	}
	actor := auth.CurrentUser(r.Context())
	username := r.PathValue("username")
	form := SuspendForm{
		Action:    suspendURL(username),
		Durations: durationsFor(*actor),
		Duration:  r.PostFormValue("duration"),
		Reason:    r.PostFormValue("reason"),
	}
	err := h.mod.Suspend(r.Context(), actor, username, SuspendInput{Duration: form.Duration, Reason: form.Reason})

	status := http.StatusUnprocessableEntity
	var verrs auth.ValidationErrors
	switch {
	case err == nil:
		httpx.Redirect(w, r, userModerationURL(username))
		return
	case errors.As(err, &verrs):
		form.Errors = verrs
	case errors.Is(err, ErrNotFound), errors.Is(err, ErrForbidden):
		fail(w, err, "suspender cuenta")
		return
	default:
		slog.Error("suspender cuenta", "err", err, "username", username)
		form.General = genericError
		status = http.StatusInternalServerError
	}
	if httpx.IsHTMX(r) {
		httpx.Render(w, r, status, SuspendFormFragment(form))
		return
	}
	v, err := h.mod.UserPage(r.Context(), actor, username)
	if err != nil {
		fail(w, err, "mostrar cuenta en moderación")
		return
	}
	v.Suspend = form
	httpx.Render(w, r, status, UserPage(v))
}

// POST /moderation/users/{username}/unsuspend
func (h *Handler) Unsuspend(w http.ResponseWriter, r *http.Request) {
	username := r.PathValue("username")
	err := h.mod.Unsuspend(r.Context(), auth.CurrentUser(r.Context()), username)
	if err != nil && !errors.Is(err, ErrNoChange) {
		fail(w, err, "levantar suspensión")
		return
	}
	httpx.Redirect(w, r, userModerationURL(username))
}

// POST /moderation/users/{username}/role
func (h *Handler) SetRole(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Formulario inválido", http.StatusBadRequest)
		return
	}
	actor := auth.CurrentUser(r.Context())
	username := r.PathValue("username")
	err := h.mod.SetRole(r.Context(), actor, username, r.PostFormValue("role"))
	var verrs auth.ValidationErrors
	switch {
	case err == nil || errors.Is(err, ErrNoChange):
		httpx.Redirect(w, r, userModerationURL(username))
	case errors.As(err, &verrs):
		// Solo pasa si alguien manipula el <select>: basta con volver a mostrar la página.
		v, err := h.mod.UserPage(r.Context(), actor, username)
		if err != nil {
			fail(w, err, "mostrar cuenta en moderación")
			return
		}
		v.Role.Error = verrs["role"]
		httpx.Render(w, r, http.StatusUnprocessableEntity, UserPage(v))
	default:
		fail(w, err, "cambiar rol")
	}
}
