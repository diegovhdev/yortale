package stories

import (
	"fmt"
	"strconv"
)

// StoryForm guarda lo que el autor escribió y los errores, para volver a mostrar el formulario.
// El mismo formulario sirve para crear y para editar.
type StoryForm struct {
	Action        string // URL a la que se envía
	IsNew         bool   // al crear no se elige estado: la historia empieza como borrador
	CancelURL     string
	Title         string
	Synopsis      string
	GenreID       string
	Language      string
	ContentRating string
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
		Language:      s.Language,
		ContentRating: s.ContentRating,
		Status:        s.Status,
		Genres:        genres,
	}
	if s.GenreID != nil {
		f.GenreID = strconv.Itoa(int(*s.GenreID))
	}
	return f
}

func (f StoryForm) Input() StoryInput {
	return StoryInput{
		Title:         f.Title,
		Synopsis:      f.Synopsis,
		GenreID:       f.GenreID,
		Language:      f.Language,
		ContentRating: f.ContentRating,
		Status:        f.Status,
	}
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
