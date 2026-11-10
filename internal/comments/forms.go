package comments

import "fmt"

// Form guarda lo que el lector escribió y los errores, para volver a mostrar el formulario.
// Sirve para comentar, responder y editar. En la página del capítulo hay varios a la vez
// (el principal y uno por hilo), así que cada uno lleva su propio ID.
type Form struct {
	ID        string // id del <form>, único en la página
	Action    string
	ParentID  int64 // al responder
	CommentID int64 // al editar
	Body      string
	Errors    map[string]string // error por campo; aquí solo Errors["body"]
	General   string            // error que no pertenece a un campo
	CancelURL string            // al editar
	// Inline: el formulario de edición está dentro de la lista de comentarios (se abrió con
	// htmx), así que al guardar o cancelar se vuelve a dibujar el comentario en su sitio.
	Inline bool
}

// newForm es el formulario para comentar el capítulo. El id acaba en -form para que el CSS
// lo muestre en un panel, como los demás formularios de página.
func newForm(chapterID int64) Form {
	return Form{ID: "comment-form", Action: CreateURL(chapterID)}
}

func replyForm(chapterID, parentID int64) Form {
	return Form{ID: fmt.Sprintf("reply-%d", parentID), Action: CreateURL(chapterID), ParentID: parentID}
}

func editForm(t Target, inline bool) Form {
	return Form{
		ID:        "edit-" + t.Comment.Anchor(),
		Action:    t.Comment.EditURL(),
		CommentID: t.Comment.ID,
		Body:      t.Comment.Body,
		CancelURL: t.PageURL(1, t.Comment.Anchor()),
		Inline:    inline,
	}
}

// BodyID es el id del <textarea>, que también debe ser único en la página.
func (f Form) BodyID() string { return f.ID + "-body" }

func (f Form) IsEdit() bool { return f.CommentID != 0 }

func (f Form) Label() string {
	switch {
	case f.IsEdit():
		return "Editar comentario"
	case f.ParentID != 0:
		return "Tu respuesta"
	default:
		return "Deja un comentario"
	}
}

func (f Form) SubmitLabel() string {
	switch {
	case f.IsEdit():
		return "Guardar cambios"
	case f.ParentID != 0:
		return "Responder"
	default:
		return "Comentar"
	}
}
