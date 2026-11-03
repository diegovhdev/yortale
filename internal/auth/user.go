package auth

import (
	"net/url"
	"time"
)

// User es un usuario tal como lo ve el resto de la aplicación.
// Nunca incluye el hash de la contraseña.
type User struct {
	ID          int64
	Username    string
	Email       string
	DisplayName *string // puede ser NULL en la base de datos
	Bio         string  // descripción del perfil; "" si no tiene
	Role        string  // RoleUser, RoleModerator o RoleAdmin
	// Suspended dice si la cuenta tiene una suspensión vigente al leerla. SuspendedUntil es
	// nil si es indefinida; los dos campos de abajo solo tienen sentido con Suspended.
	Suspended        bool
	SuspendedUntil   *time.Time
	SuspensionReason string
	CreatedAt        time.Time
}

// Deben coincidir con el CHECK de users.role.
const (
	RoleUser      = "user"
	RoleModerator = "moderator"
	RoleAdmin     = "admin"
)

// IsStaff dice si el usuario es del equipo de moderación (moderador o administrador).
func (u User) IsStaff() bool { return u.Role == RoleModerator || u.Role == RoleAdmin }

func (u User) IsAdmin() bool { return u.Role == RoleAdmin }

// RoleLabel es el rol en texto.
func (u User) RoleLabel() string { return RoleLabel(u.Role) }

func RoleLabel(role string) string {
	switch role {
	case RoleModerator:
		return "Moderador"
	case RoleAdmin:
		return "Administrador"
	default:
		return "Usuario"
	}
}

// SuspensionText describe hasta cuándo dura la suspensión: "hasta el 04/11/2026 a las 18:30"
// o "de forma indefinida".
func (u User) SuspensionText() string {
	if u.SuspendedUntil == nil {
		return "de forma indefinida"
	}
	return "hasta el " + u.SuspendedUntil.Local().Format("02/01/2006 a las 15:04")
}

// Name devuelve el nombre para mostrar, o el username si no tiene uno.
func (u User) Name() string {
	if u.DisplayName != nil && *u.DisplayName != "" {
		return *u.DisplayName
	}
	return u.Username
}

// ProfileURL es la dirección del perfil público: /users/{username}.
func (u User) ProfileURL() string { return ProfileURL(u.Username) }

// ProfileURL construye la dirección del perfil público de un username. La página la sirve
// el paquete stories, porque muestra las historias del autor.
func ProfileURL(username string) string { return "/users/" + url.PathEscape(username) }
