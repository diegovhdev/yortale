package stories

import "errors"

var (
	// ErrNotFound también se usa cuando la historia existe pero el usuario no puede verla
	// (un borrador ajeno) o no puede editarla: así no se revela que existe.
	ErrNotFound  = errors.New("historia o capítulo no encontrado")
	ErrSlugTaken = errors.New("el autor ya tiene una historia con ese slug")
)
