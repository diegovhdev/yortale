package stories

import (
	"context"
	"fmt"

	"github.com/diegovhdev/yortale/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Nombres que PostgreSQL da a la clave primaria y al UNIQUE de languages; renombrarlos rompe
// la traducción a ErrLanguageExists y ErrLanguageNameTaken.
const (
	languageCodeConstraint = "languages_pkey"
	languageNameConstraint = "languages_name_key"
)

type LanguageRepository struct {
	db *pgxpool.Pool
}

func NewLanguageRepository(db *pgxpool.Pool) *LanguageRepository {
	return &LanguageRepository{db: db}
}

// List devuelve todos los idiomas, activos o no, por nombre.
func (r *LanguageRepository) List(ctx context.Context) ([]Language, error) {
	rows, err := r.db.Query(ctx, `SELECT code, name, active FROM languages ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("listar idiomas: %w", err)
	}
	langs, err := pgx.CollectRows(rows, pgx.RowToStructByPos[Language])
	if err != nil {
		return nil, fmt.Errorf("listar idiomas: %w", err)
	}
	return langs, nil
}

func (r *LanguageRepository) Create(ctx context.Context, l Language) error {
	_, err := r.db.Exec(ctx, `INSERT INTO languages (code, name, active) VALUES ($1, $2, $3)`,
		l.Code, l.Name, l.Active)
	return languageError(err, "crear idioma")
}

// Update cambia el nombre y el estado del idioma; el código no cambia nunca.
func (r *LanguageRepository) Update(ctx context.Context, l Language) error {
	tag, err := r.db.Exec(ctx, `UPDATE languages SET name = $2, active = $3 WHERE code = $1`,
		l.Code, l.Name, l.Active)
	if err != nil {
		return languageError(err, "actualizar idioma")
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func languageError(err error, msg string) error {
	switch {
	case err == nil:
		return nil
	case database.UniqueViolation(err) == languageCodeConstraint:
		return ErrLanguageExists
	case database.UniqueViolation(err) == languageNameConstraint:
		return ErrLanguageNameTaken
	}
	return fmt.Errorf("%s: %w", msg, err)
}
