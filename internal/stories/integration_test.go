package stories

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/diegovhdev/yortale/internal/auth"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// testDB crea un esquema temporal con db/schema.sql y devuelve un pool que trabaja dentro de él.
// El esquema se borra al terminar el test. Se salta si no hay TEST_DATABASE_URL.
//
// TEST_DATABASE_URL debe ser una conexión directa (en Neon, el host sin "-pooler"): el pooler
// reparte las sentencias entre conexiones y el SET search_path no se mantendría.
func testDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL no está definida")
	}
	ctx := context.Background()

	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)

	schema := fmt.Sprintf("test_stories_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Errorf("borrar el esquema %s: %v", schema, err)
		}
	})

	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	// Neon ignora search_path como parámetro de conexión, así que se fija en cada conexión nueva.
	// public sigue en la ruta porque ahí están las extensiones (citext, pg_trgm).
	cfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		_, err := conn.Exec(ctx, "SET search_path TO "+schema+", public")
		return err
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	ddl, err := os.ReadFile("../../db/schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(ddl)); err != nil {
		t.Fatalf("aplicar schema.sql: %v", err)
	}
	return pool
}

func createUser(t *testing.T, pool *pgxpool.Pool, username string) *auth.User {
	t.Helper()
	u := &auth.User{Username: username}
	err := pool.QueryRow(context.Background(),
		`INSERT INTO users (username, email, password_hash) VALUES ($1, $2, 'x') RETURNING id`,
		username, username+"@example.com").Scan(&u.ID)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func TestStoriesIntegration(t *testing.T) {
	pool := testDB(t)
	ctx := context.Background()
	svc := NewService(NewStoryRepository(pool), NewChapterRepository(pool))
	author := createUser(t, pool, "autora")
	reader := createUser(t, pool, "lector")

	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	in := StoryInput{Title: "El Último Dragón", Synopsis: "Un dragón\r\nque vuela.", Kind: KindFanfic, Language: "es", ContentRating: "general",
		GenreIDs: []string{"3", "", "1"}} // Ciencia ficción y Fantasía; el hueco se cierra

	genreIDs := func(s Story) []int16 {
		ids := make([]int16, len(s.Genres))
		for i, g := range s.Genres {
			ids[i] = g.ID
		}
		return ids
	}

	// Crear: borrador, slug sin acentos, y sufijo si el autor repite título.
	story, err := svc.Create(ctx, author.ID, in)
	must(err)
	if story.Slug != "el-ultimo-dragon" || !story.IsDraft() || story.Kind != KindFanfic || !slices.Equal(genreIDs(story), []int16{3, 1}) || story.Synopsis != "Un dragón\nque vuela." {
		t.Fatalf("historia creada inesperada: %+v", story)
	}
	twin, err := svc.Create(ctx, author.ID, in)
	must(err)
	if twin.Slug != "el-ultimo-dragon-2" {
		t.Fatalf("slug repetido = %q", twin.Slug)
	}

	// Validación.
	var verrs auth.ValidationErrors
	_, err = svc.Create(ctx, author.ID, StoryInput{Kind: "poema", Language: "xx", ContentRating: "general", GenreIDs: []string{"999", "1", "1"}})
	if !errors.As(err, &verrs) || verrs["title"] == "" || verrs["language"] == "" ||
		verrs["kind"] == "" || verrs["genres"] == "" {
		t.Fatalf("se esperaban errores de validación, llegó %v", err)
	}
	tooMany := in
	tooMany.GenreIDs = []string{"1", "2", "3", "4"}
	if _, err := svc.Create(ctx, author.ID, tooMany); !errors.As(err, &verrs) || verrs["genres"] == "" {
		t.Fatalf("se esperaba error por más de %d géneros, llegó %v", maxGenres, err)
	}
	in.Status = "ongoing"
	if _, err := svc.Update(ctx, author.ID, story.ID, in); !errors.As(err, &verrs) || verrs["status"] == "" {
		t.Fatalf("publicar sin capítulos debería fallar, llegó %v", err)
	}

	// Los borradores no se ven ni se pueden editar desde fuera.
	if _, err := svc.Visible(ctx, reader, story.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("un lector vio un borrador: %v", err)
	}
	if _, err := svc.Update(ctx, reader.ID, story.ID, in); !errors.Is(err, ErrNotFound) {
		t.Fatalf("un lector editó una historia ajena: %v", err)
	}
	if stories, _, err := svc.Explore(ctx, ExploreFilter{}); err != nil || len(stories) != 0 {
		t.Fatalf("explorar mostró borradores: %v %v", stories, err)
	}

	// Capítulos: uno en borrador y otro publicado. Publicar saca la historia del borrador.
	_, draft, err := svc.CreateChapter(ctx, author.ID, story.ID, ChapterInput{Title: "Uno", Content: "Hola  mundo\n\nadiós"})
	must(err)
	_, published, err := svc.CreateChapter(ctx, author.ID, story.ID, ChapterInput{Title: "Dos", Content: "texto", Publish: true})
	must(err)
	if draft.Position != 1 || published.Position != 2 || draft.WordCount != 3 || published.PublishedAt == nil {
		t.Fatalf("capítulos inesperados: %+v / %+v", draft, published)
	}
	story, err = svc.Visible(ctx, reader, story.ID)
	must(err)
	if story.Status != "ongoing" || story.ChapterCount != 1 || story.LastChapterAt == nil || story.PublishedAt == nil {
		t.Fatalf("la historia no se publicó al publicar un capítulo: %+v", story)
	}

	// Explorar: búsqueda de texto completo y por parecido del título, y filtro por género.
	for _, q := range []string{"dragón", "dragon", "dragones", "Ultimo Dragon", "ultmo dragón"} {
		found, _, err := svc.Explore(ctx, ExploreFilter{Query: q})
		must(err)
		if len(found) != 1 || found[0].ID != story.ID {
			t.Errorf("buscar %q encontró %d historias", q, len(found))
		}
	}
	other := int16(2)
	if found, _, err := svc.Explore(ctx, ExploreFilter{GenreID: &other}); err != nil || len(found) != 0 {
		t.Fatalf("el filtro de género no filtró: %v %v", found, err)
	}
	second := int16(1) // el segundo género también cuenta para el filtro
	if found, _, err := svc.Explore(ctx, ExploreFilter{GenreID: &second}); err != nil || len(found) != 1 {
		t.Fatalf("el filtro por el segundo género no encontró la historia: %v %v", found, err)
	}

	if found, _, err := svc.Explore(ctx, ExploreFilter{Kind: KindOriginal}); err != nil || len(found) != 0 {
		t.Fatalf("el filtro de tipo no filtró: %v %v", found, err)
	}
	if found, _, err := svc.Explore(ctx, ExploreFilter{Kind: KindFanfic, GenreID: &second}); err != nil || len(found) != 1 {
		t.Fatalf("el filtro de tipo y género no encontró la historia: %v %v", found, err)
	}

	// Leer: el lector no ve el borrador; leer cuenta una sola vez por lector.
	if _, err := svc.ReadChapter(ctx, reader, story.ID, draft.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("un lector leyó un capítulo sin publicar: %v", err)
	}
	if _, err := svc.ReadChapter(ctx, reader, twin.ID, published.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("se leyó un capítulo con el id de otra historia: %v", err)
	}
	for range 2 {
		_, err := svc.ReadChapter(ctx, reader, story.ID, published.ID)
		must(err)
	}
	story, err = svc.Visible(ctx, reader, story.ID)
	must(err)
	if story.ReadsCount != 1 {
		t.Fatalf("reads_count = %d, se esperaba 1", story.ReadsCount)
	}

	// Reordenar (restricción DEFERRABLE) y vecinos.
	_, err = svc.MoveChapter(ctx, author.ID, published.ID, true)
	must(err)
	_, err = svc.MoveChapter(ctx, author.ID, published.ID, true) // ya es el primero: no hace nada
	must(err)
	view, err := svc.ReadChapter(ctx, author, story.ID, published.ID)
	must(err)
	if view.Chapter.Position != 1 || view.Prev != nil || view.Next == nil || view.Next.ID != draft.ID {
		t.Fatalf("orden inesperado tras mover: %+v", view)
	}

	// Editar y publicar el borrador; borrar el primero cierra el hueco de posiciones.
	_, draft, err = svc.UpdateChapter(ctx, author.ID, draft.ID, ChapterInput{Title: "Uno bis", Content: "nuevo", Publish: true})
	must(err)
	if !draft.IsPublished || draft.PublishedAt == nil || draft.Title != "Uno bis" {
		t.Fatalf("capítulo editado inesperado: %+v", draft)
	}
	_, err = svc.DeleteChapter(ctx, author.ID, published.ID)
	must(err)
	page, err := svc.StoryPage(ctx, reader, story.ID)
	must(err)
	if len(page.Chapters) != 1 || page.Chapters[0].Position != 1 {
		t.Fatalf("capítulos tras borrar: %+v", page.Chapters)
	}

	// Cambiar el título cambia el slug; mantenerlo no. Los géneros se reemplazan.
	in.Title = "Otro título"
	in.Kind = KindOriginal
	in.GenreIDs = []string{"2", "3", "1"} // el orden elegido es el que se guarda
	story, err = svc.Update(ctx, author.ID, story.ID, in)
	must(err)
	if story.Slug != "otro-titulo" || story.Kind != KindOriginal || story.Status != "ongoing" || !slices.Equal(genreIDs(story), []int16{2, 3, 1}) {
		t.Fatalf("historia editada inesperada: %+v", story)
	}

	// Borrar: solo el autor.
	if err := svc.Delete(ctx, reader.ID, story.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("un lector borró una historia ajena: %v", err)
	}
	must(svc.Delete(ctx, author.ID, story.ID))
	if mine, err := svc.ByAuthor(ctx, author.ID); err != nil || len(mine) != 1 {
		t.Fatalf("historias del autor tras borrar: %v %v", mine, err)
	}
}
