// Package web carries the self-hosted console font assets in the Go binary.
package web

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed public/fonts
var fontFiles embed.FS

// FontHandler serves the subset faces and their license beneath /fonts/.
var FontHandler = http.StripPrefix("/fonts/", http.FileServerFS(fontRoot()))

func fontRoot() fs.FS {
	root, err := fs.Sub(fontFiles, "public/fonts")
	if err != nil {
		panic(err)
	}
	return root
}
