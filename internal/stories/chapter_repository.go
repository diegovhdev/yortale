package stories

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ChapterRepository struct {
	db *pgxpool.Pool
}

func NewChapterRepository(db *pgxpool.Pool) *ChapterRepository {
	return &ChapterRepository{db: db}
}

const chapterColumns = `c.id, c.story_id, c.position, c.title, c.content, c.word_count,
	c.is_published, c.published_at, c.created_at, c.updated_at`

// chapterListColumns no carga el texto, que puede ser largo y en un índice no hace falta.
const chapterListColumns = `c.id, c.story_id, c.position, c.title, ''::text, c.word_count,
	c.is_published, c.published_at, c.created_at, c.updated_at`

func scanChapter(row pgx.Row) (Chapter, error) {
	var c Chapter
	err := row.Scan(&c.ID, &c.StoryID, &c.Position, &c.Title, &c.Content, &c.WordCount,
		&c.IsPublished, &c.PublishedAt, &c.CreatedAt, &c.UpdatedAt)
	return c, err
}

// ListByStory devuelve los capítulos en orden, sin su texto. Con includeDrafts incluye los no publicados.
func (r *ChapterRepository) ListByStory(ctx context.Context, storyID int64, includeDrafts bool) ([]Chapter, error) {
	const q = `SELECT ` + chapterListColumns + `
		FROM chapters c
		WHERE c.story_id = $1 AND (c.is_published OR $2)
		ORDER BY c.position`

	rows, err := r.db.Query(ctx, q, storyID, includeDrafts)
	if err != nil {
		return nil, fmt.Errorf("listar capítulos: %w", err)
	}
	chapters, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Chapter, error) { return scanChapter(row) })
	if err != nil {
		return nil, fmt.Errorf("listar capítulos: %w", err)
	}
	return chapters, nil
}

// ListForReading es como ListByStory, pero carga el texto del capítulo chapterID (solo el de ese),
// para leer un capítulo y tener sus vecinos con una sola consulta.
func (r *ChapterRepository) ListForReading(ctx context.Context, storyID, chapterID int64, includeDrafts bool) ([]Chapter, error) {
	const q = `SELECT c.id, c.story_id, c.position, c.title,
			CASE WHEN c.id = $3 THEN c.content ELSE '' END, c.word_count,
			c.is_published, c.published_at, c.created_at, c.updated_at
		FROM chapters c
		WHERE c.story_id = $1 AND (c.is_published OR $2)
		ORDER BY c.position`

	rows, err := r.db.Query(ctx, q, storyID, includeDrafts, chapterID)
	if err != nil {
		return nil, fmt.Errorf("listar capítulos para leer: %w", err)
	}
	chapters, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Chapter, error) { return scanChapter(row) })
	if err != nil {
		return nil, fmt.Errorf("listar capítulos para leer: %w", err)
	}
	return chapters, nil
}

