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

type LoginForm struct {
	Login   string
	Next    string // ruta a la que volver tras iniciar sesión; siempre pasa por SafeNext
	General string
}
