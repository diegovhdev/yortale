package stories

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

type Chapter struct {
	ID          int64
	StoryID     int64
	Position    int
	Title       string
	Content     string // vacío en los listados, que no lo cargan
	WordCount   int
	IsPublished bool
	PublishedAt *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// URL es la dirección de lectura del capítulo, dentro de la URL canónica de su historia.
func (c Chapter) URL(s Story) string {
	return fmt.Sprintf("%s/chapters/%d", s.URL(), c.ID)
}

func (c Chapter) EditURL() string   { return fmt.Sprintf("/chapters/%d/edit", c.ID) }
func (c Chapter) DeleteURL() string { return fmt.Sprintf("/chapters/%d/delete", c.ID) }
func (c Chapter) MoveURL() string   { return fmt.Sprintf("/chapters/%d/move", c.ID) }

// Paragraphs divide el texto del capítulo en párrafos separados por líneas en blanco.
func (c Chapter) Paragraphs() []string { return paragraphs(c.Content) }

var blankLines = regexp.MustCompile(`\n[ \t]*\n`)

// paragraphs se usa para mostrar texto plano: cada párrafo va en su propio <p>,
// y los saltos de línea sueltos se respetan con CSS (white-space: pre-line).
func paragraphs(text string) []string {
	var out []string
	for _, p := range blankLines.Split(text, -1) {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func countWords(text string) int { return len(strings.Fields(text)) }
