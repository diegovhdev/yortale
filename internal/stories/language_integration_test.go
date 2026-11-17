package stories

import (
	"context"
	"errors"
	"testing"

	"github.com/diegovhdev/yortale/internal/auth"
	"github.com/diegovhdev/yortale/internal/platform/dbtest"
)

// Comprueba contra PostgreSQL lo que el store falso de language_test.go no puede: los nombres
// de las restricciones de languages, la FK de stories.language y el nombre que llega con la historia.
func TestLanguagesIntegration(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	users := auth.NewService(auth.NewUserRepository(pool), auth.NewSessionRepository(pool))
	svc := NewService(NewStoryRepository(pool), NewChapterRepository(pool), NewLanguageRepository(pool), users)
	author := createUser(t, pool, "autora")
	admin := &auth.User{Role: auth.RoleAdmin}

	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}

	// Duplicados: el código choca con languages_pkey y el nombre con languages_name_key.
	var verrs auth.ValidationErrors
	if err := svc.CreateLanguage(ctx, admin, LanguageInput{Code: "es", Name: "Castellano"}); !errors.As(err, &verrs) || verrs["code"] == "" {
		t.Errorf("código repetido: %v", err)
	}
	if err := svc.CreateLanguage(ctx, admin, LanguageInput{Code: "ca", Name: "English"}); !errors.As(err, &verrs) || verrs["name"] == "" {
		t.Errorf("nombre repetido: %v", err)
	}

	must(svc.CreateLanguage(ctx, admin, LanguageInput{Code: "ca", Name: "Catala", Active: true}))
	in := StoryInput{Title: "Una història", Kind: KindOriginal, Language: "ca", ContentRating: "general",
		Warnings: []string{WarningNone}}
	story, err := svc.Create(ctx, author.ID, in)
	must(err)
	if story.LanguageLabel() != "Catala" {
		t.Errorf("LanguageLabel = %q", story.LanguageLabel())
	}

	// Renombrar se ve en la historia; desactivar impide elegirlo, pero la historia lo conserva.
	must(svc.UpdateLanguage(ctx, admin, "ca", LanguageInput{Name: "Català", Active: false}))
	if _, err := svc.Create(ctx, author.ID, in); !errors.As(err, &verrs) || verrs["language"] == "" {
		t.Errorf("una historia nueva no debería poder usar un idioma inactivo: %v", err)
	}
	in.Status = StatusDraft
	updated, err := svc.Update(ctx, author.ID, story.ID, in)
	must(err)
	if updated.Language != "ca" || updated.LanguageLabel() != "Català" {
		t.Errorf("al editar, la historia debía conservar ca con el nombre nuevo: %q %q", updated.Language, updated.LanguageLabel())
	}

	if err := svc.UpdateLanguage(ctx, admin, "xx", LanguageInput{Name: "Nada"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("editar un idioma que no existe: %v", err)
	}

	// La FK rechaza un idioma que no está en la tabla aunque alguien salte la validación de Go.
	if _, err := pool.Exec(ctx, `UPDATE stories SET language = 'xx' WHERE id = $1`, story.ID); err == nil {
		t.Error("la FK debía rechazar un idioma desconocido")
	}
}