func (r *ChapterRepository) Get(ctx context.Context, id int64) (Chapter, error) {
	c, err := scanChapter(r.db.QueryRow(ctx, `SELECT `+chapterColumns+` FROM chapters c WHERE c.id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Chapter{}, ErrNotFound
	}
	if err != nil {
		return Chapter{}, fmt.Errorf("buscar capítulo: %w", err)
	}
	return c, nil
}

type ChapterParams struct {
	Title     string
	Content   string
	WordCount int
	Publish   bool
}

// Create añade un capítulo al final de la historia. Si se publica, actualiza la historia
// (ver touchStory) en la misma transacción.
func (r *ChapterRepository) Create(ctx context.Context, storyID int64, p ChapterParams) (Chapter, error) {
	var c Chapter
	err := pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		// Bloquear la historia evita que dos capítulos creados a la vez reciban la misma posición.
		if _, err := tx.Exec(ctx, `SELECT 1 FROM stories WHERE id = $1 FOR UPDATE`, storyID); err != nil {
			return err
		}
		const q = `
			INSERT INTO chapters AS c (story_id, position, title, content, word_count, is_published, published_at)
			SELECT $1::bigint, coalesce(max(position), 0) + 1, $2::text, $3::text, $4::int, $5::bool,
				CASE WHEN $5::bool THEN now() END
			FROM chapters WHERE story_id = $1
			RETURNING ` + chapterColumns

		var err error
		c, err = scanChapter(tx.QueryRow(ctx, q, storyID, p.Title, p.Content, p.WordCount, p.Publish))
		if err != nil {
			return err
		}
		if p.Publish {
			return touchStory(ctx, tx, storyID)
		}
		return nil
	})
	if err != nil {
		return Chapter{}, fmt.Errorf("crear capítulo: %w", err)
	}
	return c, nil
}

// Update modifica el capítulo. published_at guarda la primera publicación y no cambia
// si el capítulo se despublica y se vuelve a publicar.
func (r *ChapterRepository) Update(ctx context.Context, id int64, p ChapterParams) (Chapter, error) {
	var c Chapter
	err := pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		var wasPublished bool
		err := tx.QueryRow(ctx, `SELECT is_published FROM chapters WHERE id = $1 FOR UPDATE`, id).Scan(&wasPublished)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}

		const q = `
			UPDATE chapters c SET
				title = $2, content = $3, word_count = $4, is_published = $5::bool,
				published_at = CASE WHEN $5::bool THEN coalesce(published_at, now()) ELSE published_at END
			WHERE id = $1
			RETURNING ` + chapterColumns
		c, err = scanChapter(tx.QueryRow(ctx, q, id, p.Title, p.Content, p.WordCount, p.Publish))
		if err != nil {
			return err
		}
		if p.Publish && !wasPublished {
			return touchStory(ctx, tx, c.StoryID)
		}
		return nil
	})
	if errors.Is(err, ErrNotFound) {
		return Chapter{}, err
	}
	if err != nil {
		return Chapter{}, fmt.Errorf("actualizar capítulo: %w", err)
	}
	return c, nil
}

// touchStory se llama al publicar un capítulo: actualiza last_chapter_at (desnormalizado,
// para "actualizadas recientemente") y, si la historia era un borrador, la publica como "en curso".
func touchStory(ctx context.Context, tx pgx.Tx, storyID int64) error {
	const q = `
		UPDATE stories SET
			last_chapter_at = now(),
			published_at = coalesce(published_at, now()),
			status = CASE WHEN status = 'draft' THEN 'ongoing' ELSE status END
		WHERE id = $1`
	_, err := tx.Exec(ctx, q, storyID)
	return err
}

// Delete borra el capítulo y sube una posición los que iban detrás, para no dejar huecos.
func (r *ChapterRepository) Delete(ctx context.Context, id int64) error {
	err := pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		var storyID int64
		var position int
		err := tx.QueryRow(ctx, `DELETE FROM chapters WHERE id = $1 RETURNING story_id, position`, id).
			Scan(&storyID, &position)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `SET CONSTRAINTS chapters_story_position_key DEFERRED`); err != nil {
			return err
		}
		_, err = tx.Exec(ctx,
			`UPDATE chapters SET position = position - 1 WHERE story_id = $1 AND position > $2`,
			storyID, position)
		return err
	})
	if errors.Is(err, ErrNotFound) {
		return err
	}
	if err != nil {
		return fmt.Errorf("borrar capítulo: %w", err)
	}
	return nil
}

// Move intercambia el capítulo con el anterior (delta = -1) o el siguiente (delta = 1).
// Si no hay vecino en esa dirección no hace nada. La restricción chapters_story_position_key
// es DEFERRABLE: se comprueba al terminar la transacción, cuando ya no hay posiciones repetidas.
func (r *ChapterRepository) Move(ctx context.Context, id int64, delta int) error {
	err := pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		var storyID int64
		var position int
		err := tx.QueryRow(ctx, `SELECT story_id, position FROM chapters WHERE id = $1 FOR UPDATE`, id).
			Scan(&storyID, &position)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}

		target := position + delta
		var neighborID int64
		err = tx.QueryRow(ctx, `SELECT id FROM chapters WHERE story_id = $1 AND position = $2 FOR UPDATE`,
			storyID, target).Scan(&neighborID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil // ya es el primero o el último
		}
		if err != nil {
			return err
		}

		if _, err := tx.Exec(ctx, `SET CONSTRAINTS chapters_story_position_key DEFERRED`); err != nil {
			return err
		}
		_, err = tx.Exec(ctx,
			`UPDATE chapters SET position = CASE WHEN id = $1 THEN $3::int ELSE $4::int END WHERE id IN ($1, $2)`,
			id, neighborID, target, position)
		return err
	})
	if errors.Is(err, ErrNotFound) {
		return err
	}
	if err != nil {
		return fmt.Errorf("mover capítulo: %w", err)
	}
	return nil
}
