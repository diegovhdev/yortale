package moderation

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

// ---------- Reportes ----------

// PublishedChapterTitle devuelve el título del capítulo si es de esa historia y está publicado.
func (r *Repository) PublishedChapterTitle(ctx context.Context, storyID, chapterID int64) (string, error) {
	var title string
	err := r.db.QueryRow(ctx,
		`SELECT title FROM chapters WHERE id = $1 AND story_id = $2 AND is_published`,
		chapterID, storyID).Scan(&title)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("buscar capítulo reportado: %w", err)
	}
	return title, nil
}

type ReportParams struct {
	ReporterID int64
	StoryID    int64
	ChapterID  int64 // 0 si se reporta la historia entera
	Reason     string
	Details    string
}

// CreateReport guarda el reporte, salvo que la misma persona ya tenga uno abierto sobre lo
// mismo: así reenviar el formulario o insistir no llena la cola. En ese caso no hace nada.
func (r *Repository) CreateReport(ctx context.Context, p ReportParams) error {
	const q = `
		INSERT INTO reports (reporter_id, story_id, chapter_id, reason, details)
		SELECT $1, $2, nullif($3::bigint, 0), $4, nullif($5, '')
		WHERE NOT EXISTS (
			SELECT 1 FROM reports
			WHERE reporter_id = $1 AND story_id = $2 AND status = 'open'
				AND chapter_id IS NOT DISTINCT FROM nullif($3::bigint, 0))`
	if _, err := r.db.Exec(ctx, q, p.ReporterID, p.StoryID, p.ChapterID, p.Reason, p.Details); err != nil {
		return fmt.Errorf("crear reporte: %w", err)
	}
	return nil
}

// OpenCases agrupa por historia los reportes abiertos. Primero las que llevan más tiempo esperando.
func (r *Repository) OpenCases(ctx context.Context, limit int) ([]Case, error) {
	const q = `
		SELECT s.id, s.title, u.username::text, s.hidden_at IS NOT NULL,
			count(*), array_agg(DISTINCT r.reason), min(r.created_at)
		FROM reports r
			JOIN stories s ON s.id = r.story_id
			JOIN users u ON u.id = s.author_id
		WHERE r.status = 'open'
		GROUP BY s.id, u.username
		ORDER BY min(r.created_at)
		LIMIT $1`
	rows, err := r.db.Query(ctx, q, limit)
	if err != nil {
		return nil, fmt.Errorf("listar casos abiertos: %w", err)
	}
	cases, err := pgx.CollectRows(rows, pgx.RowToStructByPos[Case])
	if err != nil {
		return nil, fmt.Errorf("listar casos abiertos: %w", err)
	}
	return cases, nil
}

// StoryReports devuelve los reportes de la historia (y de sus capítulos): primero los
// abiertos y, dentro de cada grupo, los más recientes.
func (r *Repository) StoryReports(ctx context.Context, storyID int64, limit int) ([]Report, error) {
	const q = `
		SELECT r.id, r.story_id, r.chapter_id, c.title, ru.username::text, r.reason,
			coalesce(r.details, ''), r.status, vu.username::text, r.reviewed_at, r.created_at
		FROM reports r
			LEFT JOIN chapters c ON c.id = r.chapter_id
			LEFT JOIN users ru ON ru.id = r.reporter_id
			LEFT JOIN users vu ON vu.id = r.reviewed_by
		WHERE r.story_id = $1
		ORDER BY r.status = 'open' DESC, r.created_at DESC
		LIMIT $2`
	rows, err := r.db.Query(ctx, q, storyID, limit)
	if err != nil {
		return nil, fmt.Errorf("listar reportes de la historia: %w", err)
	}
	reports, err := pgx.CollectRows(rows, pgx.RowToStructByPos[Report])
	if err != nil {
		return nil, fmt.Errorf("listar reportes de la historia: %w", err)
	}
	return reports, nil
}

// ---------- Acciones sobre historias ----------

