package stories

import (
	"slices"
	"testing"
)

func TestSlugify(t *testing.T) {
	tests := []struct{ title, want string }{
		{"¡El Último Dragón!", "el-ultimo-dragon"},
		{"  Niño   y  pingüino  ", "nino-y-pinguino"},
		{"Harry Potter & la Piedra (fanfic)", "harry-potter-la-piedra-fanfic"},
		{"Capítulo 2: el regreso", "capitulo-2-el-regreso"},
		{"東京の夜", "東京の夜"},
		{"Ελληνικά", "ελληνικά"}, // solo se quitan los acentos de letras latinas
		{"हिन्दी कहानी", "हिन्दी-कहानी"},
		{"!!!", "historia"},
		{"Edit", "edit-historia"},
	}
	for _, tt := range tests {
		if got := Slugify(tt.title); got != tt.want {
			t.Errorf("Slugify(%q) = %q, se esperaba %q", tt.title, got, tt.want)
		}
	}
}

func TestSlugifyMaxLength(t *testing.T) {
	long := ""
	for range 30 {
		long += "palabra "
	}
	got := []rune(Slugify(long))
	if len(got) > maxSlugRunes {
		t.Fatalf("el slug tiene %d caracteres, el máximo es %d", len(got), maxSlugRunes)
	}
	if got[len(got)-1] == '-' {
		t.Fatalf("el slug no debe terminar en guion: %q", string(got))
	}
}

func TestParagraphs(t *testing.T) {
	got := paragraphs("Primero.\nsigue igual\n\n  \nSegundo.\n\n\n")
	want := []string{"Primero.\nsigue igual", "Segundo."}
	if !slices.Equal(got, want) {
		t.Fatalf("paragraphs = %q, se esperaba %q", got, want)
	}
}

func TestExcerpt(t *testing.T) {
	if got := excerpt("corto", 10); got != "corto" {
		t.Errorf("excerpt de un texto corto = %q", got)
	}
	if got := excerpt("una frase bastante larga, de verdad", 20); got != "una frase bastante…" {
		t.Errorf("excerpt = %q", got)
	}
}
