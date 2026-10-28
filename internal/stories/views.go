package stories

import (
	"fmt"
	"net/url"
	"slices"
	"strconv"

	"github.com/diegovhdev/yortale/internal/auth"
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

// ExploreView es la página de explorar: búsqueda, filtros por géneros y tipo, y resultados paginados.
type ExploreView struct {
	Query    string
	GenreIDs []string // hasta maxGenres, ya validados
	Kind     string
	// ExcludeWarnings son las advertencias que el lector oculta, ya validadas.
	ExcludeWarnings []string
	// Tags y ExcludeTags son el texto de los filtros de tags tal como se escribió
	// (nombres separados por comas), para volver a mostrarlo en el formulario.
	Tags        string
	ExcludeTags string
	Genres      []Genre
	Stories     []Story
	Page        int
	HasNext     bool
}

// PageURL construye el enlace a otra página conservando la búsqueda y los filtros.
func (v ExploreView) PageURL(page int) string {
	q := url.Values{}
	if v.Query != "" {
		q.Set("q", v.Query)
	}
	for _, id := range v.GenreIDs {
		q.Add("genre", id)
	}
	if v.Kind != "" {
		q.Set("kind", v.Kind)
	}
	for _, w := range v.ExcludeWarnings {
		q.Add("exclude", w)
	}
	if v.Tags != "" {
		q.Set("tags", v.Tags)
	}
	if v.ExcludeTags != "" {
		q.Set("exclude_tags", v.ExcludeTags)
	}
	if page > 1 {
		q.Set("page", strconv.Itoa(page))
	}
	if len(q) == 0 {
		return "/stories"
	}
	return "/stories?" + q.Encode()
}

// HasGenre dice si el género está marcado en el filtro.
func (v ExploreView) HasGenre(id int16) bool {
	return slices.Contains(v.GenreIDs, strconv.Itoa(int(id)))
}

// Filtered dice si hay alguna búsqueda o filtro activo.
func (v ExploreView) Filtered() bool {
	return v.Query != "" || len(v.GenreIDs) > 0 || v.Kind != "" || len(v.ExcludeWarnings) > 0 ||
		v.Tags != "" || v.ExcludeTags != ""
}

// Excludes dice si el lector oculta las historias con esa advertencia.
func (v ExploreView) Excludes(warning string) bool {
	return slices.Contains(v.ExcludeWarnings, warning)
}

// SelectedGenresJSON son los géneros marcados, para el atributo data-selected del componente
// genreFilter de Alpine. Sin géneros codifica [] y no null, porque el JS espera un array.
func (v ExploreView) SelectedGenresJSON() string {
	if v.GenreIDs == nil {
		return "[]"
	}
	return mustJSON(v.GenreIDs)
}

// TagView es la página de un tag: sus historias publicadas, paginadas.
type TagView struct {
	Tag     Tag
	Stories []Story
	Page    int
	HasNext bool
}

// PageURL es el enlace a otra página del tag.
func (v TagView) PageURL(page int) string {
	if page > 1 {
		return fmt.Sprintf("%s?page=%d", v.Tag.URL(), page)
	}
	return v.Tag.URL()
}

// ExploreURL lleva a explorar con este tag ya puesto en el filtro, para combinarlo con otros.
func (v TagView) ExploreURL() string {
	return "/stories?" + url.Values{"tags": {v.Tag.Name}}.Encode()
}

// AuthorView es el perfil público de un autor.
type AuthorView struct {
	Author     auth.User
	Stories    []Story // solo publicadas
	TotalReads int64   // suma de los lectores de sus historias
	IsOwner    bool    // quien mira es el propio autor
}
