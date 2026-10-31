//go:build embedweb

// Package web exposes the built frontend (web/dist) to the Go binary.
// Build with -tags embedweb after `npm --prefix web run build`.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// Embedded reports whether the real frontend is compiled in.
const Embedded = true

// Dist returns the built frontend, rooted at dist/.
func Dist() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err) // the embed pattern guarantees dist exists
	}
	return sub
}
