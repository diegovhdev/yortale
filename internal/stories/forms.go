package stories

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/diegovhdev/yortale/web/components"
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
	Languages     []Language
	keepLanguage  string            // el idioma guardado de la historia; se ofrece aunque esté inactivo
	Errors        map[string]string // error por campo, por ejemplo Errors["title"]
	General       string            // error que no pertenece a un campo concreto
}

// storyChoices son las listas completas de la base de datos (con los géneros e idiomas
// inactivos); el formulario deja elegir solo los activos y los que la historia ya tiene.
type storyChoices struct {
	Genres    []Genre
	Languages []Language
}

func newStoryForm(c storyChoices) StoryForm {
	return StoryForm{
		Action:        "/stories",
		IsNew:         true,
		CancelURL:     "/my/stories",
		Kind:          KindOriginal,
		Language:      defaultLanguage,
		ContentRating: "general",
		Status:        StatusDraft,
		Genres:        selectableGenres(c.Genres, nil),
		Languages:     c.Languages,
	}
}

func editStoryForm(s Story, c storyChoices) StoryForm {
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
		Genres:        selectableGenres(c.Genres, genreIDs(s.Genres)),
		Languages:     c.Languages,
		keepLanguage:  s.Language,
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

// LanguageOptions son los idiomas del <select>: los activos y el que la historia ya tenía.
func (f StoryForm) LanguageOptions() []components.Option {
	return languageOptions(f.Languages, f.keepLanguage)
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
	// DraftNotice explica por qué el capítulo se acaba de guardar como borrador aunque el
	// autor pidió publicarlo (publishBlocker); vacío si no es el caso.
	DraftNotice string
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

// LanguageForm es el formulario de idioma de /admin/languages, para agregar uno (IsNew) o
// editarlo. Al editar no se muestra el código, que no cambia.
type LanguageForm struct {
	Action  string
	IsNew   bool
	Code    string
	Name    string
	Active  bool
	Errors  map[string]string
	General string
}

const languagesURL = "/admin/languages"

func newLanguageForm() LanguageForm {
	return LanguageForm{Action: languagesURL, IsNew: true, Active: true}
}

func editLanguageForm(l Language) LanguageForm {
	return LanguageForm{Action: l.EditURL(), Code: l.Code, Name: l.Name, Active: l.Active}
}

func (f LanguageForm) Input() LanguageInput {
	return LanguageInput{Code: f.Code, Name: f.Name, Active: f.Active}
}

// GenreForm es el formulario de género de /admin/genres, para agregar uno (IsNew) o editarlo.
type GenreForm struct {
	Action  string
	IsNew   bool
	Name    string
	Active  bool
	Errors  map[string]string
	General string
}

const genresURL = "/admin/genres"

func newGenreForm() GenreForm {
	return GenreForm{Action: genresURL, IsNew: true, Active: true}
}

func editGenreForm(g Genre) GenreForm {
	return GenreForm{Action: g.EditURL(), Name: g.Name, Active: g.Active}
}

func (f GenreForm) Input() GenreInput {
	return GenreInput{Name: f.Name, Active: f.Active}
}
