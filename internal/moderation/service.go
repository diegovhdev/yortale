package moderation

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/diegovhdev/yortale/internal/auth"
	"github.com/diegovhdev/yortale/internal/stories"
	"github.com/diegovhdev/yortale/web/components"
)

const (
	// maxCases y maxStoryReports limitan las listas del panel; con más, hace falta paginar.
	maxCases        = 100
	maxStoryReports = 100
	// maxHistory es cuántas acciones del registro se ven en la página de una historia o cuenta.
	maxHistory = 50
)

type store interface {
	PublishedChapterTitle(ctx context.Context, storyID, chapterID int64) (string, error)
	CreateReport(ctx context.Context, p ReportParams) error
	OpenCases(ctx context.Context, limit int) ([]Case, error)
	StoryReports(ctx context.Context, storyID int64, limit int) ([]Report, error)
	HideStory(ctx context.Context, moderatorID, storyID int64, reason string) error
	UnhideStory(ctx context.Context, moderatorID, storyID int64) error
	DismissReports(ctx context.Context, moderatorID, storyID int64) error
	Suspend(ctx context.Context, moderatorID, userID int64, until *time.Time, reason, details string) error
	Unsuspend(ctx context.Context, moderatorID, userID int64) error
	SetRole(ctx context.Context, adminID, userID int64, role, details string) error
	Actions(ctx context.Context, f ActionFilter, limit, offset int) ([]Action, error)
}

// storyFinder busca historias. Lo cumple *stories.Service.
type storyFinder interface {
	Visible(ctx context.Context, viewer *auth.User, id int64) (stories.Story, error)
	Story(ctx context.Context, id int64) (stories.Story, error)
}

// userFinder busca cuentas. Lo cumple *auth.Service.
type userFinder interface {
	UserByUsername(ctx context.Context, username string) (auth.User, error)
}

type Service struct {
	store   store
	stories storyFinder
	users   userFinder
}

func NewService(store store, stories storyFinder, users userFinder) *Service {
	return &Service{store: store, stories: stories, users: users}
}

// ---------- Permisos ----------

// canSuspend: nadie se suspende a sí mismo ni suspende a un administrador; un moderador solo
// puede suspender a usuarios normales, y un administrador también a moderadores.
func canSuspend(actor, target auth.User) bool {
	switch {
	case actor.ID == target.ID || target.IsAdmin():
		return false
	case actor.IsAdmin():
		return true
	default:
		return actor.IsStaff() && target.Role == auth.RoleUser
	}
}

// canChangeRole: solo los administradores cambian roles, y nunca el suyo propio, para que
// siempre quede al menos un administrador.
func canChangeRole(actor, target auth.User) bool {
	return actor.IsAdmin() && actor.ID != target.ID
}

// durationsFor son las duraciones de suspensión que puede elegir actor: la indefinida es solo
// para administradores.
func durationsFor(actor auth.User) []components.Option {
	var opts []components.Option
	for _, d := range suspensionDurations {
		if d.Days > 0 || actor.IsAdmin() {
			opts = append(opts, d.Option)
		}
	}
	return opts
}

// ---------- Reportar ----------

// ReportTarget devuelve lo que reporter quiere reportar: una historia pública y, si chapterID
// no es 0, uno de sus capítulos publicados. Si es suya devuelve también ErrOwnContent.
func (s *Service) ReportTarget(ctx context.Context, reporter *auth.User, storyID, chapterID int64) (ReportTarget, error) {
	story, err := s.stories.Visible(ctx, reporter, storyID)
	if errors.Is(err, stories.ErrNotFound) {
		return ReportTarget{}, ErrNotFound
	}
	if err != nil {
		return ReportTarget{}, err
	}
	// Visible deja al autor ver sus borradores y al equipo las ocultas, pero eso no se reporta.
	if story.IsDraft() || story.IsHidden() {
		return ReportTarget{}, ErrNotFound
	}
	t := ReportTarget{Story: story}
	if chapterID != 0 {
		if t.ChapterTitle, err = s.store.PublishedChapterTitle(ctx, storyID, chapterID); err != nil {
			return ReportTarget{}, err
		}
		t.ChapterID = chapterID
	}
	if story.AuthorID == reporter.ID {
		return t, ErrOwnContent
	}
	return t, nil
}

type ReportInput struct {
	StoryID   int64
	ChapterID int64 // 0 si se reporta la historia entera
	Reason    string
	Details   string
}

