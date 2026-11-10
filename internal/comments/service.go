package comments

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/diegovhdev/yortale/internal/auth"
	"github.com/diegovhdev/yortale/internal/stories"
)

type store interface {
	Get(ctx context.Context, id int64) (Comment, error)
	Threads(ctx context.Context, chapterID int64, limit, offset int) ([]Comment, bool, error)
	ThreadsBefore(ctx context.Context, chapterID, threadID int64) (int, error)
	Count(ctx context.Context, chapterID int64) (int, error)
	Create(ctx context.Context, p CreateParams) (Comment, error)
	UpdateBody(ctx context.Context, id int64, body string) (Comment, error)
	Delete(ctx context.Context, id int64) (Comment, error)
}

// chapterFinder busca capítulos con las reglas de visibilidad de stories. Lo cumple *stories.Service.
type chapterFinder interface {
	VisibleChapter(ctx context.Context, viewer *auth.User, chapterID int64) (stories.Story, stories.Chapter, error)
}

type Service struct {
	store    store
	chapters chapterFinder
}

func NewService(store store, chapters chapterFinder) *Service {
	return &Service{store: store, chapters: chapters}
}

// ---------- Permisos ----------

// Scope es dónde se comenta y quién mira. Reúne las reglas de permisos, que también usan
// las plantillas para decidir qué controles mostrar.
type Scope struct {
	Viewer  *auth.User // nil para los visitantes
	Story   stories.Story
	Chapter stories.Chapter
}

// CanPost dice si el lector puede comentar: hace falta una cuenta, y que el capítulo y la
// historia estén publicados y la historia no esté oculta por moderación.
func (s Scope) CanPost() bool {
	return s.Viewer != nil && s.Chapter.IsPublished && !s.Story.IsDraft() && !s.Story.IsHidden()
}

// CanEdit: solo quien lo escribió, y mientras se pueda comentar en el capítulo.
func (s Scope) CanEdit(c Comment) bool {
	return !c.Deleted && c.IsBy(s.Viewer) && s.CanPost()
}

// CanDelete: quien lo escribió, el autor de la historia (que cuida su espacio) y el equipo
// de moderación.
func (s Scope) CanDelete(c Comment) bool {
	if c.Deleted || s.Viewer == nil {
		return false
	}
	return c.IsBy(s.Viewer) || s.Viewer.ID == s.Story.AuthorID || s.Viewer.IsStaff()
}

// IsStoryAuthor dice si el comentario es del autor de la historia, para destacarlo.
func (s Scope) IsStoryAuthor(c Comment) bool {
	return c.UserID != nil && *c.UserID == s.Story.AuthorID
}

func (s Scope) ChapterURL() string { return s.Chapter.URL(s.Story) }

// PageURL es la página de comentarios del capítulo, con el ancla del final.
func (s Scope) PageURL(page int, anchor string) string {
	u := s.ChapterURL()
	if page > 1 {
		u += "?comments=" + strconv.Itoa(page)
	}
	return u + "#" + anchor
}

// LoginURL lleva a iniciar sesión y vuelve al capítulo.
func (s Scope) LoginURL() string {
	return "/login?next=" + url.QueryEscape(s.ChapterURL())
}

// scope comprueba que viewer puede leer el capítulo. Los errores de stories se traducen
// a los de este paquete.
func (s *Service) scope(ctx context.Context, viewer *auth.User, chapterID int64) (Scope, error) {
	story, chapter, err := s.chapters.VisibleChapter(ctx, viewer, chapterID)
	if errors.Is(err, stories.ErrNotFound) {
		return Scope{}, ErrNotFound
	}
	if err != nil {
		return Scope{}, err
	}
	return Scope{Viewer: viewer, Story: story, Chapter: chapter}, nil
}

// Target es un comentario con su contexto, para las acciones sobre él.
type Target struct {
	Scope
	Comment Comment
}

// Find devuelve el comentario si viewer puede ver su capítulo. Incluye los borrados, que
// se muestran como «Comentario borrado».
func (s *Service) Find(ctx context.Context, viewer *auth.User, id int64) (Target, error) {
	c, err := s.store.Get(ctx, id)
	if err != nil {
		return Target{}, err
	}
	scope, err := s.scope(ctx, viewer, c.ChapterID)
	if err != nil {
		return Target{}, err
	}
	return Target{Scope: scope, Comment: c}, nil
}

// ---------- Leer ----------

// Section es la sección de comentarios al pie de un capítulo.
type Section struct {
	Scope
	Threads []Comment
	Total   int // comentarios sin borrar, respuestas incluidas
	Page    int
	HasNext bool
}

