// Package service contiene la lógica de negocio: validar, decidir y coordinar repositorios.
package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/mail"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/diegovhdev/yortale/internal/models"
	"github.com/diegovhdev/yortale/internal/repository"
)

// CurrentTermsVersion es la versión vigente de los términos de uso.
// Cuando cambies los términos, sube este valor (v2, v3...).
const CurrentTermsVersion = "v1"

const SessionTTL = 30 * 24 * time.Hour

var (
	ErrInvalidCredentials = errors.New("credenciales inválidas")
	ErrNoSession          = errors.New("sesión inválida o expirada")

	// Debe coincidir con el CHECK de la columna users.username.
	usernamePattern = regexp.MustCompile(`^[A-Za-z0-9_]{3,30}$`)
)

// ValidationErrors asocia el nombre de cada campo del formulario con su mensaje de error.
type ValidationErrors map[string]string

func (v ValidationErrors) Error() string { return "datos de formulario inválidos" }

type userStore interface {
	Create(ctx context.Context, p repository.CreateUserParams) (models.User, error)
	GetWithPasswordByLogin(ctx context.Context, login string) (models.User, string, error)
}

type sessionStore interface {
	Create(ctx context.Context, tokenHash string, userID int64, expiresAt time.Time) error
	GetUser(ctx context.Context, tokenHash string) (models.User, error)
	Delete(ctx context.Context, tokenHash string) error
	DeleteExpired(ctx context.Context) (int64, error)
}

type AuthService struct {
	users    userStore
	sessions sessionStore
	// Hash de una contraseña falsa. Se usa cuando el usuario no existe para que
	// el login tarde lo mismo y no revele qué correos están registrados.
	dummyHash string
}

func NewAuthService(users userStore, sessions sessionStore) *AuthService {
	dummy, _ := HashPassword("contraseña-que-nadie-usa")
	return &AuthService{users: users, sessions: sessions, dummyHash: dummy}
}

type RegisterInput struct {
	Username        string
	Email           string
	Password        string
	PasswordConfirm string
	AcceptTerms     bool
}

// Register valida los datos, crea el usuario y lo devuelve.
// Si hay problemas con los datos devuelve ValidationErrors.
func (s *AuthService) Register(ctx context.Context, in RegisterInput) (models.User, error) {
	in.Username = strings.TrimSpace(in.Username)
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))

	errs := ValidationErrors{}
	if !usernamePattern.MatchString(in.Username) {
		errs["username"] = "Usa entre 3 y 30 caracteres: letras, números o guion bajo."
	}
	if addr, err := mail.ParseAddress(in.Email); err != nil || addr.Address != in.Email {
		errs["email"] = "Escribe un correo válido."
	}
	switch n := utf8.RuneCountInString(in.Password); {
	case n < 8:
		errs["password"] = "La contraseña debe tener al menos 8 caracteres."
	case n > 128:
		errs["password"] = "La contraseña no puede tener más de 128 caracteres."
	}
	if in.Password != in.PasswordConfirm {
		errs["password_confirm"] = "Las contraseñas no coinciden."
	}
	if !in.AcceptTerms {
		errs["accept_terms"] = "Debes aceptar los términos de uso."
	}
	if len(errs) > 0 {
		return models.User{}, errs
	}

	hash, err := HashPassword(in.Password)
	if err != nil {
		return models.User{}, fmt.Errorf("hashear contraseña: %w", err)
	}

	user, err := s.users.Create(ctx, repository.CreateUserParams{
		Username:     in.Username,
		Email:        in.Email,
		PasswordHash: hash,
		TermsVersion: CurrentTermsVersion,
	})
	switch {
	case errors.Is(err, repository.ErrUsernameTaken):
		return models.User{}, ValidationErrors{"username": "Ese nombre de usuario ya está en uso."}
	case errors.Is(err, repository.ErrEmailTaken):
		return models.User{}, ValidationErrors{"email": "Ya existe una cuenta con ese correo."}
	case err != nil:
		return models.User{}, err
	}
	return user, nil
}

// Authenticate comprueba usuario/correo y contraseña.
// Siempre devuelve el mismo error para no revelar si la cuenta existe.
func (s *AuthService) Authenticate(ctx context.Context, login, password string) (models.User, error) {
	login = strings.TrimSpace(login)
	if login == "" || password == "" {
		return models.User{}, ErrInvalidCredentials
	}

	user, hash, err := s.users.GetWithPasswordByLogin(ctx, login)
	if errors.Is(err, repository.ErrNotFound) {
		_, _ = VerifyPassword(password, s.dummyHash) // igualar el tiempo de respuesta
		return models.User{}, ErrInvalidCredentials
	}
	if err != nil {
		return models.User{}, err
	}

	ok, err := VerifyPassword(password, hash)
	if err != nil {
		return models.User{}, fmt.Errorf("verificar contraseña: %w", err)
	}
	if !ok {
		return models.User{}, ErrInvalidCredentials
	}
	return user, nil
}

// StartSession crea una sesión y devuelve el token que irá en la cookie.
func (s *AuthService) StartSession(ctx context.Context, userID int64) (string, time.Time, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", time.Time{}, err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	expires := time.Now().Add(SessionTTL)

	if err := s.sessions.Create(ctx, hashToken(token), userID, expires); err != nil {
		return "", time.Time{}, err
	}
	return token, expires, nil
}

// UserFromSession devuelve el usuario dueño del token, o ErrNoSession.
func (s *AuthService) UserFromSession(ctx context.Context, token string) (models.User, error) {
	user, err := s.sessions.GetUser(ctx, hashToken(token))
	if errors.Is(err, repository.ErrNotFound) {
		return models.User{}, ErrNoSession
	}
	return user, err
}

func (s *AuthService) EndSession(ctx context.Context, token string) error {
	return s.sessions.Delete(ctx, hashToken(token))
}

func (s *AuthService) CleanupExpiredSessions(ctx context.Context) (int64, error) {
	return s.sessions.DeleteExpired(ctx)
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
