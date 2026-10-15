package stories

import (
	"context"
	"errors"
	"fmt"

	"github.com/diegovhdev/yortale/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type StoryRepository struct {
	db *pgxpool.Pool
}

func NewStoryRepository(db *pgxpool.Pool) *StoryRepository {
	return &StoryRepository{db: db}
}

// storyColumns espera las tablas con los alias de storyJoins: s (historia), u (autor), g (género).
const storyColumns = `s.id, s.author_id, u.username::text, coalesce(nullif(u.display_name, ''), u.username::text),
	s.genre_id, g.name, s.title, s.slug, coalesce(s.synopsis, ''), s.language, s.content_rating, s.status,
	s.published_at, s.last_chapter_at, s.reads_count,
	(SELECT count(*) FROM chapters c WHERE c.story_id = s.id AND c.is_published),
	s.created_at, s.updated_at`

// storyJoins une la historia (alias s) con su autor y su género. La tabla s puede ser stories
// o un CTE con sus mismas columnas, como las filas de un INSERT ... RETURNING *.
const storyJoins = `
		JOIN users u ON u.id = s.author_id
		LEFT JOIN genres g ON g.id = s.genre_id`

func scanStory(row pgx.Row) (Story, error) {
	var s Story
	err := row.Scan(&s.ID, &s.AuthorID, &s.AuthorUsername, &s.AuthorName,
		&s.GenreID, &s.GenreName, &s.Title, &s.Slug, &s.Synopsis, &s.Language, &s.ContentRating, &s.Status,
		&s.PublishedAt, &s.LastChapterAt, &s.ReadsCount, &s.ChapterCount,
		&s.CreatedAt, &s.UpdatedAt)
	return s, err
}

func collectStories(rows pgx.Rows) ([]Story, error) {
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Story, error) { return scanStory(row) })
}

// StoryParams son los campos que el autor puede escribir. AuthorID identifica al dueño:
// al crear es el autor nuevo; al actualizar, solo se modifica la historia si le pertenece.
type StoryParams struct {
	AuthorID      int64
	GenreID       *int16
	Title         string
	Slug          string
	Synopsis      string
	Language      string
	ContentRating string
	Status        string
}

// slugConstraint es el nombre que PostgreSQL genera para UNIQUE (author_id, slug).
const slugConstraint = "stories_author_id_slug_key"

// Create inserta una historia. Devuelve ErrSlugTaken si el autor ya usa ese slug.
func (r *StoryRepository) Create(ctx context.Context, p StoryParams) (Story, error) {
	const q = `
		WITH saved AS (
			INSERT INTO stories (author_id, genre_id, title, slug, synopsis, language, content_rating, status, published_at)
			VALUES ($1, $2, $3, $4, nullif($5, ''), $6, $7, $8, CASE WHEN $8 <> 'draft' THEN now() END)
			RETURNING *
		)
		SELECT ` + storyColumns + ` FROM saved s` + storyJoins

	s, err := scanStory(r.db.QueryRow(ctx, q,
		p.AuthorID, p.GenreID, p.Title, p.Slug, p.Synopsis, p.Language, p.ContentRating, p.Status))
	if err != nil {
		if database.UniqueViolation(err) == slugConstraint {
			return Story{}, ErrSlugTaken
		}
		return Story{}, fmt.Errorf("crear historia: %w", err)
	}
	return s, nil
}

// Update modifica la historia id si pertenece a p.AuthorID; si no, devuelve ErrNotFound.
// published_at se fija la primera vez que la historia deja de ser borrador.
func (r *StoryRepository) Update(ctx context.Context, id int64, p StoryParams) (Story, error) {
	const q = `
		WITH saved AS (
			UPDATE stories SET
				genre_id = $3, title = $4, slug = $5, synopsis = nullif($6, ''),
				language = $7, content_rating = $8, status = $9,
				published_at = CASE WHEN $9 <> 'draft' THEN coalesce(published_at, now()) ELSE published_at END
			WHERE id = $1 AND author_id = $2
			RETURNING *
		)
		SELECT ` + storyColumns + ` FROM saved s` + storyJoins

	s, err := scanStory(r.db.QueryRow(ctx, q, id, p.AuthorID,
		p.GenreID, p.Title, p.Slug, p.Synopsis, p.Language, p.ContentRating, p.Status))
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return Story{}, ErrNotFound
	case database.UniqueViolation(err) == slugConstraint:
		return Story{}, ErrSlugTaken
	case err != nil:
		return Story{}, fmt.Errorf("actualizar historia: %w", err)
	}
	return s, nil
}

