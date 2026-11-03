// Package moderation contiene los reportes de los lectores y las herramientas del equipo de
// moderación: la cola de reportes, ocultar historias, suspender cuentas, cambiar roles y el
// registro de todo lo que se hace (moderation_actions).
//
// Escribe directamente las columnas de moderación de otras tablas (stories.hidden_*,
// users.suspended_*, users.role) para hacerlo en la misma transacción que el registro.
// Leer esas columnas y respetarlas (no listar historias ocultas, no dejar entrar a cuentas
// suspendidas) es cosa de stories y de auth.
package moderation

import (
	"net/url"
	"time"

	"github.com/diegovhdev/yortale/internal/auth"
	"github.com/diegovhdev/yortale/internal/stories"
	"github.com/diegovhdev/yortale/web/components"
)

// ReportReason es un motivo de reporte, con la explicación que ve el lector al elegirlo.
type ReportReason struct {
	Value       string
	Label       string
	Description string
}

const ReasonOther = "other"

// ReportReasons debe coincidir con el CHECK de reports.reason.
var ReportReasons = []ReportReason{
	{Value: "spam", Label: "Spam o publicidad",
		Description: "No es una historia, sino anuncios, enlaces o texto repetido."},
	{Value: "plagiarism", Label: "Plagio",
		Description: "Copia la obra de otra persona sin su permiso."},
	{Value: "harassment", Label: "Acoso u odio",
		Description: "Ataca, amenaza o humilla a una persona real o a un colectivo."},
	{Value: "inappropriate", Label: "Contenido inapropiado o mal clasificado",
		Description: "Infringe los términos de uso, o le faltan las advertencias o la clasificación que le corresponden."},
	{Value: ReasonOther, Label: "Otro motivo",
		Description: "Explícalo en los detalles."},
}

func reasonLabel(value string) string {
	for _, r := range ReportReasons {
		if r.Value == value {
			return r.Label
		}
	}
	return value
}

func validReason(value string) bool {
	for _, r := range ReportReasons {
		if r.Value == value {
			return true
		}
	}
	return false
}

const (
	// maxReportDetailsRunes debe coincidir con el CHECK (char_length(details) <= 1000) de reports.
	maxReportDetailsRunes = 1000
	// maxModReasonRunes limita el motivo que escribe un moderador. Las columnas hidden_reason,
	// suspension_reason y moderation_actions.reason no tienen CHECK: este límite es la única validación.
	maxModReasonRunes = 500
	logPageSize       = 50
)

// Estados de un reporte, del CHECK de reports.status. 'reviewing' existe en el esquema pero
// todavía no se usa: un reporte pasa de abierto a resuelto o descartado.
const (
	StatusOpen      = "open"
	StatusResolved  = "resolved"
	StatusDismissed = "dismissed"
)

func statusLabel(status string) string {
	switch status {
	case StatusOpen:
		return "Abierto"
	case StatusResolved:
		return "Resuelto"
	case StatusDismissed:
		return "Descartado"
	default:
		return "En revisión"
	}
}

// Acciones del registro. Deben coincidir con el CHECK de moderation_actions.action.
const (
	ActionHideStory      = "hide_story"
	ActionUnhideStory    = "unhide_story"
	ActionDismissReports = "dismiss_reports"
	ActionSuspendUser    = "suspend_user"
	ActionUnsuspendUser  = "unsuspend_user"
	ActionChangeRole     = "change_role"
)

func actionLabel(action string) string {
	switch action {
	case ActionHideStory:
		return "Ocultó la historia"
	case ActionUnhideStory:
		return "Volvió a mostrar la historia"
	case ActionDismissReports:
		return "Descartó reportes"
	case ActionSuspendUser:
		return "Suspendió la cuenta"
	case ActionUnsuspendUser:
		return "Levantó la suspensión"
	case ActionChangeRole:
		return "Cambió el rol"
	default:
		return action
	}
}

// Duraciones de suspensión que se pueden elegir. La indefinida solo la pueden aplicar los
// administradores (ver durationsFor).
const durationPermanent = "permanent"

type suspensionDuration struct {
	components.Option
	Days int // 0 = indefinida
}

var suspensionDurations = []suspensionDuration{
	{components.Option{Value: "1", Label: "1 día"}, 1},
	{components.Option{Value: "7", Label: "7 días"}, 7},
	{components.Option{Value: "30", Label: "30 días"}, 30},
	{components.Option{Value: durationPermanent, Label: "Indefinida"}, 0},
}

// roleOptions son los roles para el formulario de cambio de rol; deben coincidir con el CHECK de users.role.
var roleOptions = []components.Option{
	{Value: auth.RoleUser, Label: auth.RoleLabel(auth.RoleUser)},
	{Value: auth.RoleModerator, Label: auth.RoleLabel(auth.RoleModerator)},
	{Value: auth.RoleAdmin, Label: auth.RoleLabel(auth.RoleAdmin)},
}

// ReportTarget es lo que se reporta: una historia pública y, opcionalmente, uno de sus
// capítulos publicados.
type ReportTarget struct {
	Story        stories.Story
	ChapterID    int64  // 0 si se reporta la historia entera
	ChapterTitle string // "" si se reporta la historia entera
}

// URL es la página de lo que se reporta, para volver a ella.
func (t ReportTarget) URL() string {
	if t.ChapterID == 0 {
		return t.Story.URL()
	}
	return stories.Chapter{ID: t.ChapterID}.URL(t.Story)
}

// Case es una historia con reportes abiertos, tal como sale en la cola de moderación.
type Case struct {
	StoryID        int64
	StoryTitle     string
	AuthorUsername string
	Hidden         bool
	OpenReports    int
	Reasons        []string  // motivos distintos de los reportes abiertos
	OldestReportAt time.Time // el reporte abierto que lleva más tiempo esperando
}

func (c Case) URL() string { return stories.Story{ID: c.StoryID}.ModerationURL() }

func (c Case) ReasonLabels() []string {
	labels := make([]string, len(c.Reasons))
	for i, r := range c.Reasons {
		labels[i] = reasonLabel(r)
	}
	return labels
}

// Report es un reporte con los nombres de quien lo hizo y quien lo cerró ya unidos.
type Report struct {
	ID               int64
	StoryID          int64
	ChapterID        *int64
	ChapterTitle     *string // nil si el reporte es de la historia entera, o el capítulo ya no existe
	ReporterUsername *string // nil si la cuenta se borró
	Reason           string
	Details          string
	Status           string
	ReviewerUsername *string
	ReviewedAt       *time.Time
	CreatedAt        time.Time
}

func (r Report) ReasonLabel() string { return reasonLabel(r.Reason) }
func (r Report) StatusLabel() string { return statusLabel(r.Status) }
func (r Report) IsOpen() bool        { return r.Status == StatusOpen }

// Action es una entrada del registro de moderación.
type Action struct {
	ID                int64
	ModeratorUsername *string // nil si la cuenta se borró
	Action            string
	TargetUsername    *string
	StoryID           *int64 // nil si la historia se borró
	Reason            string
	Details           string
	CreatedAt         time.Time
}

func (a Action) Label() string { return actionLabel(a.Action) }

// userModerationURL es la página de moderación de una cuenta.
func userModerationURL(username string) string {
	return "/moderation/users/" + url.PathEscape(username)
}

// formatDateTime escribe una fecha con hora, en el formato corto que usa el panel de moderación.
func formatDateTime(t time.Time) string { return t.Local().Format("02/01/2006 15:04") }
