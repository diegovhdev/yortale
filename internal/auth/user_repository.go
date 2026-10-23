package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/diegovhdev/yortale/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type UserRepository struct {
	db *pgxpool.Pool
}

func NewUserRepository(db *pgxpool.Pool) *UserRepository {
	return &UserRepository{db: db}
}

// Las columnas citext se convierten a text (::text) para leerlas como string en Go.
const userColumns = `u.id, u.username::text, u.email::text, u.display_name, coalesce(u.bio, ''), u.role, u.created_at`

func scanUser(row pgx.Row, extra ...any) (User, error) {
	var u User
	dest := append([]any{&u.ID, &u.Username, &u.Email, &u.DisplayName, &u.Bio, &u.Role, &u.CreatedAt}, extra...)
	err := row.Scan(dest...)
	return u, err
}

type CreateUserParams struct {
	Username     string
	Email        string
	PasswordHash string
	TermsVersion string
}

// Create inserta un usuario nuevo. Devuelve ErrUsernameTaken o ErrEmailTaken si ya existen.
func (r *UserRepository) Create(ctx context.Context, p CreateUserParams) (User, error) {
	const q = `
		INSERT INTO users AS u (username, email, password_hash, terms_version, terms_accepted_at)
		VALUES ($1, $2, $3, $4, now())
		RETURNING ` + userColumns

	user, err := scanUser(r.db.QueryRow(ctx, q, p.Username, p.Email, p.PasswordHash, p.TermsVersion))
	if err != nil {
		switch database.UniqueViolation(err) {
		case "users_username_key":
			return User{}, ErrUsernameTaken
		case "users_email_key":
			return User{}, ErrEmailTaken
		}
		return User{}, fmt.Errorf("crear usuario: %w", err)
	}
	return user, nil
}

// GetWithPasswordByLogin busca un usuario por username o email (sin distinguir mayúsculas,
// gracias a citext) y devuelve también el hash de su contraseña para verificarla.
func (r *UserRepository) GetWithPasswordByLogin(ctx context.Context, login string) (User, string, error) {
	const q = `
		SELECT ` + userColumns + `, u.password_hash
		FROM users u
		WHERE u.username = $1 OR u.email = $1
		LIMIT 1`

	var hash string
	user, err := scanUser(r.db.QueryRow(ctx, q, login), &hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, "", ErrNotFound
	}
	if err != nil {
		return User{}, "", fmt.Errorf("buscar usuario: %w", err)
	}
	return user, hash, nil
}

// GetByUsername busca un usuario por su username, sin distinguir mayúsculas (citext).
func (r *UserRepository) GetByUsername(ctx context.Context, username string) (User, error) {
	const q = `SELECT ` + userColumns + ` FROM users u WHERE u.username = $1`
	user, err := scanUser(r.db.QueryRow(ctx, q, username))
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("buscar usuario por username: %w", err)
	}
	return user, nil
}

// ProfileParams son los campos del perfil que el usuario puede editar. Los vacíos se guardan como NULL.
type ProfileParams struct {
	DisplayName string
	Bio         string
}

// UpdateProfile guarda el nombre para mostrar y la descripción del usuario id.
func (r *UserRepository) UpdateProfile(ctx context.Context, id int64, p ProfileParams) (User, error) {
	const q = `
		UPDATE users AS u SET display_name = nullif($2, ''), bio = nullif($3, '')
		WHERE u.id = $1
		RETURNING ` + userColumns
	user, err := scanUser(r.db.QueryRow(ctx, q, id, p.DisplayName, p.Bio))
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("actualizar perfil: %w", err)
	}
	return user, nil
}
