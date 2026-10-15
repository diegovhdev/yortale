package stories

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/diegovhdev/yortale/internal/auth"
)

const (
	explorePageSize = 20
	maxChapterRunes = 200_000
	// maxSlugAttempts limita cuántos sufijos (-2, -3...) se prueban si el autor repite título.
	maxSlugAttempts = 50
)

type storyStore interface {
	Create(ctx context.Context, p StoryParams) (Story, error)
	Update(ctx context.Context, id int64, p StoryParams) (Story, error)
	Get(ctx context.Context, id int64) (Story, error)
	ListPublished(ctx context.Context, query string, genreID *int16, limit, offset int) ([]Story, error)
	ListByAuthor(ctx context.Context, authorID int64) ([]Story, error)
	Delete(ctx context.Context, id, authorID int64) error
	ListGenres(ctx context.Context) ([]Genre, error)
	RecordRead(ctx context.Context, userID, storyID, chapterID int64) error
}

type chapterStore interface {
	ListByStory(ctx context.Context, storyID int64, includeDrafts bool) ([]Chapter, error)
	Get(ctx context.Context, id int64) (Chapter, error)
	Create(ctx context.Context, storyID int64, p ChapterParams) (Chapter, error)
	Update(ctx context.Context, id int64, p ChapterParams) (Chapter, error)
	Delete(ctx context.Context, id int64) error
	Move(ctx context.Context, id int64, delta int) error
}

type Service struct {
	stories  storyStore
	chapters chapterStore
}

func NewService(stories storyStore, chapters chapterStore) *Service {
	return &Service{stories: stories, chapters: chapters}
}

func isAuthor(user *auth.User, s Story) bool {
	return user != nil && user.ID == s.AuthorID
}

// ---------- Historias ----------

// StoryInput son los valores del formulario de historia, tal como llegan.
type StoryInput struct {
	Title         string
	Synopsis      string
	GenreID       string
	Language      string
	ContentRating string
	Status        string // se ignora al crear: toda historia empieza como borrador
}

func (s *Service) Genres(ctx context.Context) ([]Genre, error) {
	return s.stories.ListGenres(ctx)
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
	if !validOption(Languages, p.Language) {
		errs["language"] = "Elige un idioma de la lista."
	}
	if !validOption(ContentRatings, p.ContentRating) {
		errs["content_rating"] = "Elige una clasificación de la lista."
	}
	if !validOption(Statuses, p.Status) {
		errs["status"] = "Elige un estado de la lista."
	}

	if in.GenreID != "" {
		id, err := strconv.ParseInt(in.GenreID, 10, 16)
		if err != nil {
			errs["genre_id"] = "Elige un género de la lista."
		} else {
			genres, err := s.stories.ListGenres(ctx)
			if err != nil {
				return StoryParams{}, err
			}
			genreID := int16(id)
			if !hasGenre(genres, genreID) {
				errs["genre_id"] = "Elige un género de la lista."
			}
			p.GenreID = &genreID
		}
	}

	if len(errs) > 0 {
		return p, errs
	}
	return p, nil
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
	Query   string
	GenreID *int16
	Page    int // empieza en 1
}

func (s *Service) Explore(ctx context.Context, f ExploreFilter) (stories []Story, hasNext bool, err error) {
	if f.Page < 1 {
		f.Page = 1
	}
	// Se pide uno de más para saber si hay página siguiente.
	stories, err = s.stories.ListPublished(ctx, f.Query, f.GenreID, explorePageSize+1, (f.Page-1)*explorePageSize)
	if err != nil {
		return nil, false, err
	}
	if len(stories) > explorePageSize {
		return stories[:explorePageSize], true, nil
	}
	return stories, false, nil
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
	c, err := s.chapters.Get(ctx, chapterID)
	if err != nil {
		return ChapterView{}, err
	}
	author := isAuthor(viewer, story)
	if c.StoryID != story.ID || (!c.IsPublished && !author) {
		return ChapterView{}, ErrNotFound
	}

	chapters, err := s.chapters.ListByStory(ctx, story.ID, author)
	if err != nil {
		return ChapterView{}, err
	}
	v := ChapterView{Story: story, Chapter: c, IsAuthor: author}
	for i, other := range chapters {
		if other.ID != c.ID {
			continue
		}
		if i > 0 {
			v.Prev = &chapters[i-1]
		}
		if i < len(chapters)-1 {
			v.Next = &chapters[i+1]
		}
	}

	if viewer != nil && !author && c.IsPublished {
		// Que falle el contador no debe impedir leer: se registra el error y se sigue.
		if err := s.stories.RecordRead(ctx, viewer.ID, story.ID, c.ID); err != nil {
			slog.Error("registrar lectura", "err", err, "story_id", story.ID, "chapter_id", c.ID)
		}
	}
	return v, nil
}
