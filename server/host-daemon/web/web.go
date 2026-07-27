package web

import (
	"embed"
	"html/template"
	"io"
	"io/fs"
	"net/http"
	"time"
)

//go:embed templates/*.html static/*
var assets embed.FS

type Renderer struct{ templates *template.Template }

func New(functions template.FuncMap) (*Renderer, error) {
	templates, err := template.New("pages").Funcs(functions).ParseFS(assets, "templates/*.html")
	if err != nil {
		return nil, err
	}
	return &Renderer{templates: templates}, nil
}

func (r *Renderer) Execute(writer io.Writer, name string, data any) error {
	return r.templates.ExecuteTemplate(writer, name, data)
}

func Static() http.Handler {
	sub, err := fs.Sub(assets, "static")
	if err != nil {
		panic(err)
	}
	return http.StripPrefix("/assets/", http.FileServer(http.FS(sub)))
}

func Functions(humanBytes func(int64) string) template.FuncMap {
	return template.FuncMap{
		"bytes": humanBytes,
		"time": func(value time.Time) string {
			if value.IsZero() {
				return "Not yet"
			}
			return value.UTC().Format("2006-01-02 15:04 UTC")
		},
	}
}
