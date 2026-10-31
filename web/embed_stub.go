//go:build !embedweb

// Package web exposes the built frontend (web/dist) to the Go binary.
// Without the embedweb tag it serves a placeholder, so a plain `go build`
// works without Node.
package web

import (
	"io/fs"
	"testing/fstest"
)

// Embedded reports whether the real frontend is compiled in.
const Embedded = false

const placeholder = `<!doctype html>
<html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Drying controller</title></head>
<body style="font-family: system-ui, sans-serif; max-width: 32rem; margin: 3rem auto; padding: 0 1rem">
<h1>Frontend not built</h1>
<p>This binary was built without the web app. The API is running at
<a href="/api/v1/health">/api/v1/health</a>.</p>
<p>Build it with <code>make build</code> (or <code>npm --prefix web run build</code>
and <code>go build -tags embedweb</code>), or run <code>make dev</code> for the Vite dev server.</p>
</body></html>
`

// Dist returns a filesystem holding only the placeholder index.html.
func Dist() fs.FS {
	return fstest.MapFS{"index.html": {Data: []byte(placeholder)}}
}
