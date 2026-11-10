package comments

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/diegovhdev/yortale/internal/auth"
	"github.com/diegovhdev/yortale/internal/platform/dbtest"
	"github.com/diegovhdev/yortale/internal/stories"
)

func TestCommentsIntegration(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	users := auth.NewService(auth.NewUserRepository(pool), auth.NewSessionRepository(pool))
	storySvc := stories.NewService(stories.NewStoryRepository(pool), stories.NewChapterRepository(pool), users)
	svc := NewService(NewRepository(pool), storySvc)

	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	const password = "contraseña-de-prueba"
	register := func(name, role string) *auth.User {
		t.Helper()
		u, err := users.Register(ctx, auth.RegisterInput{Username: name, Email: name + "@example.com",
			Password: password, PasswordConfirm: password, AcceptTerms: true})
		must(err)
		_, err = pool.Exec(ctx, `UPDATE users SET role = $2 WHERE id = $1`, u.ID, role)
		must(err)
		u.Role = role
		return &u
	}
	author := register("autora", auth.RoleUser)
	reader := register("lector", auth.RoleUser)
	other := register("otra", auth.RoleUser)
	mod := register("moderadora", auth.RoleModerator)

	story, err := storySvc.Create(ctx, author.ID, stories.StoryInput{Title: "Con comentarios", Kind: stories.KindOriginal,
		Language: "es", ContentRating: "general", Warnings: []string{stories.WarningNone}})
	must(err)
	_, draft, err := storySvc.CreateChapter(ctx, author.ID, story.ID, stories.ChapterInput{Title: "Borrador", Content: "Aún no."})
	must(err)
	_, chapter, err := storySvc.CreateChapter(ctx, author.ID, story.ID,
		stories.ChapterInput{Title: "Uno", Content: "Había una vez.", Publish: true})
	must(err)
	// Al publicar el primer capítulo la historia deja de ser borrador.
	story, err = storySvc.Story(ctx, story.ID)
	must(err)
	if story.IsDraft() {
		_, err = pool.Exec(ctx, `UPDATE stories SET status = 'ongoing' WHERE id = $1`, story.ID)
		must(err)
	}

	// ---------- Comentar ----------
	if _, _, err := svc.Create(ctx, reader, draft.ID, Input{Body: "Hola"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("comentar un borrador ajeno: se esperaba ErrNotFound, llegó %v", err)
	}
	if _, _, err := svc.Create(ctx, author, draft.ID, Input{Body: "Hola"}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("comentar un borrador propio: se esperaba ErrForbidden, llegó %v", err)
	}
	var verrs auth.ValidationErrors
	if _, _, err := svc.Create(ctx, reader, chapter.ID, Input{Body: "  \r\n "}); !errors.As(err, &verrs) || verrs["body"] == "" {
		t.Fatalf("comentario vacío: llegó %v", err)
	}
	if _, _, err := svc.Create(ctx, reader, chapter.ID, Input{Body: strings.Repeat("a", maxBodyRunes+1)}); !errors.As(err, &verrs) {
		t.Fatalf("comentario largo: llegó %v", err)
	}
	// Los \r\n cuentan como un solo carácter: 5000 runas con saltos deben caber en el CHECK.
	_, long, err := svc.Create(ctx, reader, chapter.ID, Input{Body: strings.Repeat("a\r\n", maxBodyRunes/2)})
	must(err)

	_, first, err := svc.Create(ctx, reader, chapter.ID, Input{Body: "  ¡Me encantó!\r\nSigue así.  "})
	must(err)
	if first.Body != "¡Me encantó!\nSigue así." || first.Username != "lector" || first.IsReply() {
		t.Fatalf("comentario inesperado: %+v", first)
	}
	_, reply, err := svc.Create(ctx, author, chapter.ID, Input{Body: "¡Gracias!", ParentID: strconv.FormatInt(first.ID, 10)})
	must(err)
	if reply.ParentID == nil || *reply.ParentID != first.ID {
		t.Fatalf("respuesta sin padre: %+v", reply)
	}
	// No se responde a una respuesta.
	if _, _, err := svc.Create(ctx, other, chapter.ID, Input{Body: "Yo también", ParentID: strconv.FormatInt(reply.ID, 10)}); !errors.As(err, &verrs) || verrs["parent"] == "" {
		t.Fatalf("responder a una respuesta: llegó %v", err)
	}

	sec, err := svc.ChapterSection(ctx, nil, story, chapter, 1)
	must(err)
	if len(sec.Threads) != 2 || sec.Threads[0].ID != first.ID || sec.Threads[1].ID != long.ID || sec.Total != 3 {
		t.Fatalf("sección inesperada: total %d, hilos %+v", sec.Total, sec.Threads)
	}
	if len(sec.Threads[0].Replies) != 1 || !sec.IsStoryAuthor(sec.Threads[0].Replies[0]) {
		t.Fatalf("respuestas inesperadas: %+v", sec.Threads[0].Replies)
	}
	if sec.CanPost() {
		t.Fatal("un visitante no debe poder comentar")
	}

	// ---------- Editar ----------
	if _, err := svc.Update(ctx, author, first.ID, "Cambiado"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("editar uno ajeno: se esperaba ErrForbidden, llegó %v", err)
	}
	edited, err := svc.Update(ctx, reader, first.ID, "Me encantó, de verdad.")
	must(err)
	if edited.Comment.Body != "Me encantó, de verdad." {
		t.Fatalf("texto no actualizado: %q", edited.Comment.Body)
	}

	// ---------- Borrar ----------
	if _, err := svc.Delete(ctx, other, first.ID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("borrar uno ajeno: se esperaba ErrForbidden, llegó %v", err)
	}
	// El autor de la historia puede borrar comentarios en ella.
	_, err = svc.Delete(ctx, author, long.ID)
	must(err)
	if _, err := svc.Delete(ctx, author, long.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("borrar dos veces: se esperaba ErrNotFound, llegó %v", err)
	}
	// Un principal borrado con respuestas sigue en la lista, sin texto.
	_, err = svc.Delete(ctx, reader, first.ID)
	must(err)
	sec, err = svc.ChapterSection(ctx, nil, story, chapter, 1)
	must(err)
	if len(sec.Threads) != 1 || !sec.Threads[0].Deleted || sec.Threads[0].Body != "" ||
		len(sec.Threads[0].Replies) != 1 || sec.Total != 1 {
		t.Fatalf("tras borrar: total %d, hilos %+v", sec.Total, sec.Threads)
	}
	if _, _, err := svc.Create(ctx, other, chapter.ID, Input{Body: "Tarde", ParentID: strconv.FormatInt(first.ID, 10)}); !errors.As(err, &verrs) {
		t.Fatalf("responder a uno borrado: llegó %v", err)
	}
	// La moderación puede borrar cualquiera.
	_, err = svc.Delete(ctx, mod, reply.ID)
	must(err)
	sec, err = svc.ChapterSection(ctx, nil, story, chapter, 1)
	must(err)
	if len(sec.Threads) != 0 || sec.Total != 0 {
		t.Fatalf("sin comentarios vivos debería quedar vacía: %+v", sec.Threads)
	}

	// ---------- Paginación y enlace permanente ----------
	var last Comment
	for i := range threadsPerPage + 1 {
		_, last, err = svc.Create(ctx, other, chapter.ID, Input{Body: "Comentario " + strconv.Itoa(i)})
		must(err)
	}
	sec, err = svc.ChapterSection(ctx, nil, story, chapter, 1)
	must(err)
	if len(sec.Threads) != threadsPerPage || !sec.HasNext || sec.Total != threadsPerPage+1 || sec.Threads[0].ID != last.ID {
		t.Fatalf("página 1: %d hilos, hasNext %v, total %d", len(sec.Threads), sec.HasNext, sec.Total)
	}
	oldest := sec.Threads[len(sec.Threads)-1].ID - 1 // el primero de la tanda, en la página 2
	target, err := svc.Find(ctx, nil, oldest)
	must(err)
	link, err := svc.Permalink(ctx, target)
	must(err)
	if want := chapter.URL(story) + "?comments=2#comment-" + strconv.FormatInt(oldest, 10); link != want {
		t.Fatalf("enlace permanente: %q, se esperaba %q", link, want)
	}

	// ---------- Historia oculta ----------
	_, err = pool.Exec(ctx, `UPDATE stories SET hidden_at = now(), hidden_reason = 'prueba' WHERE id = $1`, story.ID)
	must(err)
	if _, _, err := svc.Create(ctx, other, chapter.ID, Input{Body: "Hola"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("comentar una historia oculta: se esperaba ErrNotFound, llegó %v", err)
	}
	if _, _, err := svc.Create(ctx, author, chapter.ID, Input{Body: "Hola"}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("el autor no debe comentar su historia oculta: llegó %v", err)
	}
	if _, err := svc.Update(ctx, other, last.ID, "Editado"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("editar en una historia oculta: se esperaba ErrNotFound, llegó %v", err)
	}
}
