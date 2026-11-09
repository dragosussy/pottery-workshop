// Package app is the glaze notebook's server: see docs/apps/glaze/design.md.
package app

import (
	"database/sql"
	"fmt"
	"io/fs"
	"net/http"
	"slices"
	"strings"
	"unicode/utf8"

	"atelier/shared/kit"
)

// App holds what the HTTP handlers need.
type App struct {
	DB      *sql.DB
	DataDir string
	Web     fs.FS // the built web app
}

// Handler serves the API and the web app.
func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()
	api := func(pattern string, h func(*http.Request) (any, error)) {
		mux.HandleFunc(pattern, kit.JSONHandler(h))
	}
	api("GET /api/summary", a.getSummary)
	api("GET /api/suggestions", a.getSuggestions)

	api("GET /api/clay-bodies", a.listClayBodies)
	api("POST /api/clay-bodies", a.postClayBody)
	api("PUT /api/clay-bodies/{id}", a.putClayBody)
	api("DELETE /api/clay-bodies/{id}", a.deleteClayBody)

	api("GET /api/materials", a.listMaterials)
	api("POST /api/materials", a.postMaterial)
	api("GET /api/materials/{id}", a.getMaterial)
	api("PUT /api/materials/{id}", a.putMaterial)
	api("DELETE /api/materials/{id}", a.deleteMaterial)
	api("PUT /api/materials/{id}/recipe", a.putRecipe)

	api("GET /api/firings", a.listFirings)
	api("POST /api/firings", a.postFiring)
	api("GET /api/firings/{id}", a.getFiring)
	api("PUT /api/firings/{id}", a.putFiring)
	api("PUT /api/firings/{id}/state", a.putFiringState)
	api("DELETE /api/firings/{id}", a.deleteFiring)

	api("GET /api/pieces", a.listPieces)
	api("POST /api/pieces", a.postPiece)
	api("GET /api/pieces/{id}", a.getPiece)
	api("PUT /api/pieces/{id}", a.putPiece)
	api("DELETE /api/pieces/{id}", a.deletePiece)

	api("POST /api/pieces/{id}/photos", a.postPhoto)
	mux.HandleFunc("GET /api/photos/{id}", a.getPhoto(photoFile))
	mux.HandleFunc("GET /api/photos/{id}/thumb", a.getPhoto(thumbFile))
	api("DELETE /api/photos/{id}", a.deletePhoto)

	mux.HandleFunc("GET /api/backup", func(w http.ResponseWriter, r *http.Request) {
		kit.ServeSnapshot(w, r, a.DB, "glaze")
	})
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		kit.WriteError(w, kit.Errorf(404, "no such API route: %s %s", r.Method, r.URL.Path))
	})
	mux.Handle("/", kit.Static(a.Web))
	return mux
}

// getSummary answers the homepage's tile.
func (a *App) getSummary(r *http.Request) (any, error) {
	var bench, kiln, waiting int
	err := a.DB.QueryRowContext(r.Context(), `SELECT
		coalesce(sum(p.rating IS NULL AND p.firing_id IS NULL), 0),
		coalesce(sum(p.rating IS NULL AND f.state != 'unloaded'), 0),
		coalesce(sum(p.rating IS NULL AND f.state = 'unloaded'), 0)
		FROM pieces p LEFT JOIN firings f ON f.id = p.firing_id`).Scan(&bench, &kiln, &waiting)
	if err != nil {
		return nil, err
	}
	return map[string]string{"text": summaryText(bench, kiln, waiting)}, nil
}

func summaryText(bench, kiln, waiting int) string {
	var parts []string
	if bench > 0 {
		parts = append(parts, fmt.Sprintf("%d to be glazed", bench))
	}
	if kiln > 0 {
		parts = append(parts, fmt.Sprintf("%d in the kiln", kiln))
	}
	if waiting > 0 {
		parts = append(parts, fmt.Sprintf("%d waiting for a result", waiting))
	}
	if len(parts) == 0 {
		return "Nothing waiting"
	}
	return strings.Join(parts, " · ")
}

// getSuggestions offers what's been typed before, so the same schedule or
// ingredient is spelled the same way next time and can be filtered on.
func (a *App) getSuggestions(r *http.Request) (any, error) {
	list := func(query string) ([]string, error) {
		rows, err := a.DB.QueryContext(r.Context(), query)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		out := []string{}
		for rows.Next() {
			var s string
			if err := rows.Scan(&s); err != nil {
				return nil, err
			}
			out = append(out, s)
		}
		return out, rows.Err()
	}
	ingredients, err := list(`SELECT DISTINCT ingredient FROM recipe_lines ORDER BY ingredient`)
	if err != nil {
		return nil, err
	}
	schedules, err := list(`SELECT DISTINCT schedule FROM firings WHERE schedule != '' ORDER BY schedule`)
	if err != nil {
		return nil, err
	}
	cones, err := list(`SELECT DISTINCT cone FROM firings WHERE cone != '' ORDER BY cone`)
	if err != nil {
		return nil, err
	}
	return map[string][]string{"ingredients": ingredients, "schedules": schedules, "cones": cones}, nil
}

// --- Validation

func checkLen(field, s string, limit int) error {
	if utf8.RuneCountInString(s) > limit {
		return kit.Errorf(400, "%s is over %d characters", field, limit)
	}
	return nil
}

func checkName(field, s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", kit.Errorf(400, "%s can't be empty", field)
	}
	return s, checkLen(field, s, 100)
}

func oneOf(field, value string, values []string) error {
	if !slices.Contains(values, value) {
		return kit.Errorf(400, "%s must be one of %s", field, strings.Join(values, ", "))
	}
	return nil
}

// optionalOneOf is oneOf for a field that may be left out.
func optionalOneOf(field string, value *string, values []string) error {
	if value == nil {
		return nil
	}
	return oneOf(field, *value, values)
}