// Report valida y guarda el reporte. Devuelve auth.ValidationErrors si hay errores, y lo que se
// reportó (si existe) para volver a mostrar el formulario.
func (s *Service) Report(ctx context.Context, reporter *auth.User, in ReportInput) (ReportTarget, error) {
	t, err := s.ReportTarget(ctx, reporter, in.StoryID, in.ChapterID)
	if err != nil {
		return t, err
	}
	details := strings.TrimSpace(strings.ReplaceAll(in.Details, "\r\n", "\n"))
	errs := auth.ValidationErrors{}
	if !validReason(in.Reason) {
		errs["reason"] = "Elige un motivo de la lista."
	}
	switch n := utf8.RuneCountInString(details); {
	case n > maxReportDetailsRunes:
		errs["details"] = fmt.Sprintf("Los detalles no pueden tener más de %d caracteres.", maxReportDetailsRunes)
	case n == 0 && in.Reason == ReasonOther:
		errs["details"] = "Cuéntanos cuál es el problema."
	}
	if len(errs) > 0 {
		return t, errs
	}
	return t, s.store.CreateReport(ctx, ReportParams{
		ReporterID: reporter.ID, StoryID: in.StoryID, ChapterID: in.ChapterID, Reason: in.Reason, Details: details,
	})
}

// ---------- Historias ----------

func (s *Service) Cases(ctx context.Context) ([]Case, error) {
	return s.store.OpenCases(ctx, maxCases)
}

// StoryCase reúne lo que muestra la página de moderación de una historia.
func (s *Service) StoryCase(ctx context.Context, storyID int64) (StoryCaseView, error) {
	story, err := s.story(ctx, storyID)
	if err != nil {
		return StoryCaseView{}, err
	}
	v := StoryCaseView{Story: story, Hide: HideForm{Action: hideURL(story.ID)}}
	if v.Reports, err = s.store.StoryReports(ctx, story.ID, maxStoryReports); err != nil {
		return StoryCaseView{}, err
	}
	if v.Actions, err = s.store.Actions(ctx, ActionFilter{StoryID: story.ID}, maxHistory, 0); err != nil {
		return StoryCaseView{}, err
	}
	return v, nil
}

func (s *Service) story(ctx context.Context, id int64) (stories.Story, error) {
	story, err := s.stories.Story(ctx, id)
	if errors.Is(err, stories.ErrNotFound) {
		return stories.Story{}, ErrNotFound
	}
	return story, err
}

// HideStory oculta la historia y da por resueltos sus reportes abiertos. El motivo es
// obligatorio: lo ve el autor. Devuelve auth.ValidationErrors si falta.
func (s *Service) HideStory(ctx context.Context, actor *auth.User, storyID int64, reason string) error {
	reason, msg := cleanModReason(reason, "Explica al autor por qué se oculta su historia.")
	if msg != "" {
		return auth.ValidationErrors{"reason": msg}
	}
	if _, err := s.story(ctx, storyID); err != nil {
		return err
	}
	return s.store.HideStory(ctx, actor.ID, storyID, reason)
}

func (s *Service) UnhideStory(ctx context.Context, actor *auth.User, storyID int64) error {
	if _, err := s.story(ctx, storyID); err != nil {
		return err
	}
	return s.store.UnhideStory(ctx, actor.ID, storyID)
}

// DismissReports descarta los reportes abiertos de la historia: no había nada que corregir.
func (s *Service) DismissReports(ctx context.Context, actor *auth.User, storyID int64) error {
	if _, err := s.story(ctx, storyID); err != nil {
		return err
	}
	return s.store.DismissReports(ctx, actor.ID, storyID)
}

// cleanModReason limpia el motivo que escribe un moderador. Si está vacío o es demasiado
// largo, devuelve también el mensaje de error (emptyMsg si está vacío).
func cleanModReason(reason, emptyMsg string) (string, string) {
	reason = strings.TrimSpace(strings.ReplaceAll(reason, "\r\n", "\n"))
	switch n := utf8.RuneCountInString(reason); {
	case n == 0:
		return "", emptyMsg
	case n > maxModReasonRunes:
		return reason, fmt.Sprintf("El motivo no puede tener más de %d caracteres.", maxModReasonRunes)
	}
	return reason, ""
}

// ---------- Cuentas ----------

