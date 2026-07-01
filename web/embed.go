// Package web embeds the built SPA assets.
//
// The Vite build writes to ./dist. A tracked placeholder (dist/index.html)
// keeps this embed compiling before any frontend build has run.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var distFS embed.FS

// Dist returns the built SPA filesystem rooted at the dist directory.
func Dist() (fs.FS, error) {
	return fs.Sub(distFS, "dist")
}
