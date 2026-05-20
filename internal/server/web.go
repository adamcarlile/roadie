package server

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed all:assets
var assets embed.FS

// WebHandler serves the embedded UI assets (internal/server/assets) at the site root.
func WebHandler() http.Handler {
	sub, err := fs.Sub(assets, "assets")
	if err != nil {
		panic(err)
	}
	return http.FileServer(http.FS(sub))
}
