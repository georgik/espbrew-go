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

// MonitorHTML returns the monitor.html content
func MonitorHTML() []byte {
	content, _ := staticFiles.ReadFile("static/monitor.html")
	return content
}

// FaviconSVG returns the favicon.svg content
func FaviconSVG() []byte {
	content, _ := staticFiles.ReadFile("static/favicon.svg")
	return content
}

// HasDashboard returns true if dashboard files are embedded
func HasDashboard() bool {
	_, err := staticFiles.ReadFile("static/index.html")
	return err == nil
}

// HasMonitor returns true if monitor page is embedded
func HasMonitor() bool {
	_, err := staticFiles.ReadFile("static/monitor.html")
	return err == nil
}
