package stories

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/diegovhdev/yortale/internal/auth"
)

const (
	explorePageSize = 20
	maxChapterRunes = 200_000
	// maxSlugAttempts limita cuántos sufijos (-2, -3...) se prueban si el autor repite título.
	maxSlugAttempts = 50
	// genresTTL es cuánto se guardan en memoria los géneros. Solo cambian a mano en la base
	// de datos, así que con esto un cambio se ve en unos minutos sin reiniciar el servidor.
	genresTTL = 10 * time.Minute
)

type storyStore interface {
	Create(ctx context.Context, p StoryParams) (Story, error)
	Update(ctx context.Context, id int64, p StoryParams) (Story, error)
	Get(ctx context.Context, id int64) (Story, error)
	ListPublished(ctx context.Context, query string, genreIDs []int16, kind string, limit, offset int) ([]Story, error)
	ListByAuthor(ctx context.Context, authorID int64) ([]Story, error)
	ListPublishedByAuthor(ctx context.Context, authorID int64) ([]Story, error)
	Delete(ctx context.Context, id, authorID int64) error
	ListGenres(ctx context.Context) ([]Genre, error)
	RecordRead(ctx context.Context, userID, storyID, chapterID int64) error
}

type chapterStore interface {
	ListByStory(ctx context.Context, storyID int64, includeDrafts bool) ([]Chapter, error)
	ListForReading(ctx context.Context, storyID, chapterID int64, includeDrafts bool) ([]Chapter, error)
	Get(ctx context.Context, id int64) (Chapter, error)
	Create(ctx context.Context, storyID int64, p ChapterParams) (Chapter, error)
	Update(ctx context.Context, id int64, p ChapterParams) (Chapter, error)
	Delete(ctx context.Context, id int64) error
	Move(ctx context.Context, id int64, delta int) error
}

// authorStore busca a los autores para su perfil público. Lo cumple *auth.Service.
type authorStore interface {
	UserByUsername(ctx context.Context, username string) (auth.User, error)
}

type Service struct {
	stories  storyStore
	chapters chapterStore
	authors  authorStore

	genresMu      sync.Mutex
	genres        []Genre
	genresExpires time.Time
}

func NewService(stories storyStore, chapters chapterStore, authors authorStore) *Service {
	return &Service{stories: stories, chapters: chapters, authors: authors}
}

func isAuthor(user *auth.User, s Story) bool {
	return user != nil && user.ID == s.AuthorID
}

// ---------- Historias ----------

// StoryInput son los valores del formulario de historia, tal como llegan.
type StoryInput struct {
	Title         string
	Synopsis      string
	Kind          string
	GenreIDs      []string // en el orden en que se muestran; los vacíos se ignoran
	Language      string
	ContentRating string
	Status        string // se ignora al crear: toda historia empieza como borrador
}

// Genres devuelve todos los géneros, desde la memoria mientras no pase genresTTL.
// Cada llamada recibe su propia copia, así que puede modificarla.
func (s *Service) Genres(ctx context.Context) ([]Genre, error) {
	s.genresMu.Lock()
	defer s.genresMu.Unlock()
	if s.genres == nil || time.Now().After(s.genresExpires) {
		genres, err := s.stories.ListGenres(ctx)
		if err != nil {
			return nil, err
		}
		s.genres, s.genresExpires = genres, time.Now().Add(genresTTL)
	}
	return slices.Clone(s.genres), nil
}

// Create valida los datos y crea la historia como borrador. Devuelve auth.ValidationErrors si hay errores.
func (s *Service) Create(ctx context.Context, authorID int64, in StoryInput) (Story, error) {
	in.Status = StatusDraft
	p, err := s.validateStory(ctx, in)
	if err != nil {
		return Story{}, err
	}
	p.AuthorID = authorID
	return saveWithUniqueSlug(Slugify(p.Title), func(slug string) (Story, error) {
		p.Slug = slug
		return s.stories.Create(ctx, p)
	})
}

