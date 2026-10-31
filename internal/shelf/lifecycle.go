package shelf

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"humidity/internal/store"
)

// PiecePatch is the PATCH /pieces/{id} body (api-contract §3.3, §3.5). With
// state it is a lifecycle change; without, a correction.
type PiecePatch struct {
	State          Opt[string]  `json:"state"`
	FlippedAt      Opt[string]  `json:"flipped_at"`
	FlipDryness    Opt[float64] `json:"flip_dryness"`
	EndedAt        Opt[string]  `json:"ended_at"`
	EndDryness     Opt[float64] `json:"end_dryness"`
	EndNote        Opt[string]  `json:"end_note"`
	Mistake        Opt[bool]    `json:"mistake"`
	Form           Opt[string]  `json:"form"`
	Label          Opt[string]  `json:"label"`
	Size           Opt[string]  `json:"size"`
	Thickness      Opt[string]  `json:"thickness"`
	Notes          Opt[string]  `json:"notes"`
	PhotoID        Opt[int64]   `json:"photo_id"`
	AddedAt        Opt[string]  `json:"added_at"`
	InitialDryness Opt[float64] `json:"initial_dryness"`
}

// BulkPatch is the PATCH /pieces body: the same patch for every listed
// piece, all or nothing.
type BulkPatch struct {
	IDs *[]int64 `json:"ids"`
	PiecePatch
}

// lifecycleFields are the fields that belong to particular transitions or
// states. Plain fields (form, label, size, thickness, notes, photo_id) may
// accompany any patch.
var lifecycleFields = []string{"flipped_at", "flip_dryness", "ended_at", "end_dryness", "end_note", "mistake", "added_at", "initial_dryness"}

func (p *PiecePatch) isSet(field string) bool {
	switch field {
	case "flipped_at":
		return p.FlippedAt.Set
	case "flip_dryness":
		return p.FlipDryness.Set
	case "ended_at":
		return p.EndedAt.Set
	case "end_dryness":
		return p.EndDryness.Set
	case "end_note":
		return p.EndNote.Set
	case "mistake":
		return p.Mistake.Set
	case "added_at":
		return p.AddedAt.Set
	case "initial_dryness":
		return p.InitialDryness.Set
	}
	return false
}

type transitionFunc func(c *patchCtx, to string) error

// transitions is the lifecycle table (api-contract §3.5): (from, to) → how
// to apply it. Same-state patches are corrections and are not listed.
var transitions = map[[2]string]transitionFunc{
	{StateIn, StateFlipped}:        (*patchCtx).flip,
	{StateIn, StateTrimmed}:        (*patchCtx).end,
	{StateFlipped, StateTrimmed}:   (*patchCtx).end,
	{StateIn, StateDiscarded}:      (*patchCtx).end,
	{StateFlipped, StateDiscarded}: (*patchCtx).end,
	{StateFlipped, StateIn}:        (*patchCtx).undoFlip,
	{StateTrimmed, StateIn}:        (*patchCtx).undoEnd,
	{StateTrimmed, StateFlipped}:   (*patchCtx).undoEnd,
	{StateDiscarded, StateIn}:      (*patchCtx).undoEnd,
	{StateDiscarded, StateFlipped}: (*patchCtx).undoEnd,
	{StateTrimmed, StateDiscarded}: (*patchCtx).changeEnd,
	{StateDiscarded, StateTrimmed}: (*patchCtx).changeEnd,
}

// patchCtx applies a patch to one piece inside a transaction.
type patchCtx struct {
	ctx    context.Context
	q      store.Querier
	s      *Service
	now    time.Time
	prefix string // field prefix in a bulk patch, e.g. "ids[2]."
	ref    string // the piece in messages
	v      *ValidationError
	patch  *PiecePatch
	p      store.Piece
	obs    []store.Observation // live, oldest first
}