// HideStory oculta la historia, da por resueltos sus reportes abiertos y lo registra, todo en
// una transacción. Devuelve ErrNoChange si ya estaba oculta.
func (r *Repository) HideStory(ctx context.Context, moderatorID, storyID int64, reason string) error {
	err := pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		var authorID int64
		var title string
		err := tx.QueryRow(ctx, `
			UPDATE stories SET hidden_at = now(), hidden_reason = $2
			WHERE id = $1 AND hidden_at IS NULL
			RETURNING author_id, title`, storyID, reason).Scan(&authorID, &title)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNoChange
		}
		if err != nil {
			return err
		}
		n, err := closeReports(ctx, tx, moderatorID, storyID, StatusResolved)
		if err != nil {
			return err
		}
		return logAction(ctx, tx, actionParams{
			ModeratorID: moderatorID, Action: ActionHideStory, TargetUserID: authorID, StoryID: storyID,
			Reason: reason, Details: fmt.Sprintf("«%s». Reportes resueltos: %d.", title, n),
		})
	})
	if err != nil && !errors.Is(err, ErrNoChange) {
		return fmt.Errorf("ocultar historia: %w", err)
	}
	return err
}

// UnhideStory vuelve a mostrar la historia y lo registra. Devuelve ErrNoChange si no estaba oculta.
func (r *Repository) UnhideStory(ctx context.Context, moderatorID, storyID int64) error {
	err := pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		var authorID int64
		var title string
		err := tx.QueryRow(ctx, `
			UPDATE stories SET hidden_at = NULL, hidden_reason = NULL
			WHERE id = $1 AND hidden_at IS NOT NULL
			RETURNING author_id, title`, storyID).Scan(&authorID, &title)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNoChange
		}
		if err != nil {
			return err
		}
		return logAction(ctx, tx, actionParams{
			ModeratorID: moderatorID, Action: ActionUnhideStory, TargetUserID: authorID, StoryID: storyID,
			Details: fmt.Sprintf("«%s».", title),
		})
	})
	if err != nil && !errors.Is(err, ErrNoChange) {
		return fmt.Errorf("mostrar historia: %w", err)
	}
	return err
}

// DismissReports descarta los reportes abiertos de la historia y lo registra.
// Devuelve ErrNoChange si no había ninguno.
func (r *Repository) DismissReports(ctx context.Context, moderatorID, storyID int64) error {
	err := pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		n, err := closeReports(ctx, tx, moderatorID, storyID, StatusDismissed)
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrNoChange
		}
		var authorID int64
		var title string
		err = tx.QueryRow(ctx, `SELECT author_id, title FROM stories WHERE id = $1`, storyID).Scan(&authorID, &title)
		if err != nil {
			return err
		}
		return logAction(ctx, tx, actionParams{
			ModeratorID: moderatorID, Action: ActionDismissReports, TargetUserID: authorID, StoryID: storyID,
			Details: fmt.Sprintf("«%s». Reportes descartados: %d.", title, n),
		})
	})
	if err != nil && !errors.Is(err, ErrNoChange) {
		return fmt.Errorf("descartar reportes: %w", err)
	}
	return err
}

// closeReports cierra con status los reportes abiertos de la historia y dice cuántos eran.
func closeReports(ctx context.Context, tx pgx.Tx, moderatorID, storyID int64, status string) (int64, error) {
	tag, err := tx.Exec(ctx, `
		UPDATE reports SET status = $3, reviewed_by = $1, reviewed_at = now()
		WHERE story_id = $2 AND status IN ('open', 'reviewing')`, moderatorID, storyID, status)
	return tag.RowsAffected(), err
}

// ---------- Acciones sobre cuentas ----------

