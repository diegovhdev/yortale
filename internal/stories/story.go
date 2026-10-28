// Package stories contiene las historias y sus capítulos: escribir, publicar, explorar y leer.
package stories

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/diegovhdev/yortale/internal/auth"

	"github.com/diegovhdev/yortale/web/components"
)

// Story es una historia tal como la muestra la aplicación, con los datos del autor y de los géneros ya unidos.
type Story struct {
	ID             int64
	AuthorID       int64
	AuthorUsername string
	AuthorName     string  // display_name, o el username si no tiene
	Kind           string  // original o fanfic
	Genres         []Genre // hasta maxGenres, en el orden en que el autor quiere mostrarlos
	Tags           []Tag   // hasta maxTags, en el orden que eligió el autor
	Title          string
	Slug           string
	Synopsis       string
	Language       string
	ContentRating  string
	Warnings       []string // valores de ContentWarnings, en su orden; al menos uno
	Status         string
	PublishedAt    *time.Time // primera vez que dejó de ser borrador
	LastChapterAt  *time.Time
	ReadsCount     int64
	ChapterCount   int // solo capítulos publicados
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type Genre struct {
	ID   int16
	Name string
	Slug string
}

// Tag es una etiqueta libre. Slug es su identidad (ver TagSlug); Name, cómo se muestra.
type Tag struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
	Uses int64  `json:"uses"` // historias publicadas con el tag; solo en sugerencias y en su página
}

const StatusDraft = "draft"

const (
	KindOriginal = "original"
	KindFanfic   = "fanfic"
)

const (
	WarningNone     = "none"     // no aplica ninguna advertencia; va sola
	WarningDeclined = "declined" // el autor prefiere no advertir; se puede combinar con otras
)

// ContentWarning es una advertencia de contenido, con la explicación que ve el autor al elegirla.
type ContentWarning struct {
	Value       string
	Label       string
	Description string
}

// maxGenres debe coincidir con el CHECK (position BETWEEN 1 AND 3) de story_genres.
const maxGenres = 3

const (
	maxTags     = 20 // CHECK (position BETWEEN 1 AND 20) de story_tags
	maxTagRunes = 50 // CHECK (char_length(name) BETWEEN 1 AND 50) de tags
	// maxCardTags es cuántos tags se ven en la tarjeta de un listado; el resto, en la historia.
	maxCardTags = 6
)

// Las listas siguientes deben coincidir con los CHECK de stories.kind, stories.status y stories.content_rating.
var (
	Kinds = []components.Option{
		{Value: KindOriginal, Label: "Original"},
		{Value: KindFanfic, Label: "Fanfic"},
	}
	Statuses = []components.Option{
		{Value: StatusDraft, Label: "Borrador (solo tú la ves)"},
		{Value: "ongoing", Label: "En curso"},
		{Value: "completed", Label: "Completada"},
		{Value: "hiatus", Label: "En pausa"},
	}
	ContentRatings = []components.Option{
		{Value: "general", Label: "Para todo público"},
		{Value: "teen", Label: "Adolescentes (13+)"},
		{Value: "mature", Label: "Adultos (18+)"},
	}
	// Debe coincidir con el CHECK de stories.warnings. El orden es el del formulario y el
	// de cómo se muestran.
	ContentWarnings = []ContentWarning{
		{Value: "graphic_violence", Label: "Violencia gráfica",
			Description: "Descripciones explícitas de violencia, heridas o tortura."},
		{Value: "major_character_death", Label: "Muerte de un personaje principal",
			Description: "Muere un protagonista o un personaje central de la historia."},
		{Value: "sexual_violence", Label: "Violencia sexual",
			Description: "Violación, agresión o abuso sexual, o cualquier contenido sexual sin consentimiento."},
		{Value: WarningDeclined, Label: "El autor prefiere no advertir",
			Description: "Puede contener algo de lo anterior sin avisarlo, por ejemplo para no revelar la trama. Se puede combinar con otras."},
		{Value: WarningNone, Label: "No aplica ninguna",
			Description: "La historia no contiene nada de lo anterior. No se puede combinar con otras."},
	}
	// La columna language no tiene CHECK; esta lista es la única validación.
	Languages = []components.Option{
		{Value: "es", Label: "Español"},
		{Value: "en", Label: "English"},
		{Value: "pt", Label: "Português"},
		{Value: "fr", Label: "Français"},
		{Value: "it", Label: "Italiano"},
		{Value: "de", Label: "Deutsch"},
	}
)

