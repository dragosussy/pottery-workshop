package app

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"atelier/shared/kit"
)

const (
	photoEdge = 1600 // long edge of a stored photo, in pixels
	thumbEdge = 400  // for grids of photos on a phone connection
	pageSize  = 60
)

var (
	pieceKinds   = []string{"tile", "pot"}
	levels       = []string{"bottom", "middle", "top"}
	spots        = []string{"edge", "middle"}
	ratings      = []string{"great", "fine", "failed"}
	defects      = []string{"crawling", "pinholes", "blisters", "running", "crazing", "shivering", "underfired"}
	stages       = []string{"greenware", "bisque", "refire"}
	methods      = []string{"brush", "dip", "pour", "spray", "sponge"}
	photoStages  = []string{"before", "after"}
	pieceStates  = []string{"bench", "kiln", "waiting", "done"}
	withCoats    = []string{"brush", "sponge"}
	stateClauses = map[string]string{
		"bench":   `p.rating IS NULL AND p.firing_id IS NULL`,
		"kiln":    `p.rating IS NULL AND f.state != 'unloaded'`,
		"waiting": `p.rating IS NULL AND f.state = 'unloaded'`,
		"done":    `p.rating IS NOT NULL`,
	}
)

type Layer struct {
	MaterialID    int64    `json:"material_id"`
	Material      string   `json:"material"`
	Kind          string   `json:"kind"`
	RecipeID      *int64   `json:"recipe_id"`
	RecipeVersion *int     `json:"recipe_version"`
	Stage         *string  `json:"stage"`
	Method        *string  `json:"method"`
	Thickness     *float64 `json:"thickness"`
	Coats         *int     `json:"coats"`
	Area          string   `json:"area"`
	Notes         string   `json:"notes"`
}

type Photo struct {
	ID    int64  `json:"id"`
	Stage string `json:"stage"`
}

type PieceFiring struct {
	ID       int64   `json:"id"`
	State    string  `json:"state"`
	FiredOn  *string `json:"fired_on"`
	Cone     string  `json:"cone"`
	Schedule string  `json:"schedule"`
}

// Piece is anything that goes in the kiln: its clay, stack, firing and result.
type Piece struct {
	ID          int64        `json:"id"`
	Kind        string       `json:"kind"`
	Number      *int64       `json:"number"`
	ClayBodyID  *int64       `json:"clay_body_id"`
	ClayBody    *string      `json:"clay_body"`
	FiringID    *int64       `json:"firing_id"`
	Firing      *PieceFiring `json:"firing"`
	Level       *string      `json:"level"`
	X           *float64     `json:"x"`    // 0–1 from the left of the shelf
	Y           *float64     `json:"y"`    // 0–1 from the front of the kiln
	Spot        *string      `json:"spot"` // edge or middle, worked out from x and y
	RefireOf    *int64       `json:"refire_of"`
	RefiredBy   *int64       `json:"refired_by"`
	Notes       string       `json:"notes"`
	FromMemory  bool         `json:"from_memory"`
	Rating      *string      `json:"rating"`
	ResultNotes string       `json:"result_notes"`
	Defects     []string     `json:"defects"`
	State       string       `json:"state"`
	CreatedAt   time.Time    `json:"created_at"`
	Layers      []Layer      `json:"layers"`
	Photos      []Photo      `json:"photos"`
}

type layerInput struct {
	MaterialID int64    `json:"material_id"`
	RecipeID   *int64   `json:"recipe_id"`
	Stage      *string  `json:"stage"`
	Method     *string  `json:"method"`
	Thickness  *float64 `json:"thickness"`
	Coats      *int     `json:"coats"`
	Area       string   `json:"area"`
	Notes      string   `json:"notes"`
}

type pieceInput struct {
	Kind        string       `json:"kind"`
	Number      *int64       `json:"number"`
	ClayBodyID  *int64       `json:"clay_body_id"`
	FiringID    *int64       `json:"firing_id"`
	Level       *string      `json:"level"`
	X           *float64     `json:"x"`
	Y           *float64     `json:"y"`
	RefireOf    *int64       `json:"refire_of"`
	Notes       string       `json:"notes"`
	FromMemory  bool         `json:"from_memory"`
	Rating      *string      `json:"rating"`
	ResultNotes string       `json:"result_notes"`
	Defects     []string     `json:"defects"`
	Layers      []layerInput `json:"layers"`

	spot *string // worked out from X and Y by check
}

