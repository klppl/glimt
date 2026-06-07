package dashboard

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed assets/*
var assetsFS embed.FS

// AssetsHandler serves the embedded first-party CSS/JS under /assets/.
func AssetsHandler() http.Handler {
	sub, err := fs.Sub(assetsFS, "assets")
	if err != nil {
		panic(err)
	}
	return http.StripPrefix("/assets/", http.FileServerFS(sub))
}