// UserPage reúne lo que muestra la página de moderación de una cuenta, con lo que actor puede hacer.
func (s *Service) UserPage(ctx context.Context, actor *auth.User, username string) (UserView, error) {
	target, err := s.user(ctx, username)
	if err != nil {
		return UserView{}, err
	}
	v := UserView{
		User:          target,
		CanSuspend:    canSuspend(*actor, target),
		CanChangeRole: canChangeRole(*actor, target),
		Suspend:       newSuspendForm(*actor, target),
		Role:          RoleForm{Action: roleURL(target.Username), Role: target.Role, Options: roleOptions},
	}
	if v.Actions, err = s.store.Actions(ctx, ActionFilter{UserID: target.ID}, maxHistory, 0); err != nil {
		return UserView{}, err
	}
	return v, nil
}

func (s *Service) user(ctx context.Context, username string) (auth.User, error) {
	u, err := s.users.UserByUsername(ctx, username)
	if errors.Is(err, auth.ErrNotFound) {
		return auth.User{}, ErrNotFound
	}
	return u, err
}

type SuspendInput struct {
	Duration string // un valor de suspensionDurations
	Reason   string
}

// Suspend suspende la cuenta y cierra sus sesiones. Devuelve ErrForbidden si actor no puede
// suspenderla, y auth.ValidationErrors si la duración o el motivo no sirven.
func (s *Service) Suspend(ctx context.Context, actor *auth.User, username string, in SuspendInput) error {
	target, err := s.user(ctx, username)
	if err != nil {
		return err
	}
	if !canSuspend(*actor, target) {
		return ErrForbidden
	}
	errs := auth.ValidationErrors{}
	d, ok := durationFor(*actor, in.Duration)
	if !ok {
		errs["duration"] = "Elige una duración de la lista."
	}
	reason, msg := cleanModReason(in.Reason, "Explica el motivo: lo verá la persona al intentar entrar.")
	if msg != "" {
		errs["reason"] = msg
	}
	if len(errs) > 0 {
		return errs
	}

	var until *time.Time
	details := d.Label + "."
	if d.Days > 0 {
		t := time.Now().AddDate(0, 0, d.Days)
		until = &t
		details = fmt.Sprintf("%s, hasta el %s.", d.Label, formatDateTime(t))
	}
	return s.store.Suspend(ctx, actor.ID, target.ID, until, reason, details)
}

// durationFor busca la duración elegida, si actor puede usarla.
func durationFor(actor auth.User, value string) (suspensionDuration, bool) {
	for _, d := range suspensionDurations {
		if d.Value == value && (d.Days > 0 || actor.IsAdmin()) {
			return d, true
		}
	}
	return suspensionDuration{}, false
}

// Unsuspend levanta la suspensión. Hace falta el mismo permiso que para suspender.
func (s *Service) Unsuspend(ctx context.Context, actor *auth.User, username string) error {
	target, err := s.user(ctx, username)
	if err != nil {
		return err
	}
	if !canSuspend(*actor, target) {
		return ErrForbidden
	}
	return s.store.Unsuspend(ctx, actor.ID, target.ID)
}

// SetRole cambia el rol de la cuenta. Devuelve ErrForbidden si actor no es administrador o
// intenta cambiar el suyo.
func (s *Service) SetRole(ctx context.Context, actor *auth.User, username, role string) error {
	target, err := s.user(ctx, username)
	if err != nil {
		return err
	}
	if !canChangeRole(*actor, target) {
		return ErrForbidden
	}
	if !validRole(role) {
		return auth.ValidationErrors{"role": "Elige un rol de la lista."}
	}
	details := fmt.Sprintf("De %s a %s.", auth.RoleLabel(target.Role), auth.RoleLabel(role))
	return s.store.SetRole(ctx, actor.ID, target.ID, role, details)
}

func validRole(role string) bool {
	for _, o := range roleOptions {
		if o.Value == role {
			return true
		}
	}
	return false
}

// ---------- Registro ----------

// Log devuelve una página del registro de moderación (page empieza en 1).
func (s *Service) Log(ctx context.Context, page int) (actions []Action, hasNext bool, err error) {
	page = max(page, 1)
	// Se pide una de más para saber si hay página siguiente.
	actions, err = s.store.Actions(ctx, ActionFilter{}, logPageSize+1, (page-1)*logPageSize)
	if err != nil {
		return nil, false, err
	}
	if len(actions) > logPageSize {
		return actions[:logPageSize], true, nil
	}
	return actions, false, nil
}
