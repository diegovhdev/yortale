package auth

import "errors"

var (
	ErrNotFound      = errors.New("registro no encontrado")
	ErrUsernameTaken = errors.New("el nombre de usuario ya existe")
	ErrEmailTaken    = errors.New("el correo ya está registrado")
)
