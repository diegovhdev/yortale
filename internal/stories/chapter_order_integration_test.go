package stories

import (
	"context"
	"errors"
	"testing"

	"github.com/diegovhdev/yortale/internal/auth"
	"github.com/diegovhdev/yortale/internal/platform/dbtest"
)

// Orden de los capítulos: los publicados van siempre antes que los borradores. Publicar
// coloca el capítulo justo detrás del último publicado, mover no cruza los dos grupos y solo
// el último publicado puede volver a borrador.
func TestChapterOrderIntegration(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	users := auth.NewService(auth.NewUserRepository(pool), auth.NewSessionRepository(pool))
	svc := NewService(NewStoryRepository(pool), NewChapterRepository(pool), NewLanguageRepository(pool), users)
	author := createUser(t, pool, "autora")
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}

	story, err := svc.Create(ctx, author.ID, StoryInput{Title: "Orden", Synopsis: testSynopsis, Kind: KindOriginal,
		Language: "es", ContentRating: "general", Warnings: []string{WarningNone}})
	must(err)
	ids := map[string]int64{}
	for _, title := range []string{"A", "B", "C"} {
		_, c, err := svc.CreateChapter(ctx, author.ID, story.ID, ChapterInput{Title: title, Content: longText})
		must(err)
		ids[title] = c.ID
	}
	// order devuelve los títulos en orden, con «*» detrás de los publicados.
	order := func() string {
		t.Helper()
		page, err := svc.StoryPage(ctx, author, story.ID)
		must(err)
		s := ""
		for i, c := range page.Chapters {
			if c.Position != i+1 {
				t.Fatalf("posiciones con huecos: %+v", page.Chapters)
			}
			s += c.Title
			if c.IsPublished {
				s += "*"
			}
		}
		return s
	}
	expect := func(step, want string) {
		t.Helper()
		if got := order(); got != want {
			t.Fatalf("%s: orden %q, se esperaba %q", step, got, want)
		}
	}

	_, _, err = svc.PublishChapter(ctx, author.ID, ids["C"])
	must(err)
	expect("publicar el último borrador", "C*AB")

	_, d, err := svc.CreateChapter(ctx, author.ID, story.ID, ChapterInput{Title: "D", Content: longText, Publish: true})
	must(err)
	ids["D"] = d.ID
	expect("crear un capítulo publicado", "C*D*AB")

	if _, err := svc.MoveChapter(ctx, author.ID, ids["D"], false); !errors.Is(err, ErrMixedMove) {
		t.Fatalf("bajar un publicado entre los borradores: %v", err)
	}
	if _, err := svc.MoveChapter(ctx, author.ID, ids["A"], true); !errors.Is(err, ErrMixedMove) {
		t.Fatalf("subir un borrador entre los publicados: %v", err)
	}
	_, err = svc.MoveChapter(ctx, author.ID, ids["D"], true)
	must(err)
	_, err = svc.MoveChapter(ctx, author.ID, ids["B"], true)
	must(err)
	expect("mover dentro de cada grupo", "D*C*BA")

	// Despublicar: solo el último publicado, que pasa a ser el primer borrador.
	var verrs auth.ValidationErrors
	_, _, err = svc.UpdateChapter(ctx, author.ID, ids["D"], ChapterInput{Title: "D", Content: longText})
	if !errors.As(err, &verrs) || verrs["publish"] == "" {
		t.Fatalf("despublicar un capítulo que no es el último publicado: %v", err)
	}
	_, _, err = svc.UpdateChapter(ctx, author.ID, ids["C"], ChapterInput{Title: "C", Content: longText})
	must(err)
	expect("despublicar el último publicado", "D*CBA")

	// Datos anteriores a la regla: un publicado detrás de un borrador. Publicar el borrador
	// lo coloca detrás de él.
	_, err = pool.Exec(ctx, `UPDATE chapters SET is_published = (id = $1) WHERE story_id = $2`, ids["B"], story.ID)
	must(err)
	expect("estado heredado", "DCB*A")
	_, _, err = svc.PublishChapter(ctx, author.ID, ids["D"])
	must(err)
	expect("publicar un borrador anterior al último publicado", "CB*D*A")
	_, err = svc.DeleteChapter(ctx, author.ID, ids["C"])
	must(err)
	expect("borrar", "B*D*A")
}
