package comments

import "errors"

var (
	// ErrNotFound: el comentario no existe, ya está borrado o quien lo pide no puede ver su capítulo.
	ErrNotFound = errors.New("comentario no encontrado")
	// ErrForbidden: quien lo pide puede verlo, pero no hacer esa acción.
	ErrForbidden = errors.New("no tienes permiso para esta acción")
)
