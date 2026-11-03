//go:build embedweb

// Package web holds the built web app (web/dist). The Dockerfile builds it
// and compiles it in with -tags embedweb.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// Dist returns the built web app.
func Dist() fs.FS {
	sub, _ := fs.Sub(dist, "dist") // can't fail: the embed guarantees dist
	return sub
}