// Suspend suspende la cuenta hasta until (nil = indefinida), cierra todas sus sesiones y lo
// registra. Si ya estaba suspendida, la suspensión nueva reemplaza a la anterior.
func (r *Repository) Suspend(ctx context.Context, moderatorID, userID int64, until *time.Time, reason, details string) error {
	err := pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE users SET suspended_at = now(), suspended_until = $2, suspension_reason = $3
			WHERE id = $1`, userID, until, reason)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		if _, err := tx.Exec(ctx, `DELETE FROM sessions WHERE user_id = $1`, userID); err != nil {
			return err
		}
		return logAction(ctx, tx, actionParams{
			ModeratorID: moderatorID, Action: ActionSuspendUser, TargetUserID: userID,
			Reason: reason, Details: details,
		})
	})
	if err != nil && !errors.Is(err, ErrNotFound) {
		return fmt.Errorf("suspender cuenta: %w", err)
	}
	return err
}

// Unsuspend levanta la suspensión vigente y lo registra. Devuelve ErrNoChange si no la había.
func (r *Repository) Unsuspend(ctx context.Context, moderatorID, userID int64) error {
	err := pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE users u SET suspended_at = NULL, suspended_until = NULL, suspension_reason = NULL
			WHERE id = $1 AND u.suspended_at IS NOT NULL
				AND (u.suspended_until IS NULL OR u.suspended_until > now())`, userID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNoChange
		}
		return logAction(ctx, tx, actionParams{ModeratorID: moderatorID, Action: ActionUnsuspendUser, TargetUserID: userID})
	})
	if err != nil && !errors.Is(err, ErrNoChange) {
		return fmt.Errorf("levantar suspensión: %w", err)
	}
	return err
}

// SetRole cambia el rol de la cuenta y lo registra. Devuelve ErrNoChange si ya tenía ese rol.
func (r *Repository) SetRole(ctx context.Context, adminID, userID int64, role, details string) error {
	err := pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE users SET role = $2 WHERE id = $1 AND role <> $2`, userID, role)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNoChange
		}
		return logAction(ctx, tx, actionParams{
			ModeratorID: adminID, Action: ActionChangeRole, TargetUserID: userID, Details: details,
		})
	})
	if err != nil && !errors.Is(err, ErrNoChange) {
		return fmt.Errorf("cambiar rol: %w", err)
	}
	return err
}

// ---------- Registro ----------

type actionParams struct {
	ModeratorID  int64
	Action       string
	TargetUserID int64 // 0 = ninguna
	StoryID      int64 // 0 = ninguna
	Reason       string
	Details      string
}

func logAction(ctx context.Context, tx pgx.Tx, a actionParams) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO moderation_actions (moderator_id, action, target_user_id, story_id, reason, details)
		VALUES ($1, $2, nullif($3::bigint, 0), nullif($4::bigint, 0), nullif($5, ''), nullif($6, ''))`,
		a.ModeratorID, a.Action, a.TargetUserID, a.StoryID, a.Reason, a.Details)
	return err
}

// ActionFilter limita el registro a una cuenta o a una historia; los ceros no filtran.
type ActionFilter struct {
	UserID  int64
	StoryID int64
}

// Actions devuelve el registro de moderación, lo más reciente primero.
func (r *Repository) Actions(ctx context.Context, f ActionFilter, limit, offset int) ([]Action, error) {
	const q = `
		SELECT a.id, mu.username::text, a.action, tu.username::text, a.story_id,
			coalesce(a.reason, ''), coalesce(a.details, ''), a.created_at
		FROM moderation_actions a
			LEFT JOIN users mu ON mu.id = a.moderator_id
			LEFT JOIN users tu ON tu.id = a.target_user_id
		WHERE ($1::bigint = 0 OR a.target_user_id = $1) AND ($2::bigint = 0 OR a.story_id = $2)
		ORDER BY a.created_at DESC, a.id DESC
		LIMIT $3 OFFSET $4`
	rows, err := r.db.Query(ctx, q, f.UserID, f.StoryID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("listar registro de moderación: %w", err)
	}
	actions, err := pgx.CollectRows(rows, pgx.RowToStructByPos[Action])
	if err != nil {
		return nil, fmt.Errorf("listar registro de moderación: %w", err)
	}
	return actions, nil
}
