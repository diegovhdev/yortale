package auth

// RegisterForm guarda lo que el usuario escribió y los errores, para volver a mostrar el formulario.
// Las contraseñas nunca se devuelven al navegador.
type RegisterForm struct {
	Username    string
	Email       string
	AcceptTerms bool
	Errors      map[string]string // error por campo, por ejemplo Errors["email"]
	General     string            // error que no pertenece a un campo concreto
}

// ProfileForm es el formulario para editar el perfil propio.
type ProfileForm struct {
	ProfileURL  string // perfil público, para enlazarlo desde la página
	DisplayName string
	Bio         string
	Errors      map[string]string
	General     string
}

func newProfileForm(u User) ProfileForm {
	f := ProfileForm{ProfileURL: u.ProfileURL(), Bio: u.Bio}
	if u.DisplayName != nil {
		f.DisplayName = *u.DisplayName
	}
	return f
}

type LoginForm struct {
	Login   string
	Next    string // ruta a la que volver tras iniciar sesión; siempre pasa por SafeNext
	General string
}
