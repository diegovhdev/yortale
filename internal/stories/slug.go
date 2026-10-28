package stories

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

const maxSlugRunes = 80

// reservedSlugs son palabras que chocan con rutas como /stories/{id}/edit.
var reservedSlugs = map[string]bool{"edit": true, "delete": true}

// Slugify convierte un título en la parte legible de la URL: "¡El Último Dragón!" → "el-ultimo-dragon".
// Se quitan los acentos de las letras latinas, pero se conservan las letras de otros alfabetos
// (la plataforma es para cualquier idioma, y las URL admiten Unicode).
func Slugify(title string) string {
	s := slugify(title)
	switch {
	case s == "":
		return "historia"
	case reservedSlugs[s]:
		return s + "-historia"
	}
	return s
}

// TagSlug es la identidad de un tag: dos tags son el mismo si dan el mismo slug, así que
// "Época Victoriana" y "epoca-victoriana" son uno solo. Devuelve "" si el nombre no tiene
// letras ni números.
func TagSlug(name string) string {
	return slugify(name)
}

// slugify es la base de Slugify y TagSlug: minúsculas, sin acentos latinos y con cualquier
// otra cosa que no sea letra o número convertida en un solo guion.
func slugify(text string) string {
	var b strings.Builder
	var base rune // última letra escrita, para saber a qué alfabeto pertenece una marca
	dash := false
	for _, r := range norm.NFD.String(strings.ToLower(text)) {
		switch {
		case unicode.IsMark(r):
			// NFD separa "é" en "e" + acento. En letras latinas el acento se descarta;
			// en otros alfabetos (devanagari, por ejemplo) la marca es parte de la letra.
			if base != 0 && !unicode.Is(unicode.Latin, base) {
				b.WriteRune(r)
			}
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if dash && b.Len() > 0 {
				b.WriteByte('-')
			}
			dash = false
			b.WriteRune(r)
			base = r
		default:
			dash = true
			base = 0
		}
	}

	s := norm.NFC.String(b.String())
	if runes := []rune(s); len(runes) > maxSlugRunes {
		s = strings.TrimRight(string(runes[:maxSlugRunes]), "-")
	}
	return s
}
