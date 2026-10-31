package api

import (
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// static serves the embedded frontend: real files as they are (hashed
// assets as immutable), and index.html for any other path so client-side
// routes work.
func (s *Server) static(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	if name == "" {
		name = "index.html"
	}
	if st, err := fs.Stat(s.Web, name); err != nil || st.IsDir() {
		// A missing asset is a stale chunk from an older build (a tab left
		// open across a deploy): answer 404 rather than HTML the browser
		// would try to run as JavaScript.
		if strings.HasPrefix(name, "assets/") {
			http.NotFound(w, r)
			return
		}
		name = "index.html" // SPA fallback
	}
	switch {
	case strings.HasPrefix(name, "assets/"):
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	default:
		w.Header().Set("Cache-Control", "no-cache")
	}
	f, err := s.Web.Open(name)
	if err != nil {
		http.Error(w, "frontend not built", http.StatusNotFound)
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		http.Error(w, "frontend not built", http.StatusNotFound)
		return
	}
	rs, ok := f.(interface {
		Read([]byte) (int, error)
		Seek(int64, int) (int64, error)
	})
	if !ok {
		http.Error(w, "frontend file not seekable", http.StatusInternalServerError)
		return
	}
	if strings.HasSuffix(name, ".webmanifest") {
		w.Header().Set("Content-Type", "application/manifest+json")
	}
	http.ServeContent(w, r, name, st.ModTime(), rs)
}
