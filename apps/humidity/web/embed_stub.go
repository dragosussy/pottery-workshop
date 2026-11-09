//go:build !embedweb

// Package web holds the built web app (web/dist). Without the embedweb tag
// it is a placeholder page, so `go build` works without Node.
package web

import (
	"io/fs"
	"testing/fstest"
)

const placeholder = `<!doctype html>
<meta charset="utf-8">
<title>Humidity</title>
<p>This binary was built without the web app: use the Docker image, or
<code>make dev</code> for the Vite dev server. The API is at
<a href="/api/now">/api/now</a>.</p>
`

// Dist returns a placeholder index.html.
func Dist() fs.FS {
	return fstest.MapFS{"index.html": {Data: []byte(placeholder)}}
}
