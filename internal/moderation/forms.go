package moderation

import (
	"fmt"
	"strconv"

	"github.com/diegovhdev/yortale/internal/auth"
	"github.com/diegovhdev/yortale/internal/stories"
	"github.com/diegovhdev/yortale/web/components"
)

// Rutas de las acciones. Las páginas de una historia y de una cuenta están en
// stories.Story.ModerationURL y userModerationURL.
func hideURL(storyID int64) string        { return fmt.Sprintf("/moderation/stories/%d/hide", storyID) }
func unhideURL(storyID int64) string      { return fmt.Sprintf("/moderation/stories/%d/unhide", storyID) }
func dismissURL(storyID int64) string     { return fmt.Sprintf("/moderation/stories/%d/dismiss", storyID) }
func suspendURL(username string) string   { return userModerationURL(username) + "/suspend" }
func unsuspendURL(username string) string { return userModerationURL(username) + "/unsuspend" }
func roleURL(username string) string      { return userModerationURL(username) + "/role" }

// ReportForm es el formulario para reportar una historia o un capítulo.
type ReportForm struct {
	Target  ReportTarget
	Own     bool // es del propio usuario: no se muestra el formulario
	Reason  string
	Details string
	Errors  map[string]string
	General string
}

// StoryIDValue y ChapterIDValue son los campos ocultos del formulario.
func (f ReportForm) StoryIDValue() string { return strconv.FormatInt(f.Target.Story.ID, 10) }
func (f ReportForm) ChapterIDValue() string {
	if f.Target.ChapterID == 0 {
		return ""
	}
	return strconv.FormatInt(f.Target.ChapterID, 10)
}

// HideForm es el formulario para ocultar una historia, con el motivo que verá el autor.
type HideForm struct {
	Action  string
	Reason  string
	Errors  map[string]string
	General string
}

// SuspendForm es el formulario para suspender una cuenta.
type SuspendForm struct {
	Action    string
	Durations []components.Option // las que puede elegir quien modera
	Duration  string
	Reason    string
	Errors    map[string]string
	General   string
}

func newSuspendForm(actor, target auth.User) SuspendForm {
	return SuspendForm{Action: suspendURL(target.Username), Durations: durationsFor(actor), Duration: "7"}
}

// RoleForm es el formulario para cambiar el rol de una cuenta (solo administradores).
type RoleForm struct {
	Action  string
	Role    string
	Options []components.Option
	Error   string
}

// StoryCaseView es la página de moderación de una historia.
type StoryCaseView struct {
	Story   stories.Story
	Reports []Report // primero los abiertos
	Actions []Action
	Hide    HideForm
}

func (v StoryCaseView) OpenReports() int {
	n := 0
	for _, r := range v.Reports {
		if r.IsOpen() {
			n++
		}
	}
	return n
}

// ChapterURL es la dirección de lectura del capítulo de un reporte.
func (v StoryCaseView) ChapterURL(r Report) string {
	return stories.Chapter{ID: *r.ChapterID}.URL(v.Story)
}

func (v StoryCaseView) DismissLabel() string {
	if n := v.OpenReports(); n != 1 {
		return fmt.Sprintf("Descartar los %d reportes abiertos", n)
	}
	return "Descartar el reporte abierto"
}

func (v StoryCaseView) AuthorModerationURL() string { return userModerationURL(v.Story.AuthorUsername) }
func (v StoryCaseView) UnhideURL() string           { return unhideURL(v.Story.ID) }
func (v StoryCaseView) DismissURL() string          { return dismissURL(v.Story.ID) }

// UserView es la página de moderación de una cuenta: qué puede hacer quien modera y su historial.
type UserView struct {
	User          auth.User
	Actions       []Action
	CanSuspend    bool
	CanChangeRole bool
	Suspend       SuspendForm
	Role          RoleForm
}

func (v UserView) UnsuspendURL() string { return unsuspendURL(v.User.Username) }

// LogView es una página del registro de moderación.
type LogView struct {
	Actions []Action
	Page    int
	HasNext bool
}

func (v LogView) PageURL(page int) string {
	if page > 1 {
		return fmt.Sprintf("/moderation/log?page=%d", page)
	}
	return "/moderation/log"
}
