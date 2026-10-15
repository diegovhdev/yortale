package database

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

// UniqueViolation devuelve el nombre de la restricción UNIQUE violada, o "" si el error es otro.
// 23505 es el código de PostgreSQL para "unique_violation".
func UniqueViolation(err error) string {
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok && pgErr.Code == "23505" {
		return pgErr.ConstraintName
	}
	return ""
}
