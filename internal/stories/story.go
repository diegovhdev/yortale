// Package stories contiene las historias y sus capítulos: escribir, publicar, explorar y leer.
package stories

import (
	"fmt"
	"net/url"
	"regexp"
	"slices"
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
	Language       string // code de languages
	LanguageName   string
	ContentRating  string
	Warnings       []string // valores de ContentWarnings, en su orden; al menos uno
	Status         string
	PublishedAt    *time.Time // primera vez que dejó de ser borrador
	LastChapterAt  *time.Time
	ReadsCount     int64
	ChapterCount   int // solo capítulos publicados
	// HiddenAt no es nil si la moderación la ocultó: entonces solo la ven el autor y el equipo.
	HiddenAt     *time.Time
	HiddenReason string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Genre es un género. Active solo se carga en la lista completa (Service.Genres); en los
// géneros de una historia queda en false.
type Genre struct {
	ID     int16
	Name   string
	Slug   string
	Active bool
}

// maxGenreNameRunes debe coincidir con el CHECK de genres.name.
const maxGenreNameRunes = 50

func (g Genre) EditURL() string { return fmt.Sprintf("/admin/genres/%d/edit", g.ID) }

// selectableGenres son los géneros que se pueden marcar: los activos, más los de keep aunque
// estén inactivos (los que la historia ya tiene, o los que ya están en el filtro de explorar).
func selectableGenres(genres []Genre, keep []int16) []Genre {
	var out []Genre
	for _, g := range genres {
		if g.Active || slices.Contains(keep, g.ID) {
			out = append(out, g)
		}
	}
	return out
}

func genreIDs(genres []Genre) []int16 {
	ids := make([]int16, len(genres))
	for i, g := range genres {
		ids[i] = g.ID
	}
	return ids
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
)

// Language es un idioma de la tabla languages. Code no cambia nunca (es la clave a la que
// apunta stories.language); Name es el endónimo. Un idioma inactivo no se ofrece para
// historias nuevas, pero las que ya lo tienen lo conservan.
type Language struct {
	Code   string
	Name   string
	Active bool
}

// defaultLanguage es el idioma que el formulario propone al crear una historia, el mismo que
// el DEFAULT de stories.language.
const defaultLanguage = "es"

// languageCodePattern y maxLanguageNameRunes deben coincidir con los CHECK de languages:
// ISO 639 (2 o 3 letras minúsculas) con una región opcional de 2 mayúsculas (pt-BR).
var languageCodePattern = regexp.MustCompile(`^[a-z]{2,3}(-[A-Z]{2})?$`)

const maxLanguageNameRunes = 50

func (l Language) EditURL() string { return "/admin/languages/" + url.PathEscape(l.Code) + "/edit" }

// languageOptions son los idiomas que se pueden elegir en el formulario: los activos, más
// keep (el idioma que la historia ya tiene) aunque esté inactivo, para no cambiárselo al editar.
func languageOptions(langs []Language, keep string) []components.Option {
	var opts []components.Option
	for _, l := range langs {
		if l.Active || l.Code == keep {
			opts = append(opts, components.Option{Value: l.Code, Label: l.Name})
		}
	}
	return opts
}

// URL es la dirección canónica: /stories/{id}/{slug}. Solo el id se usa para buscar.
func (s Story) URL() string {
	return fmt.Sprintf("/stories/%d/%s", s.ID, url.PathEscape(s.Slug))
}

func (s Story) EditURL() string       { return fmt.Sprintf("/stories/%d/edit", s.ID) }
func (s Story) DeleteURL() string     { return fmt.Sprintf("/stories/%d/delete", s.ID) }
func (s Story) NewChapterURL() string { return fmt.Sprintf("/stories/%d/chapters/new", s.ID) }
func (s Story) ChaptersURL() string   { return fmt.Sprintf("/stories/%d/chapters", s.ID) }

func (s Story) IsDraft() bool { return s.Status == StatusDraft }

func (s Story) IsHidden() bool { return s.HiddenAt != nil }

// hiddenReasonSuffix es ": motivo" si la moderación dio uno, para la tarjeta de «Mis historias».
func hiddenReasonSuffix(s Story) string {
	if s.HiddenReason == "" {
		return "."
	}
	return ": " + s.HiddenReason
}

// ReportURL y ModerationURL llevan a las páginas del paquete moderation.
func (s Story) ReportURL() string     { return fmt.Sprintf("/reports/new?story=%d", s.ID) }
func (s Story) ModerationURL() string { return fmt.Sprintf("/moderation/stories/%d", s.ID) }

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
func (s Story) LanguageLabel() string      { return s.LanguageName }

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

// UpdatedLater dice si la historia tiene un capítulo publicado otro día que su primera
// publicación, para mostrar «actualizada el …» solo cuando aporta algo.
func (s Story) UpdatedLater() bool {
	if s.LastChapterAt == nil {
		return false
	}
	if s.PublishedAt == nil {
		return true
	}
	return formatDate(*s.LastChapterAt) != formatDate(*s.PublishedAt)
}

// Periodos de los filtros de fecha de explorar ("actualizada en…", "publicada en…"). Se ofrecen
// periodos fijos en lugar de un calendario porque es lo que un lector suele buscar ("lo nuevo
// de esta semana") y se elige en un clic. periodStart traduce cada valor a una fecha.
const (
	PeriodWeek    = "week"
	PeriodMonth   = "month"
	Period3Months = "3months"
	PeriodYear    = "year"
)

// DatePeriods son las opciones de los filtros de fecha; "" no filtra.
var DatePeriods = []components.Option{
	{Value: "", Label: "Cualquier fecha"},
	{Value: PeriodWeek, Label: "Última semana"},
	{Value: PeriodMonth, Label: "Último mes"},
	{Value: Period3Months, Label: "Últimos 3 meses"},
	{Value: PeriodYear, Label: "Último año"},
}

// periodStart es el comienzo del periodo contado hacia atrás desde now. Para "" o un valor
// desconocido devuelve la fecha cero, que no filtra.
func periodStart(period string, now time.Time) time.Time {
	switch period {
	case PeriodWeek:
		return now.AddDate(0, 0, -7)
	case PeriodMonth:
		return now.AddDate(0, -1, 0)
	case Period3Months:
		return now.AddDate(0, -3, 0)
	case PeriodYear:
		return now.AddDate(-1, 0, 0)
	}
	return time.Time{}
}

// Órdenes de explorar. SortRelevance ("") ordena por parecido con la búsqueda y, sin texto
// que buscar, por las actualizadas más recientemente.
const (
	SortRelevance = ""
	SortUpdated   = "updated"
	SortNewest    = "newest"
	SortOldest    = "oldest"
)

var ExploreSorts = []components.Option{
	{Value: SortRelevance, Label: "Más relevantes"},
	{Value: SortUpdated, Label: "Actualizadas recientemente"},
	{Value: SortNewest, Label: "Publicadas recientemente"},
	{Value: SortOldest, Label: "Publicadas hace más tiempo"},
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