// Update valida los datos y actualiza la historia, que debe ser del autor.
// El slug solo se recalcula si cambia el título; los enlaces viejos siguen funcionando
// porque la historia se busca por id y se redirige a la URL nueva.
func (s *Service) Update(ctx context.Context, authorID, id int64, in StoryInput) (Story, error) {
	current, err := s.ForAuthor(ctx, authorID, id)
	if err != nil {
		return Story{}, err
	}
	p, err := s.validateStory(ctx, in)
	var verrs auth.ValidationErrors
	if err != nil && !errors.As(err, &verrs) {
		return Story{}, err
	}
	if p.Status != StatusDraft && current.ChapterCount == 0 {
		if verrs == nil {
			verrs = auth.ValidationErrors{}
		}
		verrs["status"] = "Publica al menos un capítulo antes de sacar la historia del borrador."
	}
	if len(verrs) > 0 {
		return Story{}, verrs
	}

	p.AuthorID = authorID
	if p.Title == current.Title {
		p.Slug = current.Slug
		return s.stories.Update(ctx, id, p)
	}
	return saveWithUniqueSlug(Slugify(p.Title), func(slug string) (Story, error) {
		p.Slug = slug
		return s.stories.Update(ctx, id, p)
	})
}

// saveWithUniqueSlug prueba base, base-2, base-3... hasta que el slug no choque con otra
// historia del mismo autor (UNIQUE (author_id, slug)).
func saveWithUniqueSlug(base string, save func(slug string) (Story, error)) (Story, error) {
	for i := 1; i <= maxSlugAttempts; i++ {
		slug := base
		if i > 1 {
			slug = fmt.Sprintf("%s-%d", base, i)
		}
		story, err := save(slug)
		if !errors.Is(err, ErrSlugTaken) {
			return story, err
		}
	}
	return Story{}, fmt.Errorf("no se encontró un slug libre para %q", base)
}

func (s *Service) validateStory(ctx context.Context, in StoryInput) (StoryParams, error) {
	p := StoryParams{
		Title:         strings.TrimSpace(in.Title),
		Synopsis:      normalizeText(in.Synopsis),
		Kind:          in.Kind,
		Language:      in.Language,
		ContentRating: in.ContentRating,
		Status:        in.Status,
	}

	errs := auth.ValidationErrors{}
	switch n := utf8.RuneCountInString(p.Title); {
	case n == 0:
		errs["title"] = "Escribe un título."
	case n > 200:
		errs["title"] = "El título no puede tener más de 200 caracteres."
	}
	if utf8.RuneCountInString(p.Synopsis) > 4000 {
		errs["synopsis"] = "La sinopsis no puede tener más de 4000 caracteres."
	}
	if !validOption(Kinds, p.Kind) {
		errs["kind"] = "Elige si la historia es original o un fanfic."
	}
	if !validOption(Languages, p.Language) {
		errs["language"] = "Elige un idioma de la lista."
	}
	if !validOption(ContentRatings, p.ContentRating) {
		errs["content_rating"] = "Elige una clasificación de la lista."
	}
	if !validOption(Statuses, p.Status) {
		errs["status"] = "Elige un estado de la lista."
	}

	if err := s.validateGenres(ctx, in.GenreIDs, &p, errs); err != nil {
		return StoryParams{}, err
	}

	if len(errs) > 0 {
		return p, errs
	}
	return p, nil
}

// validateGenres pasa a p.GenreIDs los géneros elegidos, en el orden en que llegan: con JS, el
// orden en que el autor los marcó; sin JS, el alfabético del formulario. Los vacíos se ignoran.
// Todos los errores van al campo genres.
func (s *Service) validateGenres(ctx context.Context, ids []string, p *StoryParams, errs auth.ValidationErrors) error {
	var genres []Genre
	for _, raw := range ids {
		if raw == "" {
			continue
		}
		id, err := strconv.ParseInt(raw, 10, 16)
		if err != nil {
			errs[genresField] = "Elige géneros de la lista."
			continue
		}
		if genres == nil {
			if genres, err = s.Genres(ctx); err != nil {
				return err
			}
		}
		genreID := int16(id)
		switch {
		case !hasGenre(genres, genreID):
			errs[genresField] = "Elige géneros de la lista."
		case slices.Contains(p.GenreIDs, genreID):
			errs[genresField] = "No repitas géneros."
		default:
			p.GenreIDs = append(p.GenreIDs, genreID)
		}
	}
	if len(p.GenreIDs) > maxGenres {
		errs[genresField] = fmt.Sprintf("Elige como máximo %d géneros.", maxGenres)
	}
	return nil
}

