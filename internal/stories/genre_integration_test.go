package stories

import (
	"context"
	"errors"
	"strconv"
	"testing"

	"github.com/diegovhdev/yortale/internal/auth"
	"github.com/diegovhdev/yortale/internal/platform/dbtest"
)

// Comprueba contra PostgreSQL los nombres de las restricciones de genres y que una historia
// conserva un género desactivado.
func TestGenresIntegration(t *testing.T) {
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

	var verrs auth.ValidationErrors
	if err := svc.CreateGenre(ctx, admin, GenreInput{Name: "Fantasía"}); !errors.As(err, &verrs) || verrs["name"] == "" {
		t.Errorf("nombre repetido (genres_name_key): %v", err)
	}
	if err := svc.CreateGenre(ctx, admin, GenreInput{Name: "fantasia"}); !errors.As(err, &verrs) || verrs["name"] == "" {
		t.Errorf("nombre que solo cambia en tildes (genres_slug_key): %v", err)
	}

	must(svc.CreateGenre(ctx, admin, GenreInput{Name: "Western", Active: true}))
	var western Genre
	genres, err := svc.Genres(ctx)
	must(err)
	for _, g := range genres {
		if g.Name == "Western" {
			western = g
		}
	}
	if western.Slug != "western" || !western.Active {
		t.Fatalf("género creado inesperado: %+v", western)
	}
	westernID := []string{itoa16(western.ID)}

	in := StoryInput{Title: "En el oeste", Kind: KindOriginal, Language: "es", ContentRating: "general",
		Warnings: []string{WarningNone}, GenreIDs: westernID}
	story, err := svc.Create(ctx, author.ID, in)
	must(err)

	// Renombrar cambia el slug y se ve en la historia; desactivar impide elegirlo, pero la
	// historia lo conserva al editarla.
	must(svc.UpdateGenre(ctx, admin, western.ID, GenreInput{Name: "Wéstern", Active: false}))
	if _, err := svc.Create(ctx, author.ID, in); !errors.As(err, &verrs) || verrs[genresField] == "" {
		t.Errorf("una historia nueva no debería poder usar un género inactivo: %v", err)
	}
	in.Status = StatusDraft
	updated, err := svc.Update(ctx, author.ID, story.ID, in)
	must(err)
	if len(updated.Genres) != 1 || updated.Genres[0].Name != "Wéstern" || updated.Genres[0].Slug != "western" {
		t.Errorf("al editar, la historia debía conservar el género con el nombre nuevo: %+v", updated.Genres)
	}

	if err := svc.UpdateGenre(ctx, admin, 32000, GenreInput{Name: "Nada"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("editar un género que no existe: %v", err)
	}
}

func itoa16(n int16) string { return strconv.Itoa(int(n)) }
