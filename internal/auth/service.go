// Package auth contiene todo lo relacionado con cuentas y sesiones: registro, inicio de sesión,
// cookies, y las consultas SQL de las tablas users y sessions.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/mail"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
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

const (
	// maxBioRunes debe coincidir con el CHECK (char_length(bio) <= 1000) de users.bio.
	maxBioRunes = 1000
	// display_name no tiene CHECK en la base de datos; este límite es la única validación.
	maxDisplayNameRunes = 50
)

// ValidationErrors asocia el nombre de cada campo del formulario con su mensaje de error.
type ValidationErrors map[string]string

func (v ValidationErrors) Error() string { return "datos de formulario inválidos" }

type userStore interface {
	Create(ctx context.Context, p CreateUserParams) (User, error)
	GetWithPasswordByLogin(ctx context.Context, login string) (User, string, error)
	GetByUsername(ctx context.Context, username string) (User, error)
	UpdateProfile(ctx context.Context, id int64, p ProfileParams) (User, error)
}

type sessionStore interface {
	Create(ctx context.Context, tokenHash string, userID int64, expiresAt time.Time) error
	GetUser(ctx context.Context, tokenHash string) (User, error)
	Delete(ctx context.Context, tokenHash string) error
	DeleteExpired(ctx context.Context) (int64, error)
}

type Service struct {
	users    userStore
	sessions sessionStore
	// Hash de una contraseña falsa. Se usa cuando el usuario no existe para que
	// el login tarde lo mismo y no revele qué correos están registrados.
	dummyHash string
}

func NewService(users userStore, sessions sessionStore) *Service {
	return &Service{users: users, sessions: sessions, dummyHash: HashPassword("contraseña-que-nadie-usa")}
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
func (s *Service) Register(ctx context.Context, in RegisterInput) (User, error) {
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
		errs["accept_terms"] = "Debes aceptar los términos de uso y la política de privacidad."
	}
	if len(errs) > 0 {
		return User{}, errs
	}

	user, err := s.users.Create(ctx, CreateUserParams{
		Username:     in.Username,
		Email:        in.Email,
		PasswordHash: HashPassword(in.Password),
		TermsVersion: CurrentTermsVersion,
	})
	switch {
	case errors.Is(err, ErrUsernameTaken):
		return User{}, ValidationErrors{"username": "Ese nombre de usuario ya está en uso."}
	case errors.Is(err, ErrEmailTaken):
		return User{}, ValidationErrors{"email": "Ya existe una cuenta con ese correo."}
	case err != nil:
		return User{}, err
	}
	return user, nil
}

// Authenticate comprueba usuario/correo y contraseña.
// Siempre devuelve el mismo error para no revelar si la cuenta existe.
func (s *Service) Authenticate(ctx context.Context, login, password string) (User, error) {
	login = strings.TrimSpace(login)
	if login == "" || password == "" {
		return User{}, ErrInvalidCredentials
	}

	user, hash, err := s.users.GetWithPasswordByLogin(ctx, login)
	if errors.Is(err, ErrNotFound) {
		_, _ = VerifyPassword(password, s.dummyHash) // igualar el tiempo de respuesta
		return User{}, ErrInvalidCredentials
	}
	if err != nil {
		return User{}, err
	}

	ok, err := VerifyPassword(password, hash)
	if err != nil {
		return User{}, fmt.Errorf("verificar contraseña: %w", err)
	}
	if !ok {
		return User{}, ErrInvalidCredentials
	}
	return user, nil
}

// ---------- Perfil ----------

// UserByUsername devuelve el usuario con ese username (sin distinguir mayúsculas), o ErrNotFound.
func (s *Service) UserByUsername(ctx context.Context, username string) (User, error) {
	return s.users.GetByUsername(ctx, username)
}

// ProfileInput son los valores del formulario de perfil, tal como llegan.
type ProfileInput struct {
	DisplayName string
	Bio         string
}

// UpdateProfile valida y guarda el nombre para mostrar y la descripción del usuario.
// Devuelve ValidationErrors si hay errores.
func (s *Service) UpdateProfile(ctx context.Context, userID int64, in ProfileInput) (User, error) {
	p := ProfileParams{
		DisplayName: strings.Join(strings.Fields(in.DisplayName), " "), // sin saltos ni espacios repetidos
		Bio:         strings.TrimSpace(strings.ReplaceAll(in.Bio, "\r\n", "\n")),
	}
	errs := ValidationErrors{}
	if utf8.RuneCountInString(p.DisplayName) > maxDisplayNameRunes {
		errs["display_name"] = fmt.Sprintf("El nombre no puede tener más de %d caracteres.", maxDisplayNameRunes)
	}
	if utf8.RuneCountInString(p.Bio) > maxBioRunes {
		errs["bio"] = fmt.Sprintf("La descripción no puede tener más de %d caracteres.", maxBioRunes)
	}
	if len(errs) > 0 {
		return User{}, errs
	}
	return s.users.UpdateProfile(ctx, userID, p)
}

// ---------- Sesiones ----------

// StartSession crea una sesión y devuelve el token que irá en la cookie.
func (s *Service) StartSession(ctx context.Context, userID int64) (string, time.Time, error) {
	token := rand.Text() // al menos 128 bits aleatorios, en base32
	expires := time.Now().Add(SessionTTL)

	if err := s.sessions.Create(ctx, hashToken(token), userID, expires); err != nil {
		return "", time.Time{}, err
	}
	return token, expires, nil
}

// UserFromSession devuelve el usuario dueño del token, o ErrNoSession.
func (s *Service) UserFromSession(ctx context.Context, token string) (User, error) {
	user, err := s.sessions.GetUser(ctx, hashToken(token))
	if errors.Is(err, ErrNotFound) {
		return User{}, ErrNoSession
	}
	return user, err
}

func (s *Service) EndSession(ctx context.Context, token string) error {
	return s.sessions.Delete(ctx, hashToken(token))
}

func (s *Service) CleanupExpiredSessions(ctx context.Context) (int64, error) {
	return s.sessions.DeleteExpired(ctx)
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
