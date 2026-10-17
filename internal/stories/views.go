package stories

import (
	"net/url"
	"strconv"
)

// StoryView es lo que muestra la página de una historia.
type StoryView struct {
	Story    Story
	Chapters []Chapter // sin texto; incluye borradores solo si IsAuthor
	IsAuthor bool
}

// ChapterView es lo que muestra la página de lectura de un capítulo.
type ChapterView struct {
	Story    Story
	Chapter  Chapter
	Prev     *Chapter
	Next     *Chapter
	IsAuthor bool
}

// ExploreView es la página de explorar: búsqueda, filtros por género y tipo, y resultados paginados.
type ExploreView struct {
	Query   string
	GenreID string
	Kind    string
	Genres  []Genre
	Stories []Story
	Page    int
	HasNext bool
}

// PageURL construye el enlace a otra página conservando la búsqueda y los filtros.
func (v ExploreView) PageURL(page int) string {
	q := url.Values{}
	if v.Query != "" {
		q.Set("q", v.Query)
	}
	if v.GenreID != "" {
		q.Set("genre", v.GenreID)
	}
	if v.Kind != "" {
		q.Set("kind", v.Kind)
	}
	if page > 1 {
		q.Set("page", strconv.Itoa(page))
	}
	if len(q) == 0 {
		return "/stories"
	}
	return "/stories?" + q.Encode()
}
