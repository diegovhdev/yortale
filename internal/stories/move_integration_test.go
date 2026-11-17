package stories

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/diegovhdev/yortale/internal/auth"
	"github.com/diegovhdev/yortale/internal/platform/dbtest"
)

// Mover capítulos por HTTP: el componente chapterOrder (fetch con Accept: application/json)
// recibe 204, y el formulario sin JS una redirección a la historia. Cada respuesta debe
// corresponder a un cambio real en la base de datos.
func TestMoveChapterHTTPIntegration(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	users := auth.NewService(auth.NewUserRepository(pool), auth.NewSessionRepository(pool))
	svc := NewService(NewStoryRepository(pool), NewChapterRepository(pool), users)
	author := createUser(t, pool, "autora")
	other := createUser(t, pool, "otra")
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}

	story, err := svc.Create(ctx, author.ID, StoryInput{Title: "Mover", Synopsis: testSynopsis, Kind: KindOriginal,
		Language: "es", ContentRating: "general", Warnings: []string{WarningNone}})
	must(err)
	_, first, err := svc.CreateChapter(ctx, author.ID, story.ID, ChapterInput{Title: "Uno", Content: "corto"})
	must(err)
	_, _, err = svc.CreateChapter(ctx, author.ID, story.ID, ChapterInput{Title: "Dos", Content: "corto"})
	must(err)

	mux := http.NewServeMux()
	mux.Handle("POST /chapters/{id}/move", auth.RequireUser(http.HandlerFunc(NewHandler(svc, nil).MoveChapter)))
	srv := auth.LoadUser(users, false)(mux)

	move := func(userID int64, direction, accept string) *httptest.ResponseRecorder {
		t.Helper()
		token, _, err := users.StartSession(ctx, userID)
		must(err)
		req := httptest.NewRequest("POST", first.MoveURL(), strings.NewReader(url.Values{"direction": {direction}}.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if accept != "" {
			req.Header.Set("Accept", accept)
		}
		req.AddCookie(&http.Cookie{Name: "yortale_session", Value: token})
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		return rec
	}
	position := func() int {
		t.Helper()
		c, err := NewChapterRepository(pool).Get(ctx, first.ID)
		must(err)
		return c.Position
	}

	if rec := move(author.ID, "down", "application/json"); rec.Code != http.StatusNoContent || position() != 2 {
		t.Fatalf("fetch: status %d, posición %d; se esperaba 204 y 2", rec.Code, position())
	}
	if rec := move(author.ID, "up", "text/html"); rec.Code != http.StatusSeeOther || position() != 1 {
		t.Fatalf("formulario: status %d, posición %d; se esperaba 303 y 1", rec.Code, position())
	}
	// Errores que el componente deshace: capítulo ajeno y dirección inválida.
	if rec := move(other.ID, "down", "application/json"); rec.Code != http.StatusNotFound || position() != 1 {
		t.Fatalf("capítulo ajeno: status %d, posición %d; se esperaba 404 y 1", rec.Code, position())
	}
	if rec := move(author.ID, "left", "application/json"); rec.Code != http.StatusBadRequest {
		t.Fatalf("dirección inválida: status %d, se esperaba 400", rec.Code)
	}
}