func hasGenre(genres []Genre, id int16) bool {
	for _, g := range genres {
		if g.ID == id {
			return true
		}
	}
	return false
}

// normalizeText unifica los saltos de línea (los navegadores envían \r\n) y quita espacios sobrantes.
func normalizeText(s string) string {
	return strings.TrimSpace(strings.ReplaceAll(s, "\r\n", "\n"))
}

// Visible devuelve la historia si el usuario puede verla: los borradores solo los ve su autor.
// viewer es nil para los visitantes.
func (s *Service) Visible(ctx context.Context, viewer *auth.User, id int64) (Story, error) {
	story, err := s.stories.Get(ctx, id)
	if err != nil {
		return Story{}, err
	}
	if story.IsDraft() && !isAuthor(viewer, story) {
		return Story{}, ErrNotFound
	}
	return story, nil
}

// ForAuthor devuelve la historia solo si authorID es su autor.
func (s *Service) ForAuthor(ctx context.Context, authorID, id int64) (Story, error) {
	story, err := s.stories.Get(ctx, id)
	if err != nil {
		return Story{}, err
	}
	if story.AuthorID != authorID {
		return Story{}, ErrNotFound
	}
	return story, nil
}

// StoryPage reúne lo que muestra la página de una historia.
func (s *Service) StoryPage(ctx context.Context, viewer *auth.User, id int64) (StoryView, error) {
	story, err := s.Visible(ctx, viewer, id)
	if err != nil {
		return StoryView{}, err
	}
	author := isAuthor(viewer, story)
	chapters, err := s.chapters.ListByStory(ctx, story.ID, author)
	if err != nil {
		return StoryView{}, err
	}
	return StoryView{Story: story, Chapters: chapters, IsAuthor: author}, nil
}

func (s *Service) Delete(ctx context.Context, authorID, id int64) error {
	return s.stories.Delete(ctx, id, authorID)
}

func (s *Service) ByAuthor(ctx context.Context, authorID int64) ([]Story, error) {
	return s.stories.ListByAuthor(ctx, authorID)
}

// ExploreFilter son los parámetros de la página de explorar.
type ExploreFilter struct {
	Query    string
	GenreIDs []int16 // la historia debe tener todos, en cualquier orden; vacío no filtra
	Kind     string  // "" para todos los tipos
	Page     int     // empieza en 1
}

func (s *Service) Explore(ctx context.Context, f ExploreFilter) (stories []Story, hasNext bool, err error) {
	if f.Page < 1 {
		f.Page = 1
	}
	// Se pide uno de más para saber si hay página siguiente.
	stories, err = s.stories.ListPublished(ctx, f.Query, f.GenreIDs, f.Kind, explorePageSize+1, (f.Page-1)*explorePageSize)
	if err != nil {
		return nil, false, err
	}
	if len(stories) > explorePageSize {
		return stories[:explorePageSize], true, nil
	}
	return stories, false, nil
}

// ---------- Perfil del autor ----------

// AuthorPage reúne lo que muestra el perfil público de un autor: sus datos y sus historias
// publicadas (los borradores no salen, ni siquiera para el propio autor, porque es la vista
// pública; los suyos los ve en «Mis historias»).
func (s *Service) AuthorPage(ctx context.Context, viewer *auth.User, username string) (AuthorView, error) {
	author, err := s.authors.UserByUsername(ctx, username)
	if errors.Is(err, auth.ErrNotFound) {
		return AuthorView{}, ErrNotFound
	}
	if err != nil {
		return AuthorView{}, err
	}
	stories, err := s.stories.ListPublishedByAuthor(ctx, author.ID)
	if err != nil {
		return AuthorView{}, err
	}
	v := AuthorView{Author: author, Stories: stories, IsOwner: viewer != nil && viewer.ID == author.ID}
	for _, st := range stories {
		v.TotalReads += st.ReadsCount
	}
	return v, nil
}

// ---------- Capítulos ----------

type ChapterInput struct {
	Title   string
	Content string
	Publish bool
}

