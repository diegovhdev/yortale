package comments

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

// commentColumns espera el comentario como c y su autor como u (LEFT JOIN: la cuenta puede
// no existir). El texto de un comentario borrado no sale de la base de datos.
const commentColumns = `c.id, c.story_id, c.chapter_id, c.parent_id, c.user_id,
	u.username::text, u.display_name,
	CASE WHEN c.deleted_at IS NULL THEN c.body ELSE '' END,
	c.created_at, c.updated_at, c.deleted_at IS NOT NULL`

func scanComment(row pgx.Row) (Comment, error) {
	var c Comment
	var username, displayName *string
	var chapter *int64 // NULL en los comentarios a la historia, que todavía no existen
	err := row.Scan(&c.ID, &c.StoryID, &chapter, &c.ParentID, &c.UserID,
		&username, &displayName, &c.Body, &c.CreatedAt, &c.UpdatedAt, &c.Deleted)
	if chapter != nil {
		c.ChapterID = *chapter
	}
	if username != nil {
		c.Username = *username
	}
	if displayName != nil {
		c.DisplayName = *displayName
	}
	return c, err
}

func collectComments(rows pgx.Rows) ([]Comment, error) {
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Comment, error) { return scanComment(row) })
}

func (r *Repository) Get(ctx context.Context, id int64) (Comment, error) {
	c, err := scanComment(r.db.QueryRow(ctx, `SELECT `+commentColumns+`
		FROM comments c LEFT JOIN users u ON u.id = c.user_id WHERE c.id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Comment{}, ErrNotFound
	}
	if err != nil {
		return Comment{}, fmt.Errorf("buscar comentario: %w", err)
	}
	return c, nil
}

// Threads devuelve una página de comentarios principales del capítulo, los más nuevos primero,
// cada uno con sus respuestas en orden. Un principal borrado solo aparece si tiene respuestas
// sin borrar, para no dejarlas sin contexto. Pide uno de más para saber si hay otra página.
func (r *Repository) Threads(ctx context.Context, chapterID int64, limit, offset int) ([]Comment, bool, error) {
	rows, err := r.db.Query(ctx, `SELECT `+commentColumns+`
		FROM comments c LEFT JOIN users u ON u.id = c.user_id
		WHERE c.chapter_id = $1 AND c.parent_id IS NULL
		  AND (c.deleted_at IS NULL OR EXISTS (
		        SELECT 1 FROM comments r WHERE r.parent_id = c.id AND r.deleted_at IS NULL))
		ORDER BY c.created_at DESC, c.id DESC
		LIMIT $2 OFFSET $3`, chapterID, limit+1, offset)
	if err != nil {
		return nil, false, fmt.Errorf("listar comentarios: %w", err)
	}
	threads, err := collectComments(rows)
	if err != nil {
		return nil, false, fmt.Errorf("listar comentarios: %w", err)
	}
	hasNext := len(threads) > limit
	if hasNext {
		threads = threads[:limit]
	}
	if len(threads) == 0 {
		return threads, false, nil
	}

	ids := make([]int64, len(threads))
	index := make(map[int64]int, len(threads))
	for i, t := range threads {
		ids[i] = t.ID
		index[t.ID] = i
	}
	rows, err = r.db.Query(ctx, `SELECT `+commentColumns+`
		FROM comments c LEFT JOIN users u ON u.id = c.user_id
		WHERE c.parent_id = ANY($1) AND c.deleted_at IS NULL
		ORDER BY c.created_at, c.id`, ids)
	if err != nil {
		return nil, false, fmt.Errorf("listar respuestas: %w", err)
	}
	replies, err := collectComments(rows)
	if err != nil {
		return nil, false, fmt.Errorf("listar respuestas: %w", err)
	}
	for _, reply := range replies {
		i := index[*reply.ParentID]
		threads[i].Replies = append(threads[i].Replies, reply)
	}
	return threads, hasNext, nil
}

// ThreadsBefore cuenta los hilos que Threads muestra antes que threadID, para saber en qué
// página está. Las condiciones deben coincidir con las de Threads.
func (r *Repository) ThreadsBefore(ctx context.Context, chapterID, threadID int64) (int, error) {
	var n int
	err := r.db.QueryRow(ctx, `
		SELECT count(*) FROM comments c, comments t
		WHERE t.id = $2 AND c.chapter_id = $1 AND c.parent_id IS NULL
		  AND (c.created_at, c.id) > (t.created_at, t.id)
		  AND (c.deleted_at IS NULL OR EXISTS (
		        SELECT 1 FROM comments r WHERE r.parent_id = c.id AND r.deleted_at IS NULL))`,
		chapterID, threadID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("buscar página del comentario: %w", err)
	}
	return n, nil
}

// Count cuenta los comentarios sin borrar del capítulo, respuestas incluidas.
func (r *Repository) Count(ctx context.Context, chapterID int64) (int, error) {
	var n int
	err := r.db.QueryRow(ctx,
		`SELECT count(*) FROM comments WHERE chapter_id = $1 AND deleted_at IS NULL`, chapterID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("contar comentarios: %w", err)
	}
	return n, nil
}

type CreateParams struct {
	UserID    int64
	StoryID   int64
	ChapterID int64
	ParentID  *int64
	Body      string
}

func (r *Repository) Create(ctx context.Context, p CreateParams) (Comment, error) {
	c, err := scanComment(r.db.QueryRow(ctx, `
		WITH c AS (
			INSERT INTO comments (user_id, story_id, chapter_id, parent_id, body)
			VALUES ($1, $2, $3, $4, $5)
			RETURNING *)
		SELECT `+commentColumns+` FROM c LEFT JOIN users u ON u.id = c.user_id`,
		p.UserID, p.StoryID, p.ChapterID, p.ParentID, p.Body))
	if err != nil {
		return Comment{}, fmt.Errorf("crear comentario: %w", err)
	}
	return c, nil
}

// UpdateBody cambia el texto de un comentario sin borrar. updated_at lo pone el trigger.
func (r *Repository) UpdateBody(ctx context.Context, id int64, body string) (Comment, error) {
	c, err := scanComment(r.db.QueryRow(ctx, `
		WITH c AS (
			UPDATE comments SET body = $2 WHERE id = $1 AND deleted_at IS NULL
			RETURNING *)
		SELECT `+commentColumns+` FROM c LEFT JOIN users u ON u.id = c.user_id`, id, body))
	if errors.Is(err, pgx.ErrNoRows) {
		return Comment{}, ErrNotFound
	}
	if err != nil {
		return Comment{}, fmt.Errorf("editar comentario: %w", err)
	}
	return c, nil
}

// Delete hace un borrado suave: el comentario se queda para no romper su hilo, pero sin texto
// visible. Las respuestas no se tocan.
func (r *Repository) Delete(ctx context.Context, id int64) (Comment, error) {
	c, err := scanComment(r.db.QueryRow(ctx, `
		WITH c AS (
			UPDATE comments SET deleted_at = now() WHERE id = $1 AND deleted_at IS NULL
			RETURNING *)
		SELECT `+commentColumns+` FROM c LEFT JOIN users u ON u.id = c.user_id`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Comment{}, ErrNotFound
	}
	if err != nil {
		return Comment{}, fmt.Errorf("borrar comentario: %w", err)
	}
	return c, nil
}
