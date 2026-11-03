package moderation

import "errors"

var (
	ErrNotFound = errors.New("historia, capítulo o cuenta no encontrados")
	// ErrNoChange: la acción no cambia nada (la historia ya estaba oculta, no había reportes
	// abiertos…). Pasa sobre todo si dos moderadores actúan a la vez.
	ErrNoChange = errors.New("no había nada que cambiar")
	// ErrForbidden: el rol de quien actúa no le permite hacerlo sobre esa cuenta.
	ErrForbidden = errors.New("no tienes permiso para esta acción")
	// ErrOwnContent: nadie puede reportar lo suyo.
	ErrOwnContent = errors.New("no puedes reportar tu propio contenido")
)