// spotAt says whether a position is near the edge of the shelf, where this
// kiln fires differently from the middle. The shelf is about 5 units wide and
// 3 deep, so "near" is within one unit of a side.
func spotAt(x, y float64) string {
	if x < 1.0/5 || x > 4.0/5 || y < 1.0/3 || y > 2.0/3 {
		return "edge"
	}
	return "middle"
}

func pieceState(rating *string, firing *PieceFiring) string {
	switch {
	case rating != nil:
		return "done"
	case firing == nil:
		return "bench"
	case firing.State == "unloaded":
		return "waiting"
	}
	return "kiln"
}

// check validates everything that doesn't need the database.
func (in *pieceInput) check() error {
	if err := oneOf("kind", in.Kind, pieceKinds); err != nil {
		return err
	}
	if in.RefireOf == nil { // a re-fire takes its number from the earlier record
		if in.Kind == "tile" && (in.Number == nil || *in.Number <= 0) {
			return kit.Errorf(400, "a tile needs the number written on it")
		}
		if in.Kind == "pot" && in.Number != nil {
			return kit.Errorf(400, "pots don't have numbers")
		}
	}
	if err := optionalOneOf("level", in.Level, levels); err != nil {
		return err
	}
	if in.FiringID == nil { // a position only means something within a firing
		in.Level, in.X, in.Y = nil, nil, nil
	}
	if (in.X == nil) != (in.Y == nil) {
		return kit.Errorf(400, "a position needs both x and y")
	}
	if in.X != nil {
		if *in.X < 0 || *in.X > 1 || *in.Y < 0 || *in.Y > 1 {
			return kit.Errorf(400, "that position isn't on the shelf")
		}
		x, y := math.Round(*in.X*1000)/1000, math.Round(*in.Y*1000)/1000
		in.X, in.Y = &x, &y
		spot := spotAt(x, y)
		in.spot = &spot
	}
	if err := optionalOneOf("rating", in.Rating, ratings); err != nil {
		return err
	}
	for _, d := range in.Defects {
		if err := oneOf("defect", d, defects); err != nil {
			return err
		}
	}
	slices.Sort(in.Defects)
	in.Defects = slices.Compact(in.Defects)
	if err := checkLen("notes", in.Notes, 2000); err != nil {
		return err
	}
	if err := checkLen("result_notes", in.ResultNotes, 2000); err != nil {
		return err
	}
	for i := range in.Layers {
		l := &in.Layers[i]
		if err := optionalOneOf("stage", l.Stage, stages); err != nil {
			return err
		}
		if err := optionalOneOf("method", l.Method, methods); err != nil {
			return err
		}
		if l.Thickness != nil {
			if *l.Thickness < 0 || *l.Thickness > 5 {
				return kit.Errorf(400, "thickness must be from 0 to 5")
			}
			rounded := math.Round(*l.Thickness*10) / 10
			l.Thickness = &rounded
		}
		if l.Coats != nil {
			if *l.Coats < 1 || *l.Coats > 20 {
				return kit.Errorf(400, "coats must be from 1 to 20")
			}
			if l.Method == nil || !slices.Contains(withCoats, *l.Method) {
				return kit.Errorf(400, "only brushed or sponged layers have coats")
			}
		}
		if err := checkLen("area", l.Area, 100); err != nil {
			return err
		}
		if err := checkLen("notes", l.Notes, 2000); err != nil {
			return err
		}
	}
	return nil
}

