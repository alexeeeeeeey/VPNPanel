// Package frontend contains the production web assets built by Vite.
package frontend

import (
	"embed"
	"io/fs"
)

//go:generate npm run build

//go:embed dist
var assets embed.FS

// Assets returns the frontend with paths relative to the web root.
func Assets() (fs.FS, error) {
	return fs.Sub(assets, "dist")
}
