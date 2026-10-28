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
// Los géneros y los tags llegan como arrays JSON ordenados por posición; encoding/json no
// distingue mayúsculas en los nombres de campo, así que "id" llena Genre.ID.
const storyColumns = `s.id, s.author_id, u.username::text, coalesce(nullif(u.display_name, ''), u.username::text),
	(SELECT coalesce(json_agg(json_build_object('id', g.id, 'name', g.name, 'slug', g.slug) ORDER BY sg.position), '[]')
		FROM story_genres sg JOIN genres g ON g.id = sg.genre_id WHERE sg.story_id = s.id),
	(SELECT coalesce(json_agg(json_build_object('name', t.name, 'slug', t.slug) ORDER BY st.position), '[]')
		FROM story_tags st JOIN tags t ON t.id = st.tag_id WHERE st.story_id = s.id),
	s.kind, s.title, s.slug, coalesce(s.synopsis, ''), s.language, s.content_rating, s.warnings, s.status,
	s.published_at, s.last_chapter_at, s.reads_count,
	(SELECT count(*) FROM chapters c WHERE c.story_id = s.id AND c.is_published),
	s.created_at, s.updated_at`

// storyJoins une la historia (alias s) con su autor.
const storyJoins = `
		JOIN users u ON u.id = s.author_id`

