package moderation

import (
	"context"
	"errors"
	"testing"

	"github.com/diegovhdev/yortale/internal/auth"
	"github.com/diegovhdev/yortale/internal/platform/dbtest"
	"github.com/diegovhdev/yortale/internal/stories"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestModerationIntegration(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	users := auth.NewService(auth.NewUserRepository(pool), auth.NewSessionRepository(pool))
	storySvc := stories.NewService(stories.NewStoryRepository(pool), stories.NewChapterRepository(pool), users)
	svc := NewService(NewRepository(pool), storySvc, users)

	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	const password = "contraseña-de-prueba"
	authorUser, err := users.Register(ctx, auth.RegisterInput{Username: "autora", Email: "autora@example.com",
		Password: password, PasswordConfirm: password, AcceptTerms: true})
	must(err)
	author := &authorUser
	reader := withRole(t, pool, "lector", auth.RoleUser)
	mod := withRole(t, pool, "moderadora", auth.RoleModerator)
	admin := withRole(t, pool, "admin", auth.RoleAdmin)

	// Una historia pública con un capítulo.
	story, err := storySvc.Create(ctx, author.ID, stories.StoryInput{Title: "Robada", Kind: stories.KindOriginal,
		Language: "es", ContentRating: "general", Warnings: []string{stories.WarningNone}})
	must(err)
	_, chapter, err := storySvc.CreateChapter(ctx, author.ID, story.ID,
		stories.ChapterInput{Title: "Uno", Content: "Texto copiado.", Publish: true})
	must(err)

	// ---------- Reportar ----------
	if _, err := svc.ReportTarget(ctx, author, story.ID, 0); !errors.Is(err, ErrOwnContent) {
		t.Fatalf("reportar lo propio: se esperaba ErrOwnContent, llegó %v", err)
	}
	if _, err := svc.ReportTarget(ctx, reader, story.ID, chapter.ID+100); !errors.Is(err, ErrNotFound) {
		t.Fatalf("capítulo inexistente: se esperaba ErrNotFound, llegó %v", err)
	}
	var verrs auth.ValidationErrors
	if _, err := svc.Report(ctx, reader, ReportInput{StoryID: story.ID, Reason: "aburrida"}); !errors.As(err, &verrs) || verrs["reason"] == "" {
		t.Fatalf("motivo inválido: llegó %v", err)
	}
	if _, err := svc.Report(ctx, reader, ReportInput{StoryID: story.ID, Reason: ReasonOther}); !errors.As(err, &verrs) || verrs["details"] == "" {
		t.Fatalf("«otro» sin detalles: llegó %v", err)
	}
	report := ReportInput{StoryID: story.ID, Reason: "plagiarism", Details: "Es de otro autor."}
	_, err = svc.Report(ctx, reader, report)
	must(err)
	_, err = svc.Report(ctx, reader, report) // repetido: no debe crear otro
	must(err)
	_, err = svc.Report(ctx, reader, ReportInput{StoryID: story.ID, ChapterID: chapter.ID, Reason: "spam"})
	must(err)

	cases, err := svc.Cases(ctx)
	must(err)
	if len(cases) != 1 || cases[0].StoryID != story.ID || cases[0].OpenReports != 2 || len(cases[0].Reasons) != 2 {
		t.Fatalf("cola inesperada: %+v", cases)
	}

	// ---------- Ocultar ----------
	if err := svc.HideStory(ctx, mod, story.ID, "  "); !errors.As(err, &verrs) || verrs["reason"] == "" {
		t.Fatalf("ocultar sin motivo: llegó %v", err)
	}
	must(svc.HideStory(ctx, mod, story.ID, "Plagio confirmado."))
	if err := svc.HideStory(ctx, mod, story.ID, "Otra vez."); !errors.Is(err, ErrNoChange) {
		t.Fatalf("ocultar dos veces: se esperaba ErrNoChange, llegó %v", err)
	}
	if listed := explore(t, storySvc); len(listed) != 0 {
		t.Fatalf("una historia oculta no debería salir en explorar: %+v", listed)
	}
	if _, err := storySvc.Visible(ctx, reader, story.ID); !errors.Is(err, stories.ErrNotFound) {
		t.Fatalf("un lector no debería ver una historia oculta: %v", err)
	}
	for _, viewer := range []*auth.User{author, mod} {
		if s, err := storySvc.Visible(ctx, viewer, story.ID); err != nil || !s.IsHidden() || s.HiddenReason != "Plagio confirmado." {
			t.Fatalf("%s debería ver la historia oculta: %+v, %v", viewer.Username, s, err)
		}
	}
	if _, err := svc.ReportTarget(ctx, reader, story.ID, 0); !errors.Is(err, ErrNotFound) {
		t.Fatalf("reportar una historia oculta: se esperaba ErrNotFound, llegó %v", err)
	}
	view, err := svc.StoryCase(ctx, story.ID)
	must(err)
	if view.OpenReports() != 0 || len(view.Reports) != 2 || view.Reports[0].Status != StatusResolved ||
		len(view.Actions) != 1 || view.Actions[0].Action != ActionHideStory {
		t.Fatalf("caso tras ocultar: %+v", view)
	}
	if cases, _ := svc.Cases(ctx); len(cases) != 0 {
		t.Fatalf("la cola debería quedar vacía: %+v", cases)
	}

	must(svc.UnhideStory(ctx, mod, story.ID))
	if listed := explore(t, storySvc); len(listed) != 1 {
		t.Fatalf("la historia debería volver a explorar: %+v", listed)
	}

	// ---------- Descartar ----------
	if err := svc.DismissReports(ctx, mod, story.ID); !errors.Is(err, ErrNoChange) {
		t.Fatalf("descartar sin reportes abiertos: se esperaba ErrNoChange, llegó %v", err)
	}
	_, err = svc.Report(ctx, reader, report) // el anterior está cerrado: este sí se crea
	must(err)
	must(svc.DismissReports(ctx, mod, story.ID))
	if cases, _ := svc.Cases(ctx); len(cases) != 0 {
		t.Fatalf("la cola debería quedar vacía tras descartar: %+v", cases)
	}

	// ---------- Suspender ----------
	token, _, err := users.StartSession(ctx, author.ID)
	must(err)
	if err := svc.Suspend(ctx, mod, "admin", SuspendInput{Duration: "7", Reason: "x"}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("moderador suspende a administrador: se esperaba ErrForbidden, llegó %v", err)
	}
	if err := svc.Suspend(ctx, mod, "autora", SuspendInput{Duration: durationPermanent, Reason: "x"}); !errors.As(err, &verrs) || verrs["duration"] == "" {
		t.Fatalf("moderador suspende de forma indefinida: llegó %v", err)
	}
	must(svc.Suspend(ctx, mod, "AUTORA", SuspendInput{Duration: "7", Reason: "Plagio reiterado."}))
	if _, err := users.UserFromSession(ctx, token); !errors.Is(err, auth.ErrNoSession) {
		t.Fatalf("la sesión de una cuenta suspendida debería cerrarse: %v", err)
	}
	if _, err := users.Authenticate(ctx, "autora", "otra-contraseña"); !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatalf("contraseña incorrecta de cuenta suspendida: llegó %v", err)
	}
	_, err = users.Authenticate(ctx, "autora", password)
	var suspended auth.SuspendedError
	if !errors.As(err, &suspended) || suspended.User.SuspendedUntil == nil || suspended.User.SuspensionReason != "Plagio reiterado." {
		t.Fatalf("login de cuenta suspendida: llegó %v", err)
	}

	must(svc.Unsuspend(ctx, mod, "autora"))
	if _, err := users.Authenticate(ctx, "autora", password); err != nil {
		t.Fatalf("tras levantar la suspensión debería poder entrar: %v", err)
	}
	must(svc.Suspend(ctx, admin, "autora", SuspendInput{Duration: durationPermanent, Reason: "Spam."}))
	if u, err := users.UserByUsername(ctx, "autora"); err != nil || !u.Suspended || u.SuspendedUntil != nil {
		t.Fatalf("suspensión indefinida: %+v, %v", u, err)
	}

	// ---------- Roles ----------
	if err := svc.SetRole(ctx, mod, "lector", auth.RoleModerator); !errors.Is(err, ErrForbidden) {
		t.Fatalf("un moderador no cambia roles: llegó %v", err)
	}
	if err := svc.SetRole(ctx, admin, "admin", auth.RoleUser); !errors.Is(err, ErrForbidden) {
		t.Fatalf("un administrador no cambia su propio rol: llegó %v", err)
	}
	if err := svc.SetRole(ctx, admin, "lector", "rey"); !errors.As(err, &verrs) || verrs["role"] == "" {
		t.Fatalf("rol inválido: llegó %v", err)
	}
	must(svc.SetRole(ctx, admin, "lector", auth.RoleModerator))
	if u, _ := users.UserByUsername(ctx, "lector"); u.Role != auth.RoleModerator {
		t.Fatalf("rol tras el cambio = %q", u.Role)
	}

	// ---------- Registro ----------
	page, err := svc.UserPage(ctx, admin, "autora")
	must(err)
	// ocultar, mostrar, descartar, suspender, levantar, suspender
	if len(page.Actions) != 6 || page.Actions[0].Action != ActionSuspendUser || !page.CanSuspend || !page.CanChangeRole {
		t.Fatalf("página de la cuenta: %+v", page)
	}
	log, _, err := svc.Log(ctx, 1)
	must(err)
	if len(log) != 7 || log[0].Action != ActionChangeRole || log[0].Details != "De Usuario a Moderador." {
		t.Fatalf("registro: %+v", log)
	}
}

// withRole crea una cuenta con ese rol.
func withRole(t *testing.T, pool *pgxpool.Pool, username, role string) *auth.User {
	t.Helper()
	id := dbtest.CreateUser(t, pool, username)
	if _, err := pool.Exec(context.Background(), `UPDATE users SET role = $2 WHERE id = $1`, id, role); err != nil {
		t.Fatal(err)
	}
	return &auth.User{ID: id, Username: username, Role: role}
}

func explore(t *testing.T, svc *stories.Service) []stories.Story {
	t.Helper()
	list, _, err := svc.Explore(context.Background(), stories.ExploreFilter{})
	if err != nil {
		t.Fatal(err)
	}
	return list
}