// Update applies patch to every piece in ids, in one transaction. bulk
// selects per-item error paths (ids[i].field).
func (s *Service) Update(ctx context.Context, ids []int64, patch PiecePatch, bulk bool) ([]PieceView, error) {
	now := s.Now().UTC().Truncate(time.Second)
	v := &ValidationError{}
	if bulk {
		if len(ids) == 0 || len(ids) > 50 {
			v.add("ids", CodeOutOfRange, "list 1 to 50 pieces")
		}
		seen := map[int64]bool{}
		for i, id := range ids {
			if seen[id] {
				v.add(fmt.Sprintf("ids[%d]", i), CodeDuplicate, "listed twice")
			}
			seen[id] = true
		}
		if err := v.err(); err != nil {
			return nil, err
		}
	}

	var updated []store.Piece
	err := s.st.Tx(ctx, func(q store.Querier) error {
		for i, id := range ids {
			c := &patchCtx{ctx: ctx, q: q, s: s, now: now, v: v, patch: &patch, ref: fmt.Sprintf("piece %d", id)}
			if bulk {
				c.prefix = fmt.Sprintf("ids[%d].", i)
				c.ref = fmt.Sprintf("piece %d (ids[%d])", id, i)
			}
			p, err := store.GetPiece(ctx, q, id)
			if errors.Is(err, store.ErrNotFound) {
				return errorf(ErrNotFound, "no %s", c.ref)
			}
			if err != nil {
				return err
			}
			obs, err := store.PieceObservations(ctx, q, []int64{id})
			if err != nil {
				return err
			}
			c.p, c.obs = p, obs[id]
			if err := c.apply(); err != nil {
				return err
			}
			updated = append(updated, c.p)
		}
		return v.err()
	})
	if err != nil {
		return nil, err
	}
	s.changed([]string{ScopePieces, ScopeObservations, ScopePredictions}, pieceIDs(updated))
	return s.views(ctx, updated)
}

func (c *patchCtx) field(name string) string { return c.prefix + name }

func (c *patchCtx) apply() error {
	from := state(c.p)
	to := from
	if c.patch.State.Set {
		st, ok := c.patch.State.Value()
		if !ok {
			c.v.add(c.field("state"), CodeInvalidValue, "must not be null")
			return nil
		}
		if !validKey(pieceStates, st) {
			c.v.add(c.field("state"), CodeInvalidEnum, "not a valid state")
			return nil
		}
		to = st
	}
	before := len(c.v.Fields)
	c.plainFields()
	if from == to {
		return c.corrections(from, before)
	}
	fn, ok := transitions[[2]string{from, to}]
	if !ok {
		return errorf(ErrInvalidTransition, "%s can't go from %s to %s", c.ref, from, to)
	}
	return fn(c, to)
}

// only rejects lifecycle fields outside allowed, and reports whether the
// item is still free of errors since mark.
func (c *patchCtx) only(mark int, allowed ...string) bool {
	for _, f := range lifecycleFields {
		if c.patch.isSet(f) && !slices.Contains(allowed, f) {
			c.v.add(c.field(f), CodeInvalidValue, "doesn't apply to this change")
		}
	}
	return len(c.v.Fields) == mark
}

// plainFields validates and applies form, size, thickness, label, notes and
// photo_id.
func (c *patchCtx) plainFields() {
	pp := c.patch
	enum := func(name string, o Opt[string], opts []Option, dst *string) {
		if !o.Set {
			return
		}
		val, ok := o.Value()
		if !ok || !validKey(opts, val) {
			c.v.add(c.field(name), CodeInvalidEnum, "not a valid option")
			return
		}
		*dst = val
	}
	text := func(name string, o Opt[string], limit int, dst *string) {
		if !o.Set {
			return
		}
		val, ok := o.Value()
		switch {
		case !ok:
			c.v.add(c.field(name), CodeInvalidValue, "must not be null; send \"\" to clear")
		case len([]rune(val)) > limit:
			c.v.add(c.field(name), CodeTooLong, "too long")
		default:
			*dst = val
		}
	}
	enum("form", pp.Form, forms, &c.p.Form)
	enum("size", pp.Size, sizes, &c.p.Size)
	enum("thickness", pp.Thickness, thicknesses, &c.p.Thickness)
	text("label", pp.Label, maxLabel, &c.p.Label)
	text("notes", pp.Notes, maxNotes, &c.p.Notes)
	if pp.PhotoID.Set {
		if id, ok := pp.PhotoID.Value(); ok {
			mark := len(c.v.Fields)
			c.s.checkPhoto(c.ctx, c.q, c.v, c.field("photo_id"), id)
			if len(c.v.Fields) == mark {
				c.p.PhotoID = &id
			}
		} else {
			c.p.PhotoID = nil // detaches from this piece only
		}
	}
}

