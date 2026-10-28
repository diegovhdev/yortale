package stories

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// StoryForm guarda lo que el autor escribió y los errores, para volver a mostrar el formulario.
// El mismo formulario sirve para crear y para editar.
type StoryForm struct {
	Action        string // URL a la que se envía
	IsNew         bool   // al crear no se elige estado: la historia empieza como borrador
	CancelURL     string
	Title         string
	Synopsis      string
	Kind          string
	GenreIDs      []string // ids marcados, en orden: el de marcado con JS, alfabético sin JS
	Language      string
	ContentRating string
	Warnings      []string // al crear empieza vacío, para que el autor tenga que elegir
	Tags          []string // tal como llegan: un valor por tag con JS, un texto con comas sin JS
	Status        string
	Genres        []Genre
	Errors        map[string]string // error por campo, por ejemplo Errors["title"]
	General       string            // error que no pertenece a un campo concreto
}

func newStoryForm(genres []Genre) StoryForm {
	return StoryForm{
		Action:        "/stories",
		IsNew:         true,
		CancelURL:     "/my/stories",
		Kind:          KindOriginal,
		Language:      "es",
		ContentRating: "general",
		Status:        StatusDraft,
		Genres:        genres,
	}
}

func editStoryForm(s Story, genres []Genre) StoryForm {
	f := StoryForm{
		Action:        s.EditURL(),
		CancelURL:     s.URL(),
		Title:         s.Title,
		Synopsis:      s.Synopsis,
		Kind:          s.Kind,
		Language:      s.Language,
		ContentRating: s.ContentRating,
		Warnings:      s.Warnings,
		Status:        s.Status,
		Genres:        genres,
	}
	for _, g := range s.Genres {
		f.GenreIDs = append(f.GenreIDs, strconv.Itoa(int(g.ID)))
	}
	for _, t := range s.Tags {
		f.Tags = append(f.Tags, t.Name)
	}
	return f
}

func (f StoryForm) Input() StoryInput {
	return StoryInput{
		Title:         f.Title,
		Synopsis:      f.Synopsis,
		Kind:          f.Kind,
		GenreIDs:      f.GenreIDs,
		Language:      f.Language,
		ContentRating: f.ContentRating,
		Warnings:      f.Warnings,
		Tags:          f.Tags,
		Status:        f.Status,
	}
}

// genresField es el name de las casillas de género y la clave de sus errores.
const genresField = "genres"

// HasGenre dice si el género está marcado en el formulario.
func (f StoryForm) HasGenre(id int16) bool {
	return slices.Contains(f.GenreIDs, strconv.Itoa(int(id)))
}

// warningsField es el name de las casillas de advertencias y la clave de sus errores.
const warningsField = "warnings"

// HasWarning dice si la advertencia está marcada en el formulario.
func (f StoryForm) HasWarning(value string) bool {
	return slices.Contains(f.Warnings, value)
}

// WarningsJSON son las advertencias marcadas, para el atributo data-selected del componente
// warningChecks de Alpine. Sin ninguna codifica [] y no null, porque el JS espera un array.
func (f StoryForm) WarningsJSON() string {
	if f.Warnings == nil {
		return "[]"
	}
	return mustJSON(f.Warnings)
}

// tagsField es el name del campo de tags y la clave de sus errores.
const tagsField = "tags"

// TagsText son los tags en un solo texto separado por comas, para el campo sin JS.
func (f StoryForm) TagsText() string {
	return strings.Join(cleanTagNames(f.Tags), ", ")
}

// TagsJSON son los tags, uno por elemento, para el atributo data-selected del componente
// tagPicker de Alpine.
func (f StoryForm) TagsJSON() string {
	names := cleanTagNames(f.Tags)
	if names == nil {
		return "[]"
	}
	return mustJSON(names)
}

// genreChoice es un género tal como lo recibe el componente genreChecks de Alpine. El id va
// como texto para compararlo directamente con el value de las casillas.
type genreChoice struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// GenreChoicesJSON son todos los géneros, para el atributo data-genres.
func (f StoryForm) GenreChoicesJSON() string {
	choices := make([]genreChoice, len(f.Genres))
	for i, g := range f.Genres {
		choices[i] = genreChoice{ID: strconv.Itoa(int(g.ID)), Name: g.Name}
	}
	return mustJSON(choices)
}

// SelectedGenresJSON son los ids marcados, en orden, para el atributo data-selected.
// Al editar, es el orden guardado.
func (f StoryForm) SelectedGenresJSON() string {
	ids := []string{}
	for _, id := range f.GenreIDs {
		if id != "" {
			ids = append(ids, id)
		}
	}
	return mustJSON(ids)
}

// mustJSON solo recibe slices de strings y structs de strings, que siempre se pueden codificar.
func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

// ChapterForm es el formulario de capítulo, para crear y para editar.
type ChapterForm struct {
	Story     Story
	Action    string
	IsNew     bool
	CancelURL string
	Title     string
	Content   string
	Publish   bool
	Errors    map[string]string
	General   string
}

func newChapterForm(s Story) ChapterForm {
	return ChapterForm{Story: s, Action: s.ChaptersURL(), IsNew: true, CancelURL: s.URL()}
}

func editChapterForm(s Story, c Chapter) ChapterForm {
	return ChapterForm{
		Story:     s,
		Action:    c.EditURL(),
		CancelURL: s.URL(),
		Title:     c.Title,
		Content:   c.Content,
		Publish:   c.IsPublished,
	}
}

func (f ChapterForm) Input() ChapterInput {
	return ChapterInput{Title: f.Title, Content: f.Content, Publish: f.Publish}
}

// ConfirmDelete describe la página que pide confirmación antes de borrar algo.
// Es una página aparte (y no un confirm() de JavaScript) para que funcione sin JS.
type ConfirmDelete struct {
	Title     string
	Message   string
	Action    string
	CancelURL string
}

func confirmDeleteStory(s Story) ConfirmDelete {
	return ConfirmDelete{
		Title:     "Borrar historia",
		Message:   fmt.Sprintf("¿Seguro que quieres borrar «%s»? Se borrarán también sus capítulos y comentarios. No se puede deshacer.", s.Title),
		Action:    s.DeleteURL(),
		CancelURL: s.URL(),
	}
}

func confirmDeleteChapter(s Story, c Chapter) ConfirmDelete {
	return ConfirmDelete{
		Title:     "Borrar capítulo",
		Message:   fmt.Sprintf("¿Seguro que quieres borrar el capítulo «%s» de «%s»? No se puede deshacer.", c.Title, s.Title),
		Action:    c.DeleteURL(),
		CancelURL: s.URL(),
	}
}