func (r *StoryRepository) Get(ctx context.Context, id int64) (Story, error) {
	const q = `SELECT ` + storyColumns + ` FROM stories s` + storyJoins + ` WHERE s.id = $1`
	s, err := scanStory(r.db.QueryRow(ctx, q, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Story{}, ErrNotFound
	}
	if err != nil {
		return Story{}, fmt.Errorf("buscar historia: %w", err)
	}
	return s, nil
}

// ListPublished devuelve las historias que no son borradores. Con query busca por texto completo
// (columna search, configuración spanish) y, para tolerar errores de escritura, por parecido
// del título con pg_trgm: % compara con el título entero y <% con su palabra más parecida,
// que funciona mejor con búsquedas cortas. Sin query, ordena por las actualizadas más recientemente.
func (r *StoryRepository) ListPublished(ctx context.Context, query string, genreID *int16, limit, offset int) ([]Story, error) {
	const q = `
		SELECT ` + storyColumns + ` FROM stories s` + storyJoins + `
		WHERE s.status <> 'draft'
			AND ($1::text = '' OR s.search @@ websearch_to_tsquery('spanish', $1)
				OR s.title % $1 OR $1 <% s.title)
			AND ($2::smallint IS NULL OR s.genre_id = $2)
		ORDER BY
			CASE WHEN $1 = '' THEN 0
				ELSE ts_rank(s.search, websearch_to_tsquery('spanish', $1)) + word_similarity($1, s.title)
			END DESC,
			s.last_chapter_at DESC NULLS LAST,
			s.id DESC
		LIMIT $3 OFFSET $4`

	rows, err := r.db.Query(ctx, q, query, genreID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("listar historias: %w", err)
	}
	stories, err := collectStories(rows)
	if err != nil {
		return nil, fmt.Errorf("listar historias: %w", err)
	}
	return stories, nil
}

// ListByAuthor devuelve todas las historias del autor, borradores incluidos.
// Se ordena por created_at y no por updated_at porque cada lectura nueva toca la fila (reads_count).
func (r *StoryRepository) ListByAuthor(ctx context.Context, authorID int64) ([]Story, error) {
	const q = `SELECT ` + storyColumns + ` FROM stories s` + storyJoins + `
		WHERE s.author_id = $1
		ORDER BY s.created_at DESC`

	rows, err := r.db.Query(ctx, q, authorID)
	if err != nil {
		return nil, fmt.Errorf("listar historias del autor: %w", err)
	}
	stories, err := collectStories(rows)
	if err != nil {
		return nil, fmt.Errorf("listar historias del autor: %w", err)
	}
	return stories, nil
}

// Delete borra la historia si pertenece al autor. Sus capítulos, comentarios, etc. se borran en cascada.
func (r *StoryRepository) Delete(ctx context.Context, id, authorID int64) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM stories WHERE id = $1 AND author_id = $2`, id, authorID)
	if err != nil {
		return fmt.Errorf("borrar historia: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *StoryRepository) ListGenres(ctx context.Context) ([]Genre, error) {
	rows, err := r.db.Query(ctx, `SELECT id, name, slug FROM genres ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("listar géneros: %w", err)
	}
	genres, err := pgx.CollectRows(rows, pgx.RowToStructByPos[Genre])
	if err != nil {
		return nil, fmt.Errorf("listar géneros: %w", err)
	}
	return genres, nil
}

// RecordRead guarda por qué capítulo va el lector (reading_progress) y, si es la primera vez
// que lee esta historia, suma uno a stories.reads_count. Así reads_count cuenta lectores
// distintos con sesión iniciada, y recargar la página no lo infla.
func (r *StoryRepository) RecordRead(ctx context.Context, userID, storyID, chapterID int64) error {
	const q = `
		WITH progress AS (
			INSERT INTO reading_progress (user_id, story_id, chapter_id)
			VALUES ($1, $2, $3)
			ON CONFLICT (user_id, story_id)
				DO UPDATE SET chapter_id = EXCLUDED.chapter_id, updated_at = now()
			RETURNING (xmax = 0) AS first_read -- xmax = 0: la fila se insertó, no se actualizó
		)
		UPDATE stories SET reads_count = reads_count + 1
		WHERE id = $2 AND (SELECT first_read FROM progress)`

	if _, err := r.db.Exec(ctx, q, userID, storyID, chapterID); err != nil {
		return fmt.Errorf("registrar lectura: %w", err)
	}
	return nil
}
