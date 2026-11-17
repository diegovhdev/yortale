package stories

import (
	"context"
	"errors"
	"testing"

	"github.com/diegovhdev/yortale/internal/auth"
)

// fakeGenres cumple storyStore solo en lo que toca a los géneros; el resto entra en pánico
// porque la interfaz embebida es nil.
type fakeGenres struct {
	storyStore
	genres []Genre
	lists  int
}

func (f *fakeGenres) ListGenres(context.Context) ([]Genre, error) {
	f.lists++
	return f.genres, nil
}

func (f *fakeGenres) CreateGenre(_ context.Context, g Genre) error {
	for _, have := range f.genres {
		if have.Slug == g.Slug {
			return ErrGenreNameTaken
		}
	}
	g.ID = int16(len(f.genres) + 1)
	f.genres = append(f.genres, g)
	return nil
}

func (f *fakeGenres) UpdateGenre(_ context.Context, g Genre) error {
	for i := range f.genres {
		if f.genres[i].ID == g.ID {
			f.genres[i] = g
			return nil
		}
	}
	return ErrNotFound
}

func TestCreateGenre(t *testing.T) {
	store := &fakeGenres{genres: []Genre{{ID: 1, Name: "Ciencia ficción", Slug: "ciencia-ficcion", Active: true}}}
	svc := &Service{stories: store}
	admin := &auth.User{Role: auth.RoleAdmin}
	ctx := t.Context()

	if err := svc.CreateGenre(ctx, &auth.User{Role: auth.RoleModerator}, GenreInput{Name: "Western"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("un moderador no debería poder crear géneros: %v", err)
	}
	if _, err := svc.Genres(ctx); err != nil { // llena la caché para ver que crear la vacía
		t.Fatal(err)
	}
	if err := svc.CreateGenre(ctx, admin, GenreInput{Name: " Western ", Active: true}); err != nil {
		t.Fatalf("crear género: %v", err)
	}
	genres, _ := svc.Genres(ctx)
	if store.lists != 2 || len(genres) != 2 || genres[1] != (Genre{ID: 2, Name: "Western", Slug: "western", Active: true}) {
		t.Errorf("tras crear, la caché debía recargarse con Western: %v (listados: %d)", genres, store.lists)
	}

	for _, name := range []string{"  ", "¡¿?!", "CIENCIA FICCION"} {
		var verrs auth.ValidationErrors
		if err := svc.CreateGenre(ctx, admin, GenreInput{Name: name}); !errors.As(err, &verrs) || verrs["name"] == "" {
			t.Errorf("%q: se esperaba un error en name, llegó %v", name, err)
		}
	}
}

func TestStoryKeepsInactiveGenres(t *testing.T) {
	store := &fakeGenres{genres: []Genre{{ID: 1, Name: "Fantasía", Active: true}, {ID: 2, Name: "Western"}}}
	svc := &Service{stories: store, languages: &fakeLanguages{langs: []Language{{Code: "es", Name: "Español", Active: true}}}}
	in := StoryInput{Title: "Una", Kind: KindOriginal, Language: "es", ContentRating: "general",
		Status: StatusDraft, Warnings: []string{WarningNone}, GenreIDs: []string{"1", "2"}}

	_, err := svc.validateStory(t.Context(), in, Story{})
	if verrs, _ := err.(auth.ValidationErrors); verrs[genresField] == "" {
		t.Errorf("una historia nueva no debería poder usar un género inactivo: %v", err)
	}
	p, err := svc.validateStory(t.Context(), in, Story{Language: "es", Genres: []Genre{{ID: 2}}})
	if err != nil || len(p.GenreIDs) != 2 {
		t.Errorf("una historia que ya tiene el género inactivo debería conservarlo: %v %v", p.GenreIDs, err)
	}
	if got := selectableGenres(store.genres, []int16{2}); len(got) != 2 {
		t.Errorf("selectableGenres con keep debía incluir el inactivo: %v", got)
	}
	if got := selectableGenres(store.genres, nil); len(got) != 1 {
		t.Errorf("selectableGenres sin keep debía dejar solo los activos: %v", got)
	}
}