// --- transitions ---

func (c *patchCtx) flip(string) error {
	mark := len(c.v.Fields)
	if !c.only(mark, "flipped_at", "flip_dryness") {
		return nil
	}
	at := c.defaultTime()
	if t, ok := c.timeField("flipped_at", c.patch.FlippedAt); ok {
		at = t
	} else if c.patch.FlippedAt.Set {
		return nil
	}
	c.notBeforeAdded("flipped_at", at)
	d, hasD := c.drynessField("flip_dryness", c.patch.FlipDryness, false)
	if len(c.v.Fields) != mark {
		return nil
	}
	c.p.FlippedAt = &at
	if hasD {
		if err := c.setObs(store.SourceFlip, d, at); err != nil {
			return err
		}
	}
	return c.save()
}

func (c *patchCtx) end(to string) error {
	mark := len(c.v.Fields)
	allowed := []string{"ended_at", "end_dryness", "end_note"}
	if to == StateDiscarded {
		allowed = append(allowed, "mistake")
	}
	if !c.only(mark, allowed...) {
		return nil
	}
	at := c.defaultTime()
	if t, ok := c.timeField("ended_at", c.patch.EndedAt); ok {
		at = t
	} else if c.patch.EndedAt.Set {
		return nil
	}
	c.checkEndTime(at)
	d, hasD := c.drynessField("end_dryness", c.patch.EndDryness, to == StateTrimmed)
	if to == StateTrimmed && !c.patch.EndDryness.Set {
		d, hasD = 3.0, true // trims default to leather hard
	}
	note := c.endNote()
	if len(c.v.Fields) != mark {
		return nil
	}
	c.p.EndedAt, c.p.EndKind, c.p.EndNote = &at, to, note
	if m, ok := c.patch.Mistake.Value(); ok {
		c.p.Mistake = m
	}
	if hasD {
		if err := c.setObs(store.SourceEnd, d, at); err != nil {
			return err
		}
	}
	return c.save()
}

func (c *patchCtx) undoFlip(string) error {
	if !c.only(len(c.v.Fields)) {
		return nil
	}
	c.p.FlippedAt = nil
	if err := c.deleteObs(store.SourceFlip, true); err != nil {
		return err
	}
	return c.save()
}

func (c *patchCtx) undoEnd(to string) error {
	before := StateIn
	if c.p.FlippedAt != nil {
		before = StateFlipped
	}
	if to != before {
		return errorf(ErrInvalidTransition, "%s was %s before it ended, not %s", c.ref, before, to)
	}
	if !c.only(len(c.v.Fields)) {
		return nil
	}
	used, err := usedNumbers(c.ctx, c.q)
	if err != nil {
		return err
	}
	if used[c.p.Number] {
		c.p.Number = lowestFree(used)
	}
	c.p.EndedAt, c.p.EndKind, c.p.EndNote, c.p.Mistake = nil, "", "", false
	if err := c.deleteObs(store.SourceEnd, true); err != nil {
		return err
	}
	return c.save()
}

func (c *patchCtx) changeEnd(to string) error {
	mark := len(c.v.Fields)
	allowed := []string{"end_dryness", "end_note"}
	if to == StateDiscarded {
		allowed = append(allowed, "mistake")
	}
	if !c.only(mark, allowed...) {
		return nil
	}
	d, hasD := c.drynessField("end_dryness", c.patch.EndDryness, to == StateTrimmed)
	note := c.endNote()
	if len(c.v.Fields) != mark {
		return nil
	}
	c.p.EndKind = to
	if c.patch.EndNote.Set {
		c.p.EndNote = note
	}
	if to == StateTrimmed {
		c.p.Mistake = false
		if !hasD && c.findObs(store.SourceEnd) == nil {
			d, hasD = 3.0, true // a trim always has an end observation
		}
	} else if m, ok := c.patch.Mistake.Value(); ok {
		c.p.Mistake = m
	}
	if hasD {
		if err := c.setObs(store.SourceEnd, d, *c.p.EndedAt); err != nil {
			return err
		}
	} else if c.patch.EndDryness.Set { // null on a discard clears it
		if err := c.deleteObs(store.SourceEnd, false); err != nil {
			return err
		}
	}
	return c.save()
}

