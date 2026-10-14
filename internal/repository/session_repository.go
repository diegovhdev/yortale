package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/diegovhdev/yortale/internal/models"
)

type SessionRepository struct {
	db *pgxpool.Pool
}

func NewSessionRepository(db *pgxpool.Pool) *SessionRepository {
	return &SessionRepository{db: db}
}

// Create guarda una sesión. Solo se guarda el hash del token, nunca el token en sí:
// si alguien robara la base de datos no podría usar esas sesiones.
func (r *SessionRepository) Create(ctx context.Context, tokenHash string, userID int64, expiresAt time.Time) error {
	const q = `INSERT INTO sessions (token_hash, user_id, expires_at) VALUES ($1, $2, $3)`
	if _, err := r.db.Exec(ctx, q, tokenHash, userID, expiresAt); err != nil {
		return fmt.Errorf("crear sesión: %w", err)
	}
	return nil
}

// GetUser devuelve el usuario dueño de una sesión que todavía no ha expirado.
func (r *SessionRepository) GetUser(ctx context.Context, tokenHash string) (models.User, error) {
	const q = `
		SELECT ` + userColumns + `
		FROM sessions s
		JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = $1 AND s.expires_at > now()`

	user, err := scanUser(r.db.QueryRow(ctx, q, tokenHash))
	if errors.Is(err, pgx.ErrNoRows) {
		return models.User{}, ErrNotFound
	}
	if err != nil {
		return models.User{}, fmt.Errorf("buscar sesión: %w", err)
	}
	return user, nil
}

func (r *SessionRepository) Delete(ctx context.Context, tokenHash string) error {
	if _, err := r.db.Exec(ctx, `DELETE FROM sessions WHERE token_hash = $1`, tokenHash); err != nil {
		return fmt.Errorf("borrar sesión: %w", err)
	}
	return nil
}

// DeleteExpired limpia las sesiones vencidas y devuelve cuántas borró.
func (r *SessionRepository) DeleteExpired(ctx context.Context) (int64, error) {
	tag, err := r.db.Exec(ctx, `DELETE FROM sessions WHERE expires_at <= now()`)
	if err != nil {
		return 0, fmt.Errorf("limpiar sesiones: %w", err)
	}
	return tag.RowsAffected(), nil
}
