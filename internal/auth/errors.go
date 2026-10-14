package auth

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

var (
	ErrNotFound      = errors.New("registro no encontrado")
	ErrUsernameTaken = errors.New("el nombre de usuario ya existe")
	ErrEmailTaken    = errors.New("el correo ya está registrado")
)

// uniqueViolation devuelve el nombre de la restricción UNIQUE violada, o "" si el error es otro.
// 23505 es el código de PostgreSQL para "unique_violation".
func uniqueViolation(err error) string {
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok && pgErr.Code == "23505" {
		return pgErr.ConstraintName
	}
	return ""
}
