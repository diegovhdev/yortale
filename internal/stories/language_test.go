package stories

import (
	"context"
	"errors"
	"testing"

	"github.com/diegovhdev/yortale/internal/auth"
)

// fakeLanguages es un languageStore en memoria que cuenta cuántas veces se listó.
type fakeLanguages struct {
	langs []Language
	lists int
}

func (f *fakeLanguages) List(context.Context) ([]Language, error) {
	f.lists++
	return f.langs, nil
}

func (f *fakeLanguages) Create(_ context.Context, l Language) error {
	for _, have := range f.langs {
		switch {
		case have.Code == l.Code:
			return ErrLanguageExists
		case have.Name == l.Name:
			return ErrLanguageNameTaken
		}
	}
	f.langs = append(f.langs, l)
	return nil
}

func (f *fakeLanguages) Update(_ context.Context, l Language) error {
	for i := range f.langs {
		if f.langs[i].Code == l.Code {
			f.langs[i] = l
			return nil
		}
	}
	return ErrNotFound
}

func TestNormalizeLanguageCode(t *testing.T) {
	tests := map[string]string{"es": "es", " ES ": "es", "pt-br": "pt-BR", "PT_br": "pt-BR", "ast": "ast"}
	for in, want := range tests {
		if got := normalizeLanguageCode(in); got != want {
			t.Errorf("normalizeLanguageCode(%q) = %q, se esperaba %q", in, got, want)
		}
	}
	for _, bad := range []string{"", "e", "espa", "es-", "es-MXX", "es-419", "1a"} {
		if languageCodePattern.MatchString(normalizeLanguageCode(bad)) {
			t.Errorf("%q no debería ser un código válido", bad)
		}
	}
}

func TestLanguageOptionsKeepsInactiveCurrent(t *testing.T) {
	langs := []Language{{Code: "es", Name: "Español", Active: true}, {Code: "la", Name: "Latina"}}
	if opts := languageOptions(langs, ""); len(opts) != 1 || opts[0].Value != "es" {
		t.Errorf("sin idioma guardado se esperaba solo es, llegó %v", opts)
	}
	if opts := languageOptions(langs, "la"); len(opts) != 2 {
		t.Errorf("con la guardado se esperaban los dos, llegó %v", opts)
	}
}

func TestCreateLanguage(t *testing.T) {
	store := &fakeLanguages{langs: []Language{{Code: "es", Name: "Español", Active: true}}}
	svc := &Service{languages: store}
	admin := &auth.User{Role: auth.RoleAdmin}
	ctx := t.Context()

	if err := svc.CreateLanguage(ctx, &auth.User{Role: auth.RoleModerator}, LanguageInput{Code: "ca", Name: "Català"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("un moderador no debería poder crear idiomas: %v", err)
	}

	// Llena la caché para comprobar que crear la vacía.
	if _, err := svc.Languages(ctx); err != nil {
		t.Fatal(err)
	}
	if err := svc.CreateLanguage(ctx, admin, LanguageInput{Code: "PT_br", Name: " Português do Brasil ", Active: true}); err != nil {
		t.Fatalf("crear idioma: %v", err)
	}
	langs, _ := svc.Languages(ctx)
	if store.lists != 2 || len(langs) != 2 || langs[1] != (Language{Code: "pt-BR", Name: "Português do Brasil", Active: true}) {
		t.Errorf("tras crear, la caché debía recargarse con pt-BR normalizado: %v (listados: %d)", langs, store.lists)
	}

	tests := []struct {
		in    LanguageInput
		field string
	}{
		{LanguageInput{Code: "español", Name: "Español 2"}, "code"},
		{LanguageInput{Code: "ca", Name: "  "}, "name"},
		{LanguageInput{Code: "ES", Name: "Castellano"}, "code"}, // ya existe como es
		{LanguageInput{Code: "ca", Name: "Español"}, "name"},
	}
	for _, tt := range tests {
		var verrs auth.ValidationErrors
		if err := svc.CreateLanguage(ctx, admin, tt.in); !errors.As(err, &verrs) || verrs[tt.field] == "" {
			t.Errorf("%+v: se esperaba un error en %s, llegó %v", tt.in, tt.field, err)
		}
	}
}

func TestStoryRejectsInactiveLanguageUnlessKept(t *testing.T) {
	store := &fakeLanguages{langs: []Language{{Code: "es", Name: "Español", Active: true}, {Code: "la", Name: "Latina"}}}
	svc := &Service{languages: store}
	in := StoryInput{Title: "Una", Kind: KindOriginal, Language: "la", ContentRating: "general",
		Status: StatusDraft, Warnings: []string{WarningNone}}

	_, err := svc.validateStory(t.Context(), in, Story{})
	if verrs, _ := err.(auth.ValidationErrors); verrs["language"] == "" {
		t.Errorf("una historia nueva no debería poder usar un idioma inactivo: %v", err)
	}
	_, err = svc.validateStory(t.Context(), in, Story{Language: "la"})
	if verrs, _ := err.(auth.ValidationErrors); verrs["language"] != "" {
		t.Errorf("una historia que ya tiene el idioma inactivo debería conservarlo: %v", verrs["language"])
	}
}
