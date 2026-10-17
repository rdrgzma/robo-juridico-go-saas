package web

import (
	"embed"
	"html/template"
	"net/http"
)

//go:embed templates/public/*.html templates/auth/*.html static/css/style.css openapi.json
var files embed.FS
var index = template.Must(template.ParseFS(files, "templates/public/index.html"))
var docs = template.Must(template.ParseFS(files, "templates/public/docs.html"))
var identity = template.Must(template.ParseFS(files, "templates/auth/account.html"))

func Auth(w http.ResponseWriter, data any) { _ = identity.Execute(w, data) }

func Docs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = docs.Execute(w, nil)
}

func Index(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = index.Execute(w, nil)
}
func Style(w http.ResponseWriter, r *http.Request) {
	b, _ := files.ReadFile("static/css/style.css")
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	_, _ = w.Write(b)
}
func OpenAPI(w http.ResponseWriter, r *http.Request) {
	b, _ := files.ReadFile("openapi.json")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write(b)
}