func validateChapter(in ChapterInput) (ChapterParams, error) {
	p := ChapterParams{
		Title:   strings.TrimSpace(in.Title),
		Content: normalizeText(in.Content),
		Publish: in.Publish,
	}
	p.WordCount = countWords(p.Content)

	errs := auth.ValidationErrors{}
	switch n := utf8.RuneCountInString(p.Title); {
	case n == 0:
		errs["title"] = "Escribe un título."
	case n > 200:
		errs["title"] = "El título no puede tener más de 200 caracteres."
	}
	switch n := utf8.RuneCountInString(p.Content); {
	case n == 0:
		errs["content"] = "El capítulo está vacío."
	case n > maxChapterRunes:
		errs["content"] = "El capítulo no puede tener más de 200 000 caracteres. Divídelo en varios."
	}
	if len(errs) > 0 {
		return p, errs
	}
	return p, nil
}

func (s *Service) CreateChapter(ctx context.Context, authorID, storyID int64, in ChapterInput) (Story, Chapter, error) {
	story, err := s.ForAuthor(ctx, authorID, storyID)
	if err != nil {
		return Story{}, Chapter{}, err
	}
	p, err := validateChapter(in)
	if err != nil {
		return story, Chapter{}, err
	}
	c, err := s.chapters.Create(ctx, story.ID, p)
	return story, c, err
}

// ChapterForAuthor devuelve el capítulo y su historia solo si authorID es el autor.
func (s *Service) ChapterForAuthor(ctx context.Context, authorID, chapterID int64) (Story, Chapter, error) {
	c, err := s.chapters.Get(ctx, chapterID)
	if err != nil {
		return Story{}, Chapter{}, err
	}
	story, err := s.ForAuthor(ctx, authorID, c.StoryID)
	if err != nil {
		return Story{}, Chapter{}, err
	}
	return story, c, nil
}

func (s *Service) UpdateChapter(ctx context.Context, authorID, chapterID int64, in ChapterInput) (Story, Chapter, error) {
	story, c, err := s.ChapterForAuthor(ctx, authorID, chapterID)
	if err != nil {
		return Story{}, Chapter{}, err
	}
	p, err := validateChapter(in)
	if err != nil {
		return story, c, err
	}
	c, err = s.chapters.Update(ctx, chapterID, p)
	return story, c, err
}

func (s *Service) DeleteChapter(ctx context.Context, authorID, chapterID int64) (Story, error) {
	story, _, err := s.ChapterForAuthor(ctx, authorID, chapterID)
	if err != nil {
		return Story{}, err
	}
	return story, s.chapters.Delete(ctx, chapterID)
}

// MoveChapter sube (up = true) o baja el capítulo una posición.
func (s *Service) MoveChapter(ctx context.Context, authorID, chapterID int64, up bool) (Story, error) {
	story, _, err := s.ChapterForAuthor(ctx, authorID, chapterID)
	if err != nil {
		return Story{}, err
	}
	delta := 1
	if up {
		delta = -1
	}
	return story, s.chapters.Move(ctx, chapterID, delta)
}

// ReadChapter devuelve el capítulo con su historia y sus vecinos, y registra la lectura.
// Los capítulos sin publicar solo los ve el autor.
func (s *Service) ReadChapter(ctx context.Context, viewer *auth.User, storyID, chapterID int64) (ChapterView, error) {
	story, err := s.Visible(ctx, viewer, storyID)
	if err != nil {
		return ChapterView{}, err
	}
	author := isAuthor(viewer, story)
	// Una sola consulta trae el índice y el texto del capítulo pedido. Si el capítulo es de
	// otra historia, o es un borrador y no lo pide el autor, no está en la lista.
	chapters, err := s.chapters.ListForReading(ctx, story.ID, chapterID, author)
	if err != nil {
		return ChapterView{}, err
	}
	i := slices.IndexFunc(chapters, func(c Chapter) bool { return c.ID == chapterID })
	if i < 0 {
		return ChapterView{}, ErrNotFound
	}
	c := chapters[i]
	v := ChapterView{Story: story, Chapter: c, IsAuthor: author}
	if i > 0 {
		v.Prev = &chapters[i-1]
	}
	if i < len(chapters)-1 {
		v.Next = &chapters[i+1]
	}

	if viewer != nil && !author && c.IsPublished {
		// Que falle el contador no debe impedir leer: se registra el error y se sigue.
		if err := s.stories.RecordRead(ctx, viewer.ID, story.ID, c.ID); err != nil {
			slog.Error("registrar lectura", "err", err, "story_id", story.ID, "chapter_id", c.ID)
		}
	}
	return v, nil
}