// ChapterSection reúne una página de comentarios. La visibilidad del capítulo ya la comprobó
// quien lo muestra (stories.Service.ReadChapter), así que no se vuelve a consultar.
func (s *Service) ChapterSection(ctx context.Context, viewer *auth.User, story stories.Story, chapter stories.Chapter, page int) (Section, error) {
	sec := Section{Scope: Scope{Viewer: viewer, Story: story, Chapter: chapter}, Page: max(page, 1)}
	var err error
	sec.Threads, sec.HasNext, err = s.store.Threads(ctx, chapter.ID, threadsPerPage, (sec.Page-1)*threadsPerPage)
	if err != nil {
		return sec, err
	}
	// En la primera página sin más páginas el total sale de lo ya cargado; si no, se cuenta.
	if sec.Page == 1 && !sec.HasNext {
		for _, t := range sec.Threads {
			if !t.Deleted {
				sec.Total++
			}
			sec.Total += len(t.Replies)
		}
		return sec, nil
	}
	sec.Total, err = s.store.Count(ctx, chapter.ID)
	return sec, err
}

func (s *Service) Count(ctx context.Context, chapterID int64) (int, error) {
	return s.store.Count(ctx, chapterID)
}

// Permalink es la dirección del comentario en su capítulo, en la página de comentarios donde
// está su hilo.
func (s *Service) Permalink(ctx context.Context, t Target) (string, error) {
	threadID := t.Comment.ID
	if t.Comment.ParentID != nil {
		threadID = *t.Comment.ParentID
	}
	before, err := s.store.ThreadsBefore(ctx, t.Chapter.ID, threadID)
	if err != nil {
		return "", err
	}
	return t.PageURL(before/threadsPerPage+1, t.Comment.Anchor()), nil
}

// ---------- Escribir ----------

// Input son los valores del formulario de comentario, tal como llegan.
type Input struct {
	Body     string
	ParentID string // vacío en un comentario principal
}

// validateBody unifica los saltos de línea (los navegadores envían \r\n, que cuentan para
// el CHECK) y comprueba la longitud.
func validateBody(raw string) (string, error) {
	body := strings.TrimSpace(strings.ReplaceAll(raw, "\r\n", "\n"))
	switch n := utf8.RuneCountInString(body); {
	case n == 0:
		return body, auth.ValidationErrors{"body": "Escribe un comentario."}
	case n > maxBodyRunes:
		return body, auth.ValidationErrors{"body": fmt.Sprintf(
			"El comentario no puede tener más de %d caracteres (tiene %d).", maxBodyRunes, n)}
	}
	return body, nil
}

// Create publica un comentario o una respuesta. Devuelve el Scope aunque falle la validación,
// para que el handler pueda volver a mostrar el formulario.
func (s *Service) Create(ctx context.Context, viewer *auth.User, chapterID int64, in Input) (Scope, Comment, error) {
	scope, err := s.scope(ctx, viewer, chapterID)
	if err != nil {
		return scope, Comment{}, err
	}
	if !scope.CanPost() {
		return scope, Comment{}, ErrForbidden
	}
	body, err := validateBody(in.Body)
	if err != nil {
		return scope, Comment{}, err
	}
	p := CreateParams{UserID: viewer.ID, StoryID: scope.Story.ID, ChapterID: scope.Chapter.ID, Body: body}
	if in.ParentID != "" {
		parentID, err := strconv.ParseInt(in.ParentID, 10, 64)
		if err != nil {
			return scope, Comment{}, ErrNotFound
		}
		parent, err := s.store.Get(ctx, parentID)
		// Solo se responde a comentarios principales sin borrar del mismo capítulo.
		if errors.Is(err, ErrNotFound) || (err == nil && (parent.ChapterID != scope.Chapter.ID ||
			parent.IsReply() || parent.Deleted)) {
			return scope, Comment{}, auth.ValidationErrors{"parent": "El comentario al que respondes ya no existe."}
		}
		if err != nil {
			return scope, Comment{}, err
		}
		p.ParentID = &parent.ID
	}
	c, err := s.store.Create(ctx, p)
	return scope, c, err
}

// Update cambia el texto de un comentario propio.
func (s *Service) Update(ctx context.Context, viewer *auth.User, id int64, body string) (Target, error) {
	t, err := s.Find(ctx, viewer, id)
	if err != nil {
		return t, err
	}
	if t.Comment.Deleted {
		return t, ErrNotFound
	}
	if !t.CanEdit(t.Comment) {
		return t, ErrForbidden
	}
	clean, err := validateBody(body)
	if err != nil {
		return t, err
	}
	t.Comment, err = s.store.UpdateBody(ctx, id, clean)
	return t, err
}

// Delete borra un comentario (borrado suave). Sus respuestas se quedan.
func (s *Service) Delete(ctx context.Context, viewer *auth.User, id int64) (Target, error) {
	t, err := s.Find(ctx, viewer, id)
	if err != nil {
		return t, err
	}
	if t.Comment.Deleted {
		return t, ErrNotFound
	}
	if !t.CanDelete(t.Comment) {
		return t, ErrForbidden
	}
	t.Comment, err = s.store.Delete(ctx, id)
	return t, err
}
