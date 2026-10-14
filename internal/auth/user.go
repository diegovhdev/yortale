package auth

import "time"

// User es un usuario tal como lo ve el resto de la aplicación.
// Nunca incluye el hash de la contraseña.
type User struct {
	ID          int64
	Username    string
	Email       string
	DisplayName *string // puede ser NULL en la base de datos
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
