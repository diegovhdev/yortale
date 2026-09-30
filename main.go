package main

import (
	"embed"
	"io/fs"
	"log"
	"net/http"

	"github.com/a-h/templ"
	"github.com/diegovhdev/yortale/web/components"
)

//go:embed web/static
var staticFiles embed.FS

func main() {

	sub, err := fs.Sub(staticFiles, "web/static")

	if err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()

	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(sub))))

	mux.Handle("/", templ.Handler(components.Base()))

	err = http.ListenAndServe(":8080", mux)
	log.Fatal(err)
}
