// Package comments es la feature de comentarios. De momento solo hay comentarios en los
// capítulos, con un nivel de respuestas: se responde a un comentario principal, nunca a una
// respuesta, y todas las respuestas de un hilo se muestran juntas debajo de él.
package comments

import (
	"fmt"
	"time"

	"github.com/diegovhdev/yortale/internal/auth"
)

const (
	// maxBodyRunes debe coincidir con el CHECK de comments.body.
	maxBodyRunes = 5000
	// threadsPerPage es cuántos comentarios principales se ven por página; sus respuestas
	// van siempre completas.
	threadsPerPage = 30
	// editedAfter: un comentario cambiado después de este margen se marca como editado,
	// para que corregir una errata justo al publicar no lo marque.
	editedAfter = time.Minute
)

type Comment struct {
	ID        int64
	StoryID   int64
	ChapterID int64
	ParentID  *int64 // nil en los comentarios principales
	UserID    *int64 // nil si la cuenta se borró
	// Username y DisplayName son de quien lo escribió; "" si la cuenta ya no existe.
	Username    string
	DisplayName string
	Body        string // "" si está borrado
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Deleted     bool
	Replies     []Comment // solo en los principales, sin las borradas
}

// AuthorName es el nombre para mostrar de quien lo escribió.
func (c Comment) AuthorName() string {
	switch {
	case c.Username == "":
		return "Cuenta eliminada"
	case c.DisplayName != "":
		return c.DisplayName
	default:
		return c.Username
	}
}

// AuthorURL es el perfil público de quien lo escribió, o "" si la cuenta ya no existe.
func (c Comment) AuthorURL() string {
	if c.Username == "" {
		return ""
	}
	return auth.ProfileURL(c.Username)
}

func (c Comment) IsReply() bool { return c.ParentID != nil }

func (c Comment) Edited() bool { return !c.Deleted && c.UpdatedAt.Sub(c.CreatedAt) > editedAfter }

// IsBy dice si el comentario es de ese usuario.
func (c Comment) IsBy(u *auth.User) bool { return u != nil && c.UserID != nil && *c.UserID == u.ID }

// Anchor es el id del elemento del comentario en la página del capítulo.
func (c Comment) Anchor() string { return fmt.Sprintf("comment-%d", c.ID) }

// RepliesID es el id de la lista de respuestas de un hilo, a la que htmx añade las nuevas.
func (c Comment) RepliesID() string { return fmt.Sprintf("replies-%d", c.ID) }

func (c Comment) URL() string       { return fmt.Sprintf("/comments/%d", c.ID) }
func (c Comment) EditURL() string   { return fmt.Sprintf("/comments/%d/edit", c.ID) }
func (c Comment) DeleteURL() string { return fmt.Sprintf("/comments/%d/delete", c.ID) }

// CreateURL es adonde se envían los comentarios nuevos (y las respuestas) de un capítulo.
func CreateURL(chapterID int64) string { return fmt.Sprintf("/chapters/%d/comments", chapterID) }

func formatDateTime(t time.Time) string { return t.Local().Format("02/01/2006 15:04") }
