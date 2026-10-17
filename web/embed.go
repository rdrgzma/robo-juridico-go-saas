package web

import (
	"crypto/sha256"
	"embed"
	"encoding/base64"
	"html/template"
	"net/http"
)

//go:embed templates/public/*.html templates/auth/*.html templates/app/*.html static/css/*.css static/js/*.js openapi.json
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

var workspace = template.Must(template.ParseFS(files, "templates/app/workspace.html"))

func Workspace(w http.ResponseWriter, data any) { _ = workspace.Execute(w, data) }
func AppStyle(w http.ResponseWriter, r *http.Request) {
	b, _ := files.ReadFile("static/css/app.css")
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.Write(b)
}
func AppScript(w http.ResponseWriter, r *http.Request) {
	b, _ := files.ReadFile("static/js/app.js")
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Write(b)
}

// ReportStyle is trusted repository-owned CSS, never user content.
func ReportStyle() template.CSS { b, _ := files.ReadFile("static/css/app.css"); return template.CSS(b) }
func ReportCSP() string {
	b, _ := files.ReadFile("static/css/app.css")
	sum := sha256.Sum256(b)
	return "default-src 'none'; style-src 'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'"
}