func scanStory(row pgx.Row) (Story, error) {
	var s Story
	err := row.Scan(&s.ID, &s.AuthorID, &s.AuthorUsername, &s.AuthorName,
		&s.Genres, &s.Tags, &s.Kind, &s.Title, &s.Slug, &s.Synopsis, &s.Language, &s.ContentRating, &s.Warnings, &s.Status,
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
	Tags          []Tag   // en el orden en que se muestran, sin slugs repetidos
	Title         string
	Slug          string
	Synopsis      string
	Kind          string
	Language      string
	ContentRating string
	Warnings      []string
	Status        string
}

// slugConstraint es el nombre que PostgreSQL genera para UNIQUE (author_id, slug).
const slugConstraint = "stories_author_id_slug_key"

// Create inserta una historia con sus géneros y tags. Devuelve ErrSlugTaken si el autor ya usa ese slug.
func (r *StoryRepository) Create(ctx context.Context, p StoryParams) (Story, error) {
	const q = `
		INSERT INTO stories (author_id, title, slug, synopsis, language, content_rating, status, kind, warnings, published_at)
		VALUES ($1, $2, $3, nullif($4, ''), $5, $6, $7, $8, $9, CASE WHEN $7 <> 'draft' THEN now() END)
		RETURNING id`

	var s Story
	err := pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		var id int64
		err := tx.QueryRow(ctx, q,
			p.AuthorID, p.Title, p.Slug, p.Synopsis, p.Language, p.ContentRating, p.Status, p.Kind, p.Warnings).Scan(&id)
		if err != nil {
			return err
		}
		s, err = saveRelations(ctx, tx, id, p)
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

// Update modifica la historia id, sus géneros y sus tags si pertenece a p.AuthorID; si no, devuelve ErrNotFound.
// published_at se fija la primera vez que la historia deja de ser borrador.
func (r *StoryRepository) Update(ctx context.Context, id int64, p StoryParams) (Story, error) {
	const q = `
		UPDATE stories SET
			title = $3, slug = $4, synopsis = nullif($5, ''),
			language = $6, content_rating = $7, status = $8, kind = $9, warnings = $10,
			published_at = CASE WHEN $8 <> 'draft' THEN coalesce(published_at, now()) ELSE published_at END
		WHERE id = $1 AND author_id = $2`

	var s Story
	err := pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, q, id, p.AuthorID,
			p.Title, p.Slug, p.Synopsis, p.Language, p.ContentRating, p.Status, p.Kind, p.Warnings)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		s, err = saveRelations(ctx, tx, id, p)
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

// saveRelations reemplaza los géneros y los tags de la historia, dentro de la transacción de
// Create o Update, y devuelve la historia ya guardada.
func saveRelations(ctx context.Context, tx pgx.Tx, storyID int64, p StoryParams) (Story, error) {
	if err := saveGenres(ctx, tx, storyID, p.GenreIDs); err != nil {
		return Story{}, err
	}
	if err := saveTags(ctx, tx, storyID, p.Tags); err != nil {
		return Story{}, err
	}
	return getStory(ctx, tx, storyID)
}

// saveGenres reemplaza los géneros de la historia. La posición sale del orden de genreIDs (1, 2, 3).
func saveGenres(ctx context.Context, tx pgx.Tx, storyID int64, genreIDs []int16) error {
	if _, err := tx.Exec(ctx, `DELETE FROM story_genres WHERE story_id = $1`, storyID); err != nil {
		return err
	}
	const q = `
		INSERT INTO story_genres (story_id, genre_id, position)
		SELECT $1, t.genre_id, t.position
		FROM unnest($2::smallint[]) WITH ORDINALITY AS t(genre_id, position)`
	_, err := tx.Exec(ctx, q, storyID, genreIDs)
	return err
}

// saveTags reemplaza los tags de la historia, en el orden de tags. Los que no existen se crean
// y los que ya existen (mismo slug) se reutilizan sin cambiar su nombre: ON CONFLICT lo
// resuelve en la misma consulta, así que dos autores que crean el mismo tag a la vez no chocan.
// DO UPDATE en lugar de DO NOTHING es para que RETURNING devuelva también los que ya existían.
// tags no puede traer slugs repetidos: ON CONFLICT no puede tocar la misma fila dos veces.
func saveTags(ctx context.Context, tx pgx.Tx, storyID int64, tags []Tag) error {
	if _, err := tx.Exec(ctx, `DELETE FROM story_tags WHERE story_id = $1`, storyID); err != nil {
		return err
	}
	if len(tags) == 0 {
		return nil
	}
	names := make([]string, len(tags))
	slugs := make([]string, len(tags))
	for i, t := range tags {
		names[i], slugs[i] = t.Name, t.Slug
	}
	const q = `
		WITH input AS (
			SELECT * FROM unnest($2::text[], $3::text[]) WITH ORDINALITY AS t(name, slug, position)
		), saved AS (
			INSERT INTO tags (name, slug) SELECT name, slug FROM input
			ON CONFLICT (slug) DO UPDATE SET name = tags.name
			RETURNING id, slug
		)
		INSERT INTO story_tags (story_id, tag_id, position)
		SELECT $1, saved.id, input.position FROM saved JOIN input USING (slug)`
	_, err := tx.Exec(ctx, q, storyID, names, slugs)
	return err
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
// Con f.GenreIDs o f.Tags, la historia debe tenerlos todos (en cualquier posición); con
// f.ExcludeWarnings o f.ExcludeTags, no puede tener ninguno. Los campos vacíos no filtran.
func (r *StoryRepository) ListPublished(ctx context.Context, f ExploreFilter, limit, offset int) ([]Story, error) {
	const q = `
		SELECT ` + storyColumns + ` FROM stories s` + storyJoins + `
		WHERE s.status <> 'draft'
			AND ($1::text = '' OR s.search @@ websearch_to_tsquery('spanish', $1)
				OR s.title % $1 OR $1 <% s.title)
			AND (coalesce(cardinality($2::smallint[]), 0) = 0
				OR $2::smallint[] <@ ARRAY(SELECT sg.genre_id FROM story_genres sg WHERE sg.story_id = s.id))
			AND ($5::text = '' OR s.kind = $5)
			AND (coalesce(cardinality($6::text[]), 0) = 0 OR NOT s.warnings && $6::text[])
			AND (coalesce(cardinality($7::text[]), 0) = 0
				OR $7::text[] <@ ARRAY(SELECT t.slug FROM story_tags st JOIN tags t ON t.id = st.tag_id
					WHERE st.story_id = s.id))
			AND NOT EXISTS (SELECT 1 FROM story_tags st JOIN tags t ON t.id = st.tag_id
				WHERE st.story_id = s.id AND t.slug = ANY ($8::text[]))
		ORDER BY
			CASE WHEN $1 = '' THEN 0
				ELSE ts_rank(s.search, websearch_to_tsquery('spanish', $1)) + word_similarity($1, s.title)
			END DESC,
			s.last_chapter_at DESC NULLS LAST,
			s.id DESC
		LIMIT $3 OFFSET $4`

	rows, err := r.db.Query(ctx, q, f.Query, f.GenreIDs, limit, offset, f.Kind, f.ExcludeWarnings, f.Tags, f.ExcludeTags)
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

// GetTag busca un tag por su slug y cuenta sus historias publicadas.
func (r *StoryRepository) GetTag(ctx context.Context, slug string) (Tag, error) {
	const q = `
		SELECT t.name, t.slug,
			(SELECT count(*) FROM story_tags st JOIN stories s ON s.id = st.story_id
				WHERE st.tag_id = t.id AND s.status <> 'draft')
		FROM tags t WHERE t.slug = $1`
	var t Tag
	err := r.db.QueryRow(ctx, q, slug).Scan(&t.Name, &t.Slug, &t.Uses)
	if errors.Is(err, pgx.ErrNoRows) {
		return Tag{}, ErrNotFound
	}
	if err != nil {
		return Tag{}, fmt.Errorf("buscar tag: %w", err)
	}
	return t, nil
}

// SuggestTags busca tags cuyo slug empieza por slug o se le parece (pg_trgm, para tolerar
// errores de escritura). Primero los que empiezan igual y, dentro de cada grupo, los más
// usados en historias publicadas. El slug solo tiene letras, números y guiones, así que no
// hace falta escapar los comodines de LIKE.
func (r *StoryRepository) SuggestTags(ctx context.Context, slug string, limit int) ([]Tag, error) {
	const q = `
		SELECT t.name, t.slug, count(s.id)
		FROM tags t
			LEFT JOIN story_tags st ON st.tag_id = t.id
			LEFT JOIN stories s ON s.id = st.story_id AND s.status <> 'draft'
		WHERE t.slug LIKE $1 || '%' OR t.slug % $1
		GROUP BY t.id
		ORDER BY t.slug LIKE $1 || '%' DESC, count(s.id) DESC, t.name
		LIMIT $2`
	rows, err := r.db.Query(ctx, q, slug, limit)
	if err != nil {
		return nil, fmt.Errorf("sugerir tags: %w", err)
	}
	tags, err := pgx.CollectRows(rows, pgx.RowToStructByPos[Tag])
	if err != nil {
		return nil, fmt.Errorf("sugerir tags: %w", err)
	}
	return tags, nil
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
