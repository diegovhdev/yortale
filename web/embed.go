// Package web contiene las plantillas templ y los archivos estáticos.
package web

import "embed"

// StaticFS incluye la carpeta static dentro del binario compilado,
// así el servidor se despliega como un solo archivo.
//
//go:embed static
var StaticFS embed.FS
