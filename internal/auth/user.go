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
	Role        string
	CreatedAt   time.Time
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
