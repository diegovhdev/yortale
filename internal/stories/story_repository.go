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

// storyColumns espera las tablas con los alias de storyJoins: s (historia), u (autor).
// Los géneros llegan como un array JSON ordenado por posición; encoding/json no distingue
// mayúsculas en los nombres de campo, así que "id" llena Genre.ID.
const storyColumns = `s.id, s.author_id, u.username::text, coalesce(nullif(u.display_name, ''), u.username::text),
	(SELECT coalesce(json_agg(json_build_object('id', g.id, 'name', g.name, 'slug', g.slug) ORDER BY sg.position), '[]')
		FROM story_genres sg JOIN genres g ON g.id = sg.genre_id WHERE sg.story_id = s.id),
	s.kind, s.title, s.slug, coalesce(s.synopsis, ''), s.language, s.content_rating, s.status,
	s.published_at, s.last_chapter_at, s.reads_count,
	(SELECT count(*) FROM chapters c WHERE c.story_id = s.id AND c.is_published),
	s.created_at, s.updated_at`

// storyJoins une la historia (alias s) con su autor.
const storyJoins = `
		JOIN users u ON u.id = s.author_id`

func scanStory(row pgx.Row) (Story, error) {
	var s Story
	err := row.Scan(&s.ID, &s.AuthorID, &s.AuthorUsername, &s.AuthorName,
		&s.Genres, &s.Kind, &s.Title, &s.Slug, &s.Synopsis, &s.Language, &s.ContentRating, &s.Status,
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
	GenreIDs      []int16 // en el orden en que se muestran
	Title         string
	Slug          string
	Synopsis      string
	Kind          string
	Language      string
	ContentRating string
	Status        string
}

// slugConstraint es el nombre que PostgreSQL genera para UNIQUE (author_id, slug).
const slugConstraint = "stories_author_id_slug_key"

// Create inserta una historia con sus géneros. Devuelve ErrSlugTaken si el autor ya usa ese slug.
func (r *StoryRepository) Create(ctx context.Context, p StoryParams) (Story, error) {
	const q = `
		INSERT INTO stories (author_id, title, slug, synopsis, language, content_rating, status, kind, published_at)
		VALUES ($1, $2, $3, nullif($4, ''), $5, $6, $7, $8, CASE WHEN $7 <> 'draft' THEN now() END)
		RETURNING id`

	var s Story
	err := pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		var id int64
		err := tx.QueryRow(ctx, q,
			p.AuthorID, p.Title, p.Slug, p.Synopsis, p.Language, p.ContentRating, p.Status, p.Kind).Scan(&id)
		if err != nil {
			return err
		}
		s, err = saveGenres(ctx, tx, id, p.GenreIDs)
		return err
	})
	switch {
	case database.UniqueViolation(err) == slugConstraint:
		return Story{}, ErrSlugTaken
	case err != nil:
		return Story{}, fmt.Errorf("crear historia: %w", err)
	}
	return s, nil
}

// Update modifica la historia id y sus géneros si pertenece a p.AuthorID; si no, devuelve ErrNotFound.
// published_at se fija la primera vez que la historia deja de ser borrador.
func (r *StoryRepository) Update(ctx context.Context, id int64, p StoryParams) (Story, error) {
	const q = `
		UPDATE stories SET
			title = $3, slug = $4, synopsis = nullif($5, ''),
			language = $6, content_rating = $7, status = $8, kind = $9,
			published_at = CASE WHEN $8 <> 'draft' THEN coalesce(published_at, now()) ELSE published_at END
		WHERE id = $1 AND author_id = $2`

	var s Story
	err := pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, q, id, p.AuthorID,
			p.Title, p.Slug, p.Synopsis, p.Language, p.ContentRating, p.Status, p.Kind)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		s, err = saveGenres(ctx, tx, id, p.GenreIDs)
		return err
	})
	switch {
	case errors.Is(err, ErrNotFound):
		return Story{}, ErrNotFound
	case database.UniqueViolation(err) == slugConstraint:
		return Story{}, ErrSlugTaken
	case err != nil:
		return Story{}, fmt.Errorf("actualizar historia: %w", err)
	}
	return s, nil
}

// saveGenres reemplaza los géneros de la historia, dentro de la transacción de Create o Update,
// y devuelve la historia ya guardada. La posición sale del orden de genreIDs (1, 2, 3).
func saveGenres(ctx context.Context, tx pgx.Tx, storyID int64, genreIDs []int16) (Story, error) {
	if _, err := tx.Exec(ctx, `DELETE FROM story_genres WHERE story_id = $1`, storyID); err != nil {
		return Story{}, err
	}
	const q = `
		INSERT INTO story_genres (story_id, genre_id, position)
		SELECT $1, t.genre_id, t.position
		FROM unnest($2::smallint[]) WITH ORDINALITY AS t(genre_id, position)`
	if _, err := tx.Exec(ctx, q, storyID, genreIDs); err != nil {
		return Story{}, err
	}
	return getStory(ctx, tx, storyID)
}

// querier es lo que comparten el pool y una transacción, para leer una historia desde ambos.
type querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func getStory(ctx context.Context, db querier, id int64) (Story, error) {
	const q = `SELECT ` + storyColumns + ` FROM stories s` + storyJoins + ` WHERE s.id = $1`
	return scanStory(db.QueryRow(ctx, q, id))
}

func (r *StoryRepository) Get(ctx context.Context, id int64) (Story, error) {
	s, err := getStory(ctx, r.db, id)
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
// Con genreIDs, la historia debe tenerlos todos (en cualquier posición). genreIDs vacío y
// kind "" no filtran.
func (r *StoryRepository) ListPublished(ctx context.Context, query string, genreIDs []int16, kind string, limit, offset int) ([]Story, error) {
	const q = `
		SELECT ` + storyColumns + ` FROM stories s` + storyJoins + `
		WHERE s.status <> 'draft'
			AND ($1::text = '' OR s.search @@ websearch_to_tsquery('spanish', $1)
				OR s.title % $1 OR $1 <% s.title)
			AND (coalesce(cardinality($2::smallint[]), 0) = 0
				OR $2::smallint[] <@ ARRAY(SELECT sg.genre_id FROM story_genres sg WHERE sg.story_id = s.id))
			AND ($5::text = '' OR s.kind = $5)
		ORDER BY
			CASE WHEN $1 = '' THEN 0
				ELSE ts_rank(s.search, websearch_to_tsquery('spanish', $1)) + word_similarity($1, s.title)
			END DESC,
			s.last_chapter_at DESC NULLS LAST,
			s.id DESC
		LIMIT $3 OFFSET $4`

	rows, err := r.db.Query(ctx, q, query, genreIDs, limit, offset, kind)
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

// ListPublishedByAuthor devuelve las historias del autor que no son borradores, para su
// perfil público, empezando por las actualizadas más recientemente.
func (r *StoryRepository) ListPublishedByAuthor(ctx context.Context, authorID int64) ([]Story, error) {
	const q = `SELECT ` + storyColumns + ` FROM stories s` + storyJoins + `
		WHERE s.author_id = $1 AND s.status <> 'draft'
		ORDER BY s.last_chapter_at DESC NULLS LAST, s.id DESC`

	rows, err := r.db.Query(ctx, q, authorID)
	if err != nil {
		return nil, fmt.Errorf("listar historias publicadas del autor: %w", err)
	}
	stories, err := collectStories(rows)
	if err != nil {
		return nil, fmt.Errorf("listar historias publicadas del autor: %w", err)
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
