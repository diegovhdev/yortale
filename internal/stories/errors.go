package stories

import "errors"

var (
	// ErrNotFound también se usa cuando la historia existe pero el usuario no puede verla
	// (un borrador ajeno) o no puede editarla: así no se revela que existe.
	ErrNotFound  = errors.New("historia o capítulo no encontrado")
	ErrSlugTaken = errors.New("el autor ya tiene una historia con ese slug")

	// ErrMixedMove: mover un capítulo lo cruzaría con uno de otro estado (publicado o
	// borrador). Los publicados van siempre antes que los borradores.
	ErrMixedMove = errors.New("un capítulo publicado y un borrador no pueden intercambiarse")
	// ErrNotLastPublished: solo el último capítulo publicado puede volver a borrador, para no
	// dejar huecos en lo que ya leen los lectores.
	ErrNotLastPublished = errors.New("solo el último capítulo publicado puede volver a borrador")

	ErrLanguageExists    = errors.New("ya existe un idioma con ese código")
	ErrLanguageNameTaken = errors.New("ya existe un idioma con ese nombre")
	ErrGenreNameTaken    = errors.New("ya existe un género con ese nombre")
)