// pieceFilter turns the list's query string into SQL conditions.
func pieceFilter(q url.Values) (string, []any, error) {
	var where []string
	var args []any
	equal := func(param, column string, allowed []string) error {
		v := q.Get(param)
		if v == "" {
			return nil
		}
		if allowed != nil {
			if err := oneOf(param, v, allowed); err != nil {
				return err
			}
		}
		where = append(where, column+` = ?`)
		args = append(args, v)
		return nil
	}
	for _, f := range []struct {
		param, column string
		allowed       []string
	}{
		{"kind", "p.kind", pieceKinds},
		{"rating", "p.rating", ratings},
		{"level", "p.level", levels},
		{"spot", "p.spot", spots},
		{"number", "p.number", nil},
		{"firing", "p.firing_id", nil},
		{"clay", "p.clay_body_id", nil},
		{"schedule", "f.schedule", nil},
	} {
		if err := equal(f.param, f.column, f.allowed); err != nil {
			return "", nil, err
		}
	}
	if v := q.Get("state"); v != "" {
		if err := oneOf("state", v, pieceStates); err != nil {
			return "", nil, err
		}
		where = append(where, stateClauses[v])
	}
	if v := q.Get("material"); v != "" {
		where = append(where, `EXISTS (SELECT 1 FROM layers l WHERE l.piece_id = p.id AND l.material_id = ?)`)
		args = append(args, v)
	}
	if v := q.Get("defect"); v != "" {
		if err := oneOf("defect", v, defects); err != nil {
			return "", nil, err
		}
		where = append(where, `EXISTS (SELECT 1 FROM piece_defects d WHERE d.piece_id = p.id AND d.defect = ?)`)
		args = append(args, v)
	}
	if v := strings.TrimSpace(q.Get("q")); v != "" {
		like := "%" + v + "%"
		where = append(where, `(p.notes LIKE ? OR p.result_notes LIKE ? OR CAST(p.number AS TEXT) = ?
			OR EXISTS (SELECT 1 FROM layers l JOIN materials m ON m.id = l.material_id
				WHERE l.piece_id = p.id AND (m.name LIKE ? OR l.notes LIKE ?)))`)
		args = append(args, like, like, v, like, like)
	}
	if len(where) == 0 {
		return "", nil, nil
	}
	return ` WHERE ` + strings.Join(where, ` AND `), args, nil
}