// corrections applies a patch without a state change (api-contract §3.3).
func (c *patchCtx) corrections(st string, mark int) error {
	pp := c.patch
	allowed := []string{"added_at", "initial_dryness"}
	switch st {
	case StateFlipped:
		allowed = append(allowed, "flipped_at", "flip_dryness")
	case StateTrimmed:
		allowed = append(allowed, "ended_at", "end_dryness", "end_note")
	case StateDiscarded:
		allowed = append(allowed, "ended_at", "end_dryness", "end_note", "mistake")
	}
	if !c.only(mark, allowed...) {
		return nil
	}

	added := c.p.AddedAt
	if t, ok := c.timeField("added_at", pp.AddedAt); ok {
		added = t
		if first := c.firstOtherObs(store.SourceAdd); first != nil && first.Before(t) ||
			c.p.FlippedAt != nil && c.p.FlippedAt.Before(t) || c.p.EndedAt != nil && c.p.EndedAt.Before(t) {
			c.v.add(c.field("added_at"), CodeInvalidTime, "must not be after the piece's later checks, flip or end")
		}
	}
	initial, hasInitial := c.drynessField("initial_dryness", pp.InitialDryness, true)

	flipped := c.p.FlippedAt
	if t, ok := c.timeField("flipped_at", pp.FlippedAt); ok {
		flipped = &t
		if t.Before(added) {
			c.v.add(c.field("flipped_at"), CodeBeforeAdded, "must not be before the piece was added")
		}
	}
	flipD, hasFlipD := c.drynessField("flip_dryness", pp.FlipDryness, false)

	ended := c.p.EndedAt
	if t, ok := c.timeField("ended_at", pp.EndedAt); ok {
		ended = &t
		c.p.AddedAt = added // checkEndTime compares with the corrected add time
		c.checkEndTime(t)
	}
	endD, hasEndD := c.drynessField("end_dryness", pp.EndDryness, st == StateTrimmed)
	note := c.endNote()
	if len(c.v.Fields) != mark {
		return nil
	}

	c.p.AddedAt, c.p.FlippedAt, c.p.EndedAt = added, flipped, ended
	if pp.EndNote.Set {
		c.p.EndNote = note
	}
	if m, ok := pp.Mistake.Value(); ok {
		c.p.Mistake = m
	}
	// Move or rewrite the observations the corrected fields mirror.
	err := c.correctObs(store.SourceAdd, pp.AddedAt.Set, added, pp.InitialDryness.Set, initial, hasInitial)
	if err == nil && st == StateFlipped {
		err = c.correctObs(store.SourceFlip, pp.FlippedAt.Set, *flipped, pp.FlipDryness.Set, flipD, hasFlipD)
	}
	if err == nil && (st == StateTrimmed || st == StateDiscarded) {
		err = c.correctObs(store.SourceEnd, pp.EndedAt.Set, *ended, pp.EndDryness.Set, endD, hasEndD)
	}
	if err != nil {
		return err
	}
	return c.save()
}

// correctObs moves (timeSet) and/or rewrites (dSet) the observation of the
// given source. A null dryness removes it.
func (c *patchCtx) correctObs(source string, timeSet bool, at time.Time, dSet bool, d float64, hasD bool) error {
	o := c.findObs(source)
	switch {
	case dSet && hasD:
		return c.setObs(source, d, at)
	case dSet && !hasD:
		return c.deleteObs(source, false)
	case timeSet && o != nil:
		o.ObservedAt = at
		return store.UpdateObservation(c.ctx, c.q, *o)
	}
	return nil
}

// --- helpers ---

// defaultTime is "now" for a transition, but never before the last
// observation (which may be up to two minutes ahead).
func (c *patchCtx) defaultTime() time.Time {
	at := c.now
	if n := len(c.obs); n > 0 && c.obs[n-1].ObservedAt.After(at) {
		at = c.obs[n-1].ObservedAt
	}
	return at
}

func (c *patchCtx) timeField(name string, o Opt[string]) (time.Time, bool) {
	if !o.Set {
		return time.Time{}, false
	}
	s, ok := o.Value()
	if !ok {
		c.v.add(c.field(name), CodeInvalidValue, "must not be null")
		return time.Time{}, false
	}
	return parseTime(c.v, c.field(name), s, c.now)
}

