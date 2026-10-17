package web

import (
	"crypto/sha256"
	"embed"
	"encoding/base64"
	"html/template"
	"net/http"
)

//go:embed templates/public/*.html templates/auth/*.html templates/app/*.html templates/admin/*.html templates/portal/*.html static/css/*.css static/js/*.js static/swagger/* openapi.json
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
var adminView = template.Must(template.ParseFS(files, "templates/admin/admin.html"))
var portalView = template.Must(template.ParseFS(files, "templates/portal/portal.html"))

func Workspace(w http.ResponseWriter, data any) { _ = workspace.Execute(w, data) }
func Admin(w http.ResponseWriter, data any)     { _ = adminView.Execute(w, data) }
func Portal(w http.ResponseWriter, data any)    { _ = portalView.Execute(w, data) }
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

func SwaggerCSS(w http.ResponseWriter, r *http.Request) {
	b, _ := files.ReadFile("static/swagger/swagger-ui.css")
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write(b)
}

func SwaggerBundle(w http.ResponseWriter, r *http.Request) {
	b, _ := files.ReadFile("static/swagger/swagger-ui-bundle.js")
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write(b)
}

func SwaggerPreset(w http.ResponseWriter, r *http.Request) {
	b, _ := files.ReadFile("static/swagger/swagger-ui-standalone-preset.js")
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write(b)
}

// ReportStyle is trusted repository-owned CSS, never user content.
func ReportStyle() template.CSS { b, _ := files.ReadFile("static/css/app.css"); return template.CSS(b) }
func ReportCSP() string {
	b, _ := files.ReadFile("static/css/app.css")
	sum := sha256.Sum256(b)
	return "default-src 'none'; style-src 'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'"
}