// queryPieces loads whole pieces: what's listed is what a thumbnail, a caption
// and a filter need, and there are at most a page of them at a time.
func (a *App) queryPieces(ctx context.Context, where string, args ...any) ([]*Piece, error) {
	rows, err := a.DB.QueryContext(ctx, `SELECT p.id, p.kind, p.number, p.clay_body_id, c.name,
		p.firing_id, f.state, f.fired_on, f.cone, f.schedule, p.level, p.pos_x, p.pos_y, p.spot, p.refire_of,
		(SELECT max(n.id) FROM pieces n WHERE n.refire_of = p.id),
		p.notes, p.from_memory, p.rating, p.result_notes, p.created_at
		FROM pieces p
		LEFT JOIN clay_bodies c ON c.id = p.clay_body_id
		LEFT JOIN firings f ON f.id = p.firing_id`+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Piece{}
	byID := map[int64]*Piece{}
	ids := []any{}
	for rows.Next() {
		p := &Piece{Defects: []string{}, Layers: []Layer{}, Photos: []Photo{}}
		var created int64
		var firedOn *string
		var state, cone, schedule *string
		err := rows.Scan(&p.ID, &p.Kind, &p.Number, &p.ClayBodyID, &p.ClayBody,
			&p.FiringID, &state, &firedOn, &cone, &schedule, &p.Level, &p.X, &p.Y, &p.Spot, &p.RefireOf,
			&p.RefiredBy, &p.Notes, &p.FromMemory, &p.Rating, &p.ResultNotes, &created)
		if err != nil {
			return nil, err
		}
		p.CreatedAt = time.Unix(created, 0).UTC()
		if p.FiringID != nil {
			p.Firing = &PieceFiring{ID: *p.FiringID, State: *state, FiredOn: firedOn, Cone: *cone, Schedule: *schedule}
		}
		p.State = pieceState(p.Rating, p.Firing)
		out = append(out, p)
		byID[p.ID] = p
		ids = append(ids, p.ID)
	}
	if err := rows.Err(); err != nil || len(ids) == 0 {
		return out, err
	}
	in := placeholders(len(ids))

	rows, err = a.DB.QueryContext(ctx, `SELECT l.piece_id, l.material_id, m.name, m.kind, l.recipe_id, r.version,
		l.stage, l.method, l.thickness, l.coats, l.area, l.notes
		FROM layers l JOIN materials m ON m.id = l.material_id LEFT JOIN recipes r ON r.id = l.recipe_id
		WHERE l.piece_id IN (`+in+`) ORDER BY l.piece_id, l.position`, ids...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var pieceID int64
		var l Layer
		err := rows.Scan(&pieceID, &l.MaterialID, &l.Material, &l.Kind, &l.RecipeID, &l.RecipeVersion,
			&l.Stage, &l.Method, &l.Thickness, &l.Coats, &l.Area, &l.Notes)
		if err != nil {
			return nil, err
		}
		byID[pieceID].Layers = append(byID[pieceID].Layers, l)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	rows, err = a.DB.QueryContext(ctx, `SELECT piece_id, defect FROM piece_defects WHERE piece_id IN (`+in+`) ORDER BY defect`, ids...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var pieceID int64
		var d string
		if err := rows.Scan(&pieceID, &d); err != nil {
			return nil, err
		}
		byID[pieceID].Defects = append(byID[pieceID].Defects, d)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	rows, err = a.DB.QueryContext(ctx, `SELECT piece_id, id, stage FROM photos WHERE piece_id IN (`+in+`) ORDER BY id`, ids...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var pieceID int64
		var ph Photo
		if err := rows.Scan(&pieceID, &ph.ID, &ph.Stage); err != nil {
			return nil, err
		}
		byID[pieceID].Photos = append(byID[pieceID].Photos, ph)
	}
	return out, rows.Err()
}

func (a *App) piece(ctx context.Context, id int64) (*Piece, error) {
	found, err := a.queryPieces(ctx, ` WHERE p.id = ?`, id)
	if err != nil {
		return nil, err
	}
	if len(found) == 0 {
		return nil, kit.Errorf(404, "no such piece")
	}
	return found[0], nil
}

// listPieces is newest first, a page at a time. The result walk and the kiln
// page need the shelves in order instead, and a firing's load fits one page
// many times over, so ?firing= is never cut short.
func (a *App) listPieces(r *http.Request) (any, error) {
	q := r.URL.Query()
	where, args, err := pieceFilter(q)
	if err != nil {
		return nil, err
	}
	if q.Get("firing") != "" {
		where += ` ORDER BY CASE p.level WHEN 'bottom' THEN 0 WHEN 'middle' THEN 1 WHEN 'top' THEN 2 ELSE 3 END, p.pos_y, p.pos_x, p.id`
		return a.queryPieces(r.Context(), where, args...)
	}
	offset, _ := strconv.Atoi(q.Get("offset"))
	where += ` ORDER BY p.id DESC LIMIT ? OFFSET ?`
	return a.queryPieces(r.Context(), where, append(args, pageSize, max(offset, 0))...)
}

func (a *App) getPiece(r *http.Request) (any, error) {
	id, err := kit.PathID(r)
	if err != nil {
		return nil, err
	}
	return a.piece(r.Context(), id)
}

func (a *App) postPiece(r *http.Request) (any, error) {
	var in pieceInput
	if err := kit.ReadJSON(r, &in); err != nil {
		return nil, err
	}
	if err := in.check(); err != nil {
		return nil, err
	}
	id, err := a.savePiece(r.Context(), 0, in)
	if err != nil {
		return nil, err
	}
	return a.piece(r.Context(), id)
}

func (a *App) putPiece(r *http.Request) (any, error) {
	id, err := kit.PathID(r)
	if err != nil {
		return nil, err
	}
	var in pieceInput
	if err := kit.ReadJSON(r, &in); err != nil {
		return nil, err
	}
	if err := in.check(); err != nil {
		return nil, err
	}
	if _, err := a.savePiece(r.Context(), id, in); err != nil {
		return nil, err
	}
	return a.piece(r.Context(), id)
}

// savePiece creates a piece (id 0) or replaces one, with its layers and
// defects, in one transaction.
func (a *App) savePiece(ctx context.Context, id int64, in pieceInput) (int64, error) {
	tx, err := a.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	var oldFiring *int64
	if id != 0 {
		var kind string
		var refireOf *int64
		err := tx.QueryRowContext(ctx, `SELECT kind, refire_of, firing_id FROM pieces WHERE id = ?`, id).Scan(&kind, &refireOf, &oldFiring)
		if errors.Is(err, sql.ErrNoRows) {
			return 0, kit.Errorf(404, "no such piece")
		}
		if err != nil {
			return 0, err
		}
		if kind != in.Kind {
			return 0, kit.Errorf(400, "a piece can't change between tile and pot")
		}
		in.RefireOf = refireOf
	}
	if in.FiringID != nil && (oldFiring == nil || *oldFiring != *in.FiringID) {
		var state string
		err := tx.QueryRowContext(ctx, `SELECT state FROM firings WHERE id = ?`, *in.FiringID).Scan(&state)
		if errors.Is(err, sql.ErrNoRows) {
			return 0, kit.Errorf(400, "that firing doesn't exist")
		}
		if err != nil {
			return 0, err
		}
		if state != "loading" {
			return 0, kit.Errorf(409, "that kiln isn't being loaded")
		}
	}
	if in.RefireOf != nil {
		err := tx.QueryRowContext(ctx, `SELECT kind, number FROM pieces WHERE id = ?`, *in.RefireOf).Scan(&in.Kind, &in.Number)
		if errors.Is(err, sql.ErrNoRows) {
			return 0, kit.Errorf(400, "the piece being re-fired doesn't exist")
		}
		if err != nil {
			return 0, err
		}
	}

	if id == 0 {
		res, err := tx.ExecContext(ctx, `INSERT INTO pieces (kind, number, clay_body_id, firing_id, level, pos_x, pos_y, spot, refire_of,
			notes, from_memory, rating, result_notes, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			in.Kind, in.Number, in.ClayBodyID, in.FiringID, in.Level, in.X, in.Y, in.spot, in.RefireOf,
			in.Notes, in.FromMemory, in.Rating, in.ResultNotes, time.Now().Unix())
		if err != nil {
			return 0, pieceWriteError(err, in)
		}
		id, _ = res.LastInsertId()
	} else {
		_, err := tx.ExecContext(ctx, `UPDATE pieces SET number = ?, clay_body_id = ?, firing_id = ?, level = ?, pos_x = ?, pos_y = ?, spot = ?,
			notes = ?, from_memory = ?, rating = ?, result_notes = ? WHERE id = ?`,
			in.Number, in.ClayBodyID, in.FiringID, in.Level, in.X, in.Y, in.spot, in.Notes, in.FromMemory, in.Rating, in.ResultNotes, id)
		if err != nil {
			return 0, pieceWriteError(err, in)
		}
		for _, table := range []string{"layers", "piece_defects"} {
			if _, err := tx.ExecContext(ctx, `DELETE FROM `+table+` WHERE piece_id = ?`, id); err != nil {
				return 0, err
			}
		}
	}

	for i, l := range in.Layers {
		recipeID, err := layerRecipe(ctx, tx, l)
		if err != nil {
			return 0, err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO layers (piece_id, position, material_id, recipe_id, stage, method, thickness, coats, area, notes)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, i, l.MaterialID, recipeID, l.Stage, l.Method, l.Thickness, l.Coats, strings.TrimSpace(l.Area), l.Notes)
		if isConstraint(err, "FOREIGN KEY") {
			return 0, kit.Errorf(400, "one of the layers uses a material that doesn't exist")
		}
		if err != nil {
			return 0, err
		}
	}
	for _, d := range in.Defects {
		if _, err := tx.ExecContext(ctx, `INSERT INTO piece_defects (piece_id, defect) VALUES (?, ?)`, id, d); err != nil {
			return 0, err
		}
	}
	return id, tx.Commit()
}

func pieceWriteError(err error, in pieceInput) error {
	if isConstraint(err, "UNIQUE") {
		return kit.Errorf(409, "tile %d is already in the notebook", *in.Number)
	}
	if isConstraint(err, "FOREIGN KEY") {
		return kit.Errorf(400, "that clay body or firing doesn't exist")
	}
	return err
}

// layerRecipe is the recipe version a layer used: the one given, which must
// belong to its material, or else the newest (nil for a material without one).
func layerRecipe(ctx context.Context, tx *sql.Tx, l layerInput) (*int64, error) {
	if l.RecipeID != nil {
		ok, err := exists(ctx, tx, `SELECT 1 FROM recipes WHERE id = ? AND material_id = ?`, *l.RecipeID, l.MaterialID)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, kit.Errorf(400, "that recipe version belongs to a different material")
		}
		return l.RecipeID, nil
	}
	var newest int64
	err := tx.QueryRowContext(ctx, `SELECT id FROM recipes WHERE material_id = ? ORDER BY version DESC LIMIT 1`, l.MaterialID).Scan(&newest)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &newest, err
}

func (a *App) deletePiece(r *http.Request) (any, error) {
	id, err := kit.PathID(r)
	if err != nil {
		return nil, err
	}
	rows, err := a.DB.QueryContext(r.Context(), `SELECT id FROM photos WHERE piece_id = ?`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var photos []int64
	for rows.Next() {
		var ph int64
		if err := rows.Scan(&ph); err != nil {
			return nil, err
		}
		photos = append(photos, ph)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	_, err = a.DB.ExecContext(r.Context(), `DELETE FROM pieces WHERE id = ?`, id)
	if isConstraint(err, "FOREIGN KEY") {
		return nil, kit.Errorf(409, "this piece has a re-fire record, so delete that first")
	}
	if err != nil {
		return nil, err
	}
	for _, ph := range photos {
		a.removePhotoFiles(ph)
	}
	return nil, nil
}

// --- Photos

type photoPath func(a *App, id int64) string

func photoFile(a *App, id int64) string {
	return filepath.Join(a.DataDir, "photos", strconv.FormatInt(id, 10)+".jpg")
}

func thumbFile(a *App, id int64) string {
	return filepath.Join(a.DataDir, "photos", strconv.FormatInt(id, 10)+".thumb.jpg")
}

func (a *App) removePhotoFiles(id int64) {
	os.Remove(photoFile(a, id))
	os.Remove(thumbFile(a, id))
}

// postPhoto stores a photo and its thumbnail. The row and the files appear
// together: the transaction only commits once both files are written.
func (a *App) postPhoto(r *http.Request) (any, error) {
	pieceID, err := kit.PathID(r)
	if err != nil {
		return nil, err
	}
	stage := r.URL.Query().Get("stage")
	if err := oneOf("stage", stage, photoStages); err != nil {
		return nil, err
	}
	if _, err := a.piece(r.Context(), pieceID); err != nil {
		return nil, err
	}
	img, err := kit.ReadPhoto(r.Body)
	if err != nil {
		return nil, err
	}
	tx, err := a.DB.BeginTx(r.Context(), nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(r.Context(), `INSERT INTO photos (piece_id, stage, created_at) VALUES (?, ?, ?)`,
		pieceID, stage, time.Now().Unix())
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	if err := kit.SavePhoto(img, photoFile(a, id), photoEdge); err != nil {
		return nil, err
	}
	if err := kit.SavePhoto(img, thumbFile(a, id), thumbEdge); err != nil {
		a.removePhotoFiles(id)
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		a.removePhotoFiles(id)
		return nil, err
	}
	return Photo{ID: id, Stage: stage}, nil
}

func (a *App) getPhoto(path photoPath) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := kit.PathID(r)
		if err != nil {
			kit.WriteError(w, err)
			return
		}
		file := path(a, id)
		if _, err := os.Stat(file); err != nil {
			kit.WriteError(w, kit.Errorf(404, "no such photo"))
			return
		}
		// Photos never change and their ids are never reused.
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		http.ServeFile(w, r, file)
	}
}

func (a *App) deletePhoto(r *http.Request) (any, error) {
	id, err := kit.PathID(r)
	if err != nil {
		return nil, err
	}
	if _, err := a.DB.ExecContext(r.Context(), `DELETE FROM photos WHERE id = ?`, id); err != nil {
		return nil, err
	}
	a.removePhotoFiles(id)
	return nil, nil
}
