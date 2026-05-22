package dashboard

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed all:static
var staticFiles embed.FS

// StaticFS returns the embedded static files
func StaticFS() http.FileSystem {
	sub, _ := fs.Sub(staticFiles, "static")
	return http.FS(sub)
}

// IndexHTML returns the index.html content
func IndexHTML() []byte {
	content, _ := staticFiles.ReadFile("static/index.html")
	return content
}

// HasDashboard returns true if dashboard files are embedded
func HasDashboard() bool {
	_, err := staticFiles.ReadFile("static/index.html")
	return err == nil
}