// URL es la dirección canónica: /stories/{id}/{slug}. Solo el id se usa para buscar.
func (s Story) URL() string {
	return fmt.Sprintf("/stories/%d/%s", s.ID, url.PathEscape(s.Slug))
}

func (s Story) EditURL() string       { return fmt.Sprintf("/stories/%d/edit", s.ID) }
func (s Story) DeleteURL() string     { return fmt.Sprintf("/stories/%d/delete", s.ID) }
func (s Story) NewChapterURL() string { return fmt.Sprintf("/stories/%d/chapters/new", s.ID) }
func (s Story) ChaptersURL() string   { return fmt.Sprintf("/stories/%d/chapters", s.ID) }

func (s Story) IsDraft() bool { return s.Status == StatusDraft }

func (s Story) AuthorURL() string { return auth.ProfileURL(s.AuthorUsername) }

// KindURL y GenreURL llevan a explorar ya filtrado por ese tipo o género.
func (s Story) KindURL() string { return "/stories?kind=" + url.QueryEscape(s.Kind) }
func (g Genre) URL() string     { return fmt.Sprintf("/stories?genre=%d", g.ID) }

// URL es la página del tag, con las historias que lo usan.
func (t Tag) URL() string { return "/tags/" + url.PathEscape(t.Slug) }

// CardTags son los tags que caben en una tarjeta; HiddenTagCount, cuántos quedan fuera.
func (s Story) CardTags() []Tag {
	return s.Tags[:min(len(s.Tags), maxCardTags)]
}

func (s Story) HiddenTagCount() int {
	return max(len(s.Tags)-maxCardTags, 0)
}

func (s Story) KindLabel() string          { return optionLabel(Kinds, s.Kind) }
func (s Story) StatusLabel() string        { return optionLabel(Statuses, s.Status) }
func (s Story) ContentRatingLabel() string { return optionLabel(ContentRatings, s.ContentRating) }
func (s Story) LanguageLabel() string      { return optionLabel(Languages, s.Language) }

// WarningLabels son las advertencias de la historia, en texto.
func (s Story) WarningLabels() []string {
	labels := make([]string, 0, len(s.Warnings))
	for _, v := range s.Warnings {
		labels = append(labels, warningLabel(v))
	}
	return labels
}

func warningLabel(value string) string {
	for _, w := range ContentWarnings {
		if w.Value == value {
			return w.Label
		}
	}
	return value
}

func validWarning(value string) bool {
	for _, w := range ContentWarnings {
		if w.Value == value {
			return true
		}
	}
	return false
}

// excludableWarnings son las que el lector puede ocultar en explorar: todas menos «no aplica
// ninguna», que no avisa de nada.
func excludableWarnings() []ContentWarning {
	var ws []ContentWarning
	for _, w := range ContentWarnings {
		if w.Value != WarningNone {
			ws = append(ws, w)
		}
	}
	return ws
}

func optionLabel(opts []components.Option, value string) string {
	for _, o := range opts {
		if o.Value == value {
			return o.Label
		}
	}
	return value
}

func validOption(opts []components.Option, value string) bool {
	for _, o := range opts {
		if o.Value == value {
			return true
		}
	}
	return false
}

// kindOptions son los tipos con una opción vacía delante, para el filtro de explorar.
func kindOptions(emptyLabel string) []components.Option {
	return append([]components.Option{{Value: "", Label: emptyLabel}}, Kinds...)
}

var months = [...]string{"enero", "febrero", "marzo", "abril", "mayo", "junio", "julio",
	"agosto", "septiembre", "octubre", "noviembre", "diciembre"}

// formatDate escribe una fecha como "2 de octubre de 2026".
func formatDate(t time.Time) string {
	return fmt.Sprintf("%d de %s de %d", t.Day(), months[t.Month()-1], t.Year())
}

// plural escribe "1 capítulo" o "3 capítulos".
func plural(n int64, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

// excerpt recorta un texto a n caracteres como mucho, sin cortar palabras a la mitad.
func excerpt(text string, n int) string {
	runes := []rune(text)
	if len(runes) <= n {
		return text
	}
	cut := string(runes[:n])
	if i := strings.LastIndexAny(cut, " \n"); i > 0 {
		cut = cut[:i]
	}
	return strings.TrimRight(cut, " ,.;:\n") + "…"
}
