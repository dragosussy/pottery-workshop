package app

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"

	"atelier/shared/kit"
)

var materialKinds = []string{"oxide", "slip", "commercial", "own"}

var datePattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// exists answers whether a query finds a row.
func exists(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, query string, args ...any) (bool, error) {
	var found int
	err := q.QueryRowContext(ctx, `SELECT EXISTS(`+query+`)`, args...).Scan(&found)
	return found == 1, err
}

// --- Clay bodies

type ClayBody struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Notes    string `json:"notes"`
	Archived bool   `json:"archived"`
}

type clayBodyInput struct {
	Name     string `json:"name"`
	Notes    string `json:"notes"`
	Archived bool   `json:"archived"`
}

func (in *clayBodyInput) check() (err error) {
	if in.Name, err = checkName("name", in.Name); err != nil {
		return err
	}
	return checkLen("notes", in.Notes, 2000)
}

func archivedAt(archived bool) any {
	if archived {
		return time.Now().Unix()
	}
	return nil
}

func (a *App) listClayBodies(r *http.Request) (any, error) {
	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT id, name, notes, archived_at IS NOT NULL FROM clay_bodies ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ClayBody{}
	for rows.Next() {
		var c ClayBody
		if err := rows.Scan(&c.ID, &c.Name, &c.Notes, &c.Archived); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (a *App) postClayBody(r *http.Request) (any, error) {
	var in clayBodyInput
	if err := kit.ReadJSON(r, &in); err != nil {
		return nil, err
	}
	if err := in.check(); err != nil {
		return nil, err
	}
	res, err := a.DB.ExecContext(r.Context(), `INSERT INTO clay_bodies (name, notes) VALUES (?, ?)`, in.Name, in.Notes)
	if isConstraint(err, "UNIQUE") {
		return nil, kit.Errorf(409, "there is already a clay body called %s", in.Name)
	}
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return ClayBody{ID: id, Name: in.Name, Notes: in.Notes}, nil
}

func (a *App) putClayBody(r *http.Request) (any, error) {
	id, err := kit.PathID(r)
	if err != nil {
		return nil, err
	}
	var in clayBodyInput
	if err := kit.ReadJSON(r, &in); err != nil {
		return nil, err
	}
	if err := in.check(); err != nil {
		return nil, err
	}
	res, err := a.DB.ExecContext(r.Context(), `UPDATE clay_bodies SET name = ?, notes = ?, archived_at = ? WHERE id = ?`,
		in.Name, in.Notes, archivedAt(in.Archived), id)
	if isConstraint(err, "UNIQUE") {
		return nil, kit.Errorf(409, "there is already a clay body called %s", in.Name)
	}
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, kit.Errorf(404, "no such clay body")
	}
	return ClayBody{ID: id, Name: in.Name, Notes: in.Notes, Archived: in.Archived}, nil
}

func (a *App) deleteClayBody(r *http.Request) (any, error) {
	id, err := kit.PathID(r)
	if err != nil {
		return nil, err
	}
	_, err = a.DB.ExecContext(r.Context(), `DELETE FROM clay_bodies WHERE id = ?`, id)
	if isConstraint(err, "FOREIGN KEY") {
		return nil, kit.Errorf(409, "that clay body is in use, so archive it instead")
	}
	return nil, err
}

// --- Materials and recipes

type LastUse struct {
	Stage     *string  `json:"stage"`
	Method    *string  `json:"method"`
	Thickness *float64 `json:"thickness"`
	Coats     *int     `json:"coats"`
}

type RecipeRef struct {
	ID      int64 `json:"id"`
	Version int   `json:"version"`
}

// Material is a list entry: anything we put on clay.
type Material struct {
	ID       int64       `json:"id"`
	Name     string      `json:"name"`
	Kind     string      `json:"kind"`
	Maker    string      `json:"maker"`
	Notes    string      `json:"notes"`
	Archived bool        `json:"archived"`
	Uses     int         `json:"uses"`
	LastUse  *LastUse    `json:"last_use"` // how it was last applied
	Versions []RecipeRef `json:"versions"` // newest first
}

type RecipeLine struct {
	Ingredient string  `json:"ingredient"`
	Amount     float64 `json:"amount"`
	Addition   bool    `json:"addition"`
}

type Recipe struct {
	ID          int64        `json:"id"`
	Version     int          `json:"version"`
	ChangeNote  string       `json:"change_note"`
	Water       *float64     `json:"water"`       // starting water, in the same parts as the lines
	Consistency string       `json:"consistency"` // what to aim for once mixed
	CreatedAt   time.Time    `json:"created_at"`
	Used        bool         `json:"used"` // a used version is frozen
	Lines       []RecipeLine `json:"lines"`
}

type MaterialDetail struct {
	Material
	Recipes []Recipe `json:"recipes"` // newest first
}

type materialInput struct {
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	Maker    string `json:"maker"`
	Notes    string `json:"notes"`
	Archived bool   `json:"archived"`
}

func (in *materialInput) check() (err error) {
	if in.Name, err = checkName("name", in.Name); err != nil {
		return err
	}
	if err := oneOf("kind", in.Kind, materialKinds); err != nil {
		return err
	}
	if err := checkLen("maker", in.Maker, 200); err != nil {
		return err
	}
	return checkLen("notes", in.Notes, 2000)
}

// loadMaterials returns materials, most recently used first, with how each
// was last applied. Recency goes by piece, because saving a piece rewrites
// its layers.
func (a *App) loadMaterials(ctx context.Context) ([]*Material, error) {
	rows, err := a.DB.QueryContext(ctx, `SELECT id, name, kind, maker, notes, archived_at IS NOT NULL FROM materials ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Material{}
	byID := map[int64]*Material{}
	for rows.Next() {
		m := &Material{Versions: []RecipeRef{}}
		if err := rows.Scan(&m.ID, &m.Name, &m.Kind, &m.Maker, &m.Notes, &m.Archived); err != nil {
			return nil, err
		}
		out = append(out, m)
		byID[m.ID] = m
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	rows, err = a.DB.QueryContext(ctx, `SELECT material_id, id, version FROM recipes ORDER BY version DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var materialID int64
		var v RecipeRef
		if err := rows.Scan(&materialID, &v.ID, &v.Version); err != nil {
			return nil, err
		}
		byID[materialID].Versions = append(byID[materialID].Versions, v)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	rows, err = a.DB.QueryContext(ctx, `SELECT material_id, piece_id, stage, method, thickness, coats FROM layers ORDER BY piece_id, position`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	lastPiece := map[int64]int64{}
	for rows.Next() {
		var materialID, pieceID int64
		var u LastUse
		if err := rows.Scan(&materialID, &pieceID, &u.Stage, &u.Method, &u.Thickness, &u.Coats); err != nil {
			return nil, err
		}
		byID[materialID].Uses++
		byID[materialID].LastUse = &u
		lastPiece[materialID] = pieceID
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	slices.SortStableFunc(out, func(x, y *Material) int {
		return int(lastPiece[y.ID] - lastPiece[x.ID])
	})
	return out, nil
}

func (a *App) listMaterials(r *http.Request) (any, error) { return a.loadMaterials(r.Context()) }

func (a *App) materialDetail(ctx context.Context, id int64) (*MaterialDetail, error) {
	all, err := a.loadMaterials(ctx)
	if err != nil {
		return nil, err
	}
	i := slices.IndexFunc(all, func(m *Material) bool { return m.ID == id })
	if i < 0 {
		return nil, kit.Errorf(404, "no such material")
	}
	d := &MaterialDetail{Material: *all[i], Recipes: []Recipe{}}

	rows, err := a.DB.QueryContext(ctx, `SELECT id, version, change_note, water, consistency, created_at,
		EXISTS(SELECT 1 FROM layers WHERE recipe_id = recipes.id)
		FROM recipes WHERE material_id = ? ORDER BY version DESC`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var rc Recipe
		var created int64
		if err := rows.Scan(&rc.ID, &rc.Version, &rc.ChangeNote, &rc.Water, &rc.Consistency, &created, &rc.Used); err != nil {
			return nil, err
		}
		rc.CreatedAt = time.Unix(created, 0).UTC()
		rc.Lines = []RecipeLine{}
		d.Recipes = append(d.Recipes, rc)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range d.Recipes {
		lines, err := a.DB.QueryContext(ctx, `SELECT ingredient, amount, addition FROM recipe_lines WHERE recipe_id = ? ORDER BY position`, d.Recipes[i].ID)
		if err != nil {
			return nil, err
		}
		defer lines.Close()
		for lines.Next() {
			var l RecipeLine
			if err := lines.Scan(&l.Ingredient, &l.Amount, &l.Addition); err != nil {
				return nil, err
			}
			d.Recipes[i].Lines = append(d.Recipes[i].Lines, l)
		}
		if err := lines.Err(); err != nil {
			return nil, err
		}
	}
	return d, nil
}

func (a *App) getMaterial(r *http.Request) (any, error) {
	id, err := kit.PathID(r)
	if err != nil {
		return nil, err
	}
	return a.materialDetail(r.Context(), id)
}

func (a *App) postMaterial(r *http.Request) (any, error) {
	var in materialInput
	if err := kit.ReadJSON(r, &in); err != nil {
		return nil, err
	}
	if err := in.check(); err != nil {
		return nil, err
	}
	res, err := a.DB.ExecContext(r.Context(), `INSERT INTO materials (name, kind, maker, notes) VALUES (?, ?, ?, ?)`,
		in.Name, in.Kind, in.Maker, in.Notes)
	if isConstraint(err, "UNIQUE") {
		return nil, kit.Errorf(409, "there is already a material called %s", in.Name)
	}
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return a.materialDetail(r.Context(), id)
}

func (a *App) putMaterial(r *http.Request) (any, error) {
	id, err := kit.PathID(r)
	if err != nil {
		return nil, err
	}
	var in materialInput
	if err := kit.ReadJSON(r, &in); err != nil {
		return nil, err
	}
	if err := in.check(); err != nil {
		return nil, err
	}
	if in.Kind == "commercial" {
		has, err := exists(r.Context(), a.DB, `SELECT 1 FROM recipes WHERE material_id = ?`, id)
		if err != nil {
			return nil, err
		}
		if has {
			return nil, kit.Errorf(409, "a material with a recipe can't be commercial")
		}
	}
	res, err := a.DB.ExecContext(r.Context(), `UPDATE materials SET name = ?, kind = ?, maker = ?, notes = ?, archived_at = ? WHERE id = ?`,
		in.Name, in.Kind, in.Maker, in.Notes, archivedAt(in.Archived), id)
	if isConstraint(err, "UNIQUE") {
		return nil, kit.Errorf(409, "there is already a material called %s", in.Name)
	}
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, kit.Errorf(404, "no such material")
	}
	return a.materialDetail(r.Context(), id)
}

// deleteMaterial removes a material nothing uses. Its unused recipes go with it.
func (a *App) deleteMaterial(r *http.Request) (any, error) {
	id, err := kit.PathID(r)
	if err != nil {
		return nil, err
	}
	tx, err := a.DB.BeginTx(r.Context(), nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	used, err := exists(r.Context(), tx, `SELECT 1 FROM layers WHERE material_id = ?`, id)
	if err != nil {
		return nil, err
	}
	if used {
		return nil, kit.Errorf(409, "that material is in use, so archive it instead")
	}
	if _, err := tx.ExecContext(r.Context(), `DELETE FROM recipes WHERE material_id = ?`, id); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(r.Context(), `DELETE FROM materials WHERE id = ?`, id); err != nil {
		return nil, err
	}
	return nil, tx.Commit()
}

// putRecipe saves a recipe. A version any layer uses is frozen, so old results
// keep describing what was really on them: the change becomes the next
// version. The newest version can be corrected in place until it's first used.
func (a *App) putRecipe(r *http.Request) (any, error) {
	id, err := kit.PathID(r)
	if err != nil {
		return nil, err
	}
	var in struct {
		ChangeNote  string       `json:"change_note"`
		Water       *float64     `json:"water"`
		Consistency string       `json:"consistency"`
		Lines       []RecipeLine `json:"lines"`
	}
	if err := kit.ReadJSON(r, &in); err != nil {
		return nil, err
	}
	if err := checkLen("change_note", in.ChangeNote, 500); err != nil {
		return nil, err
	}
	if err := checkLen("consistency", in.Consistency, 200); err != nil {
		return nil, err
	}
	if in.Water != nil && *in.Water <= 0 {
		return nil, kit.Errorf(400, "the water must be above 0, or left out")
	}
	if err := checkRecipeLines(in.Lines); err != nil {
		return nil, err
	}

	ctx := r.Context()
	tx, err := a.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var kind string
	err = tx.QueryRowContext(ctx, `SELECT kind FROM materials WHERE id = ?`, id).Scan(&kind)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, kit.Errorf(404, "no such material")
	}
	if err != nil {
		return nil, err
	}
	if kind == "commercial" {
		return nil, kit.Errorf(400, "commercial products don't have recipes")
	}

	var recipeID int64
	var version int
	err = tx.QueryRowContext(ctx, `SELECT id, version FROM recipes WHERE material_id = ? ORDER BY version DESC LIMIT 1`, id).Scan(&recipeID, &version)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	frozen := true
	if recipeID != 0 {
		used, err := exists(ctx, tx, `SELECT 1 FROM layers WHERE recipe_id = ?`, recipeID)
		if err != nil {
			return nil, err
		}
		frozen = used
	}
	if frozen {
		res, err := tx.ExecContext(ctx, `INSERT INTO recipes (material_id, version, change_note, water, consistency, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
			id, version+1, in.ChangeNote, in.Water, in.Consistency, time.Now().Unix())
		if err != nil {
			return nil, err
		}
		recipeID, _ = res.LastInsertId()
	} else {
		if _, err := tx.ExecContext(ctx, `UPDATE recipes SET change_note = ?, water = ?, consistency = ? WHERE id = ?`, in.ChangeNote, in.Water, in.Consistency, recipeID); err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM recipe_lines WHERE recipe_id = ?`, recipeID); err != nil {
			return nil, err
		}
	}
	for i, l := range in.Lines {
		_, err := tx.ExecContext(ctx, `INSERT INTO recipe_lines (recipe_id, position, ingredient, amount, addition) VALUES (?, ?, ?, ?, ?)`,
			recipeID, i, strings.TrimSpace(l.Ingredient), l.Amount, l.Addition)
		if err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return a.materialDetail(ctx, id)
}

func checkRecipeLines(lines []RecipeLine) error {
	base := false
	for _, l := range lines {
		if _, err := checkName("ingredient", l.Ingredient); err != nil {
			return err
		}
		if l.Amount <= 0 {
			return kit.Errorf(400, "the amount of %s must be above 0", l.Ingredient)
		}
		base = base || !l.Addition
	}
	if !base {
		return kit.Errorf(400, "a recipe needs at least one ingredient that isn't an addition")
	}
	return nil
}

// --- Firings

type Firing struct {
	ID       int64   `json:"id"`
	State    string  `json:"state"` // loading, firing or unloaded
	FiredOn  *string `json:"fired_on"`
	Cone     string  `json:"cone"`
	Schedule string  `json:"schedule"`
	Notes    string  `json:"notes"`
	Pieces   int     `json:"pieces"`
	Waiting  int     `json:"waiting"` // pieces with no result yet
}

type firingInput struct {
	FiredOn  *string `json:"fired_on"`
	Cone     string  `json:"cone"`
	Schedule string  `json:"schedule"`
	Notes    string  `json:"notes"`
}

func (in *firingInput) check() error {
	if in.FiredOn != nil {
		if _, err := time.Parse(time.DateOnly, *in.FiredOn); err != nil || !datePattern.MatchString(*in.FiredOn) {
			return kit.Errorf(400, "fired_on must be a date like 2026-11-09")
		}
	}
	in.Cone, in.Schedule = strings.TrimSpace(in.Cone), strings.TrimSpace(in.Schedule)
	if err := checkLen("cone", in.Cone, 20); err != nil {
		return err
	}
	if err := checkLen("schedule", in.Schedule, 200); err != nil {
		return err
	}
	return checkLen("notes", in.Notes, 2000)
}

const firingSelect = `SELECT f.id, f.state, f.fired_on, f.cone, f.schedule, f.notes,
	(SELECT count(*) FROM pieces p WHERE p.firing_id = f.id),
	(SELECT count(*) FROM pieces p WHERE p.firing_id = f.id AND p.rating IS NULL)
	FROM firings f`

func scanFiring(s interface{ Scan(...any) error }) (Firing, error) {
	var f Firing
	err := s.Scan(&f.ID, &f.State, &f.FiredOn, &f.Cone, &f.Schedule, &f.Notes, &f.Pieces, &f.Waiting)
	return f, err
}

func (a *App) listFirings(r *http.Request) (any, error) {
	rows, err := a.DB.QueryContext(r.Context(), firingSelect+` ORDER BY f.fired_on IS NULL, f.fired_on DESC, f.id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Firing{}
	for rows.Next() {
		f, err := scanFiring(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func (a *App) firing(ctx context.Context, id int64) (any, error) {
	f, err := scanFiring(a.DB.QueryRowContext(ctx, firingSelect+` WHERE f.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, kit.Errorf(404, "no such firing")
	}
	return f, err
}

func (a *App) getFiring(r *http.Request) (any, error) {
	id, err := kit.PathID(r)
	if err != nil {
		return nil, err
	}
	return a.firing(r.Context(), id)
}

func (a *App) postFiring(r *http.Request) (any, error) {
	var in firingInput
	if err := kit.ReadJSON(r, &in); err != nil {
		return nil, err
	}
	if err := in.check(); err != nil {
		return nil, err
	}
	res, err := a.DB.ExecContext(r.Context(), `INSERT INTO firings (state, fired_on, cone, schedule, notes) VALUES ('loading', ?, ?, ?, ?)`,
		in.FiredOn, in.Cone, in.Schedule, in.Notes)
	if isConstraint(err, "UNIQUE") {
		return nil, kit.Errorf(409, "the kiln already has a load in it")
	}
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return a.firing(r.Context(), id)
}

func (a *App) putFiring(r *http.Request) (any, error) {
	id, err := kit.PathID(r)
	if err != nil {
		return nil, err
	}
	var in firingInput
	if err := kit.ReadJSON(r, &in); err != nil {
		return nil, err
	}
	if err := in.check(); err != nil {
		return nil, err
	}
	res, err := a.DB.ExecContext(r.Context(), `UPDATE firings SET fired_on = ?, cone = ?, schedule = ?, notes = ? WHERE id = ?`,
		in.FiredOn, in.Cone, in.Schedule, in.Notes, id)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, kit.Errorf(404, "no such firing")
	}
	return a.firing(r.Context(), id)
}

// firingMoves are the changes of state a load can go through, including
// stepping back from a mistaken tap.
var firingMoves = map[string][]string{
	"loading":  {"firing"},
	"firing":   {"loading", "unloaded"},
	"unloaded": {"firing"},
}

func (a *App) putFiringState(r *http.Request) (any, error) {
	id, err := kit.PathID(r)
	if err != nil {
		return nil, err
	}
	var in struct {
		State string `json:"state"`
	}
	if err := kit.ReadJSON(r, &in); err != nil {
		return nil, err
	}
	ctx := r.Context()
	var state string
	var pieces int
	err = a.DB.QueryRowContext(ctx, `SELECT state, (SELECT count(*) FROM pieces WHERE firing_id = firings.id) FROM firings WHERE id = ?`, id).Scan(&state, &pieces)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, kit.Errorf(404, "no such firing")
	}
	if err != nil {
		return nil, err
	}
	if !slices.Contains(firingMoves[state], in.State) {
		return nil, kit.Errorf(409, "a firing that is %s can't become %s", state, in.State)
	}
	if in.State == "firing" && pieces == 0 {
		return nil, kit.Errorf(409, "there's nothing in the kiln yet")
	}
	// A firing is dated by the day it starts, unless a date was given.
	_, err = a.DB.ExecContext(ctx, `UPDATE firings SET state = ?, fired_on = coalesce(fired_on, CASE WHEN ? = 'firing' THEN ? END) WHERE id = ?`,
		in.State, in.State, time.Now().Format(time.DateOnly), id)
	if isConstraint(err, "UNIQUE") {
		return nil, kit.Errorf(409, "the kiln already has a load in it")
	}
	if err != nil {
		return nil, err
	}
	return a.firing(ctx, id)
}

func (a *App) deleteFiring(r *http.Request) (any, error) {
	id, err := kit.PathID(r)
	if err != nil {
		return nil, err
	}
	_, err = a.DB.ExecContext(r.Context(), `DELETE FROM firings WHERE id = ?`, id)
	if isConstraint(err, "FOREIGN KEY") {
		return nil, kit.Errorf(409, "that firing still has pieces in it")
	}
	return nil, err
}