// drynessField reads an optional dryness. A null is an error when required.
func (c *patchCtx) drynessField(name string, o Opt[float64], required bool) (float64, bool) {
	if !o.Set {
		return 0, false
	}
	d, ok := o.Value()
	if !ok {
		if required {
			c.v.add(c.field(name), CodeRequired, "must not be null")
		}
		return 0, false
	}
	return dryness(c.v, c.field(name), d)
}

func (c *patchCtx) endNote() string {
	if !c.patch.EndNote.Set {
		return ""
	}
	s, ok := c.patch.EndNote.Value()
	if !ok {
		c.v.add(c.field("end_note"), CodeInvalidValue, "must not be null; send \"\" to clear")
		return ""
	}
	if len([]rune(s)) > maxNotes {
		c.v.add(c.field("end_note"), CodeTooLong, "too long")
	}
	return s
}

func (c *patchCtx) notBeforeAdded(name string, t time.Time) {
	if t.Before(c.p.AddedAt) {
		c.v.add(c.field(name), CodeBeforeAdded, "must not be before the piece was added")
	}
}

// checkEndTime: an end is ≥ added_at, ≥ flipped_at and ≥ the last
// observation other than the end's own.
func (c *patchCtx) checkEndTime(t time.Time) {
	switch {
	case t.Before(c.p.AddedAt):
		c.v.add(c.field("ended_at"), CodeBeforeAdded, "must not be before the piece was added")
	case c.p.FlippedAt != nil && t.Before(*c.p.FlippedAt):
		c.v.add(c.field("ended_at"), CodeInvalidTime, "must not be before the flip")
	default:
		if last := c.lastOtherObs(store.SourceEnd); last != nil && t.Before(*last) {
			c.v.add(c.field("ended_at"), CodeInvalidTime, "must not be before the last check")
		}
	}
}

func (c *patchCtx) findObs(source string) *store.Observation {
	for i := len(c.obs) - 1; i >= 0; i-- {
		if c.obs[i].Source == source {
			return &c.obs[i]
		}
	}
	return nil
}

func (c *patchCtx) firstOtherObs(source string) *time.Time {
	for _, o := range c.obs {
		if o.Source != source {
			return &o.ObservedAt
		}
	}
	return nil
}

func (c *patchCtx) lastOtherObs(source string) *time.Time {
	for i := len(c.obs) - 1; i >= 0; i-- {
		if c.obs[i].Source != source {
			return &c.obs[i].ObservedAt
		}
	}
	return nil
}

// setObs updates the observation of a source, or inserts it.
func (c *patchCtx) setObs(source string, d float64, at time.Time) error {
	if o := c.findObs(source); o != nil {
		o.Dryness, o.ObservedAt = d, at
		return store.UpdateObservation(c.ctx, c.q, *o)
	}
	o := store.Observation{PieceID: c.p.ID, Dryness: d, ObservedAt: at, Source: source, CreatedAt: c.now}
	id, err := store.InsertObservation(c.ctx, c.q, o)
	o.ID = id
	c.obs = append(c.obs, o)
	sortObs(c.obs)
	return err
}

// deleteObs soft-deletes the observation of a source, if any. Unless force
// (undoing a transition), the piece's only observation can't be removed.
func (c *patchCtx) deleteObs(source string, force bool) error {
	o := c.findObs(source)
	if o == nil {
		return nil
	}
	if !force && len(c.obs) == 1 {
		return errorf(ErrLastObservation, "can't remove the only observation of %s", c.ref)
	}
	o.DeletedAt = &c.now
	if err := store.UpdateObservation(c.ctx, c.q, *o); err != nil {
		return err
	}
	c.obs = slices.DeleteFunc(c.obs, func(x store.Observation) bool { return x.ID == o.ID })
	return nil
}

func (c *patchCtx) save() error {
	err := store.UpdatePiece(c.ctx, c.q, c.p)
	if err != nil && strings.Contains(err.Error(), "UNIQUE") {
		return fmt.Errorf("shelf number %d taken: %w", c.p.Number, err)
	}
	return err
}
