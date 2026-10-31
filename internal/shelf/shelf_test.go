package shelf

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"humidity/internal/store"
)

type notes struct {
	mu    sync.Mutex
	calls [][]string
}

func (n *notes) Changed(scopes []string, _ []int64) {
	n.mu.Lock()
	n.calls = append(n.calls, scopes)
	n.mu.Unlock()
}

type offsets struct{ got map[string]float64 }

func (o *offsets) SetOffsets(m map[string]float64) { o.got = m }

// flatDose is a constant 0.5 effective hours per hour.
type flatDose struct{}

func (flatDose) Dose(from, to time.Time) (float64, float64) {
	if !to.After(from) {
		return 0, 0
	}
	return to.Sub(from).Hours() * 0.5, 0
}

var now = time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)

type fixture struct {
	s   *Service
	st  *store.Store
	n   *notes
	off *offsets
	ctx context.Context
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "h.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	n, off := &notes{}, &offsets{}
	s := New(st, flatDose{}, n, off, []string{"main", "top"}, "Europe/Bucharest")
	s.Now = func() time.Time { return now }
	return &fixture{s: s, st: st, n: n, off: off, ctx: context.Background()}
}

func sp(s string) *string   { return &s }
func fp(f float64) *float64 { return &f }
func ip(i int) *int         { return &i }
func ago(h float64) string {
	return now.Add(-time.Duration(h * float64(time.Hour))).Format(time.RFC3339)
}
func opt[T any](v T) Opt[T] { return Opt[T]{Set: true, V: v} }
func null[T any]() Opt[T]   { return Opt[T]{Set: true, Null: true} }

func (f *fixture) add(t *testing.T, qty int, d float64, addedHoursAgo float64) []PieceView {
	t.Helper()
	out, err := f.s.Add(f.ctx, AddPieces{Quantity: ip(qty), Form: sp("mug"), Size: sp("medium"), Thickness: sp("medium"),
		InitialDryness: fp(d), AddedAt: sp(ago(addedHoursAgo))})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	return out
}

func (f *fixture) patch(t *testing.T, id int64, p PiecePatch) PieceView {
	t.Helper()
	out, err := f.s.Update(f.ctx, []int64{id}, p, false)
	if err != nil {
		t.Fatalf("Update(%d): %v", id, err)
	}
	return out[0]
}

func fieldCodes(err error) map[string]string {
	var ve *ValidationError
	out := map[string]string{}
	if errors.As(err, &ve) {
		for _, fe := range ve.Fields {
			out[fe.Field] = fe.Code
		}
	}
	return out
}

func numbers(ps []PieceView) []int64 {
	var out []int64
	for _, p := range ps {
		out = append(out, p.Number)
	}
	return out
}

func TestAdd_NumbersGroupsAndRounding(t *testing.T) {
	f := newFixture(t)
	group := f.add(t, 3, 1.24, 5)
	if got := numbers(group); !slices.Equal(got, []int64{1, 2, 3}) {
		t.Errorf("numbers = %v", got)
	}
	for _, p := range group {
		if p.GroupID == nil || *p.GroupID != group[0].ID || p.GroupSize != 3 {
			t.Errorf("piece %d group = %v size %d, want %d/3", p.ID, p.GroupID, p.GroupSize, group[0].ID)
		}
		if *p.InitialDryness != 1.2 || *p.CurrentDryness != 1.2 || p.ObservationCount != 1 || p.State != StateIn {
			t.Errorf("piece %d = %+v", p.ID, p)
		}
		if p.AddedAt != ago(5) {
			t.Errorf("added_at = %s, want %s", p.AddedAt, ago(5))
		}
	}
	single := f.add(t, 1, 2, 1)[0]
	if single.GroupID != nil || single.GroupSize != 1 || single.Number != 4 {
		t.Errorf("single = group %v size %d number %d", single.GroupID, single.GroupSize, single.Number)
	}
	if len(f.n.calls) != 2 {
		t.Errorf("notifications = %d, want 2", len(f.n.calls))
	}
}

func TestAdd_Validation(t *testing.T) {
	f := newFixture(t)
	tests := []struct {
		name string
		in   AddPieces
		want map[string]string
	}{
		{"required", AddPieces{}, map[string]string{"form": CodeRequired, "size": CodeRequired, "thickness": CodeRequired, "initial_dryness": CodeRequired}},
		{"enums and ranges", AddPieces{Quantity: ip(21), Form: sp("cup"), Size: sp("huge"), Thickness: sp("thin"), InitialDryness: fp(5.06)},
			map[string]string{"quantity": CodeOutOfRange, "form": CodeInvalidEnum, "size": CodeInvalidEnum, "initial_dryness": CodeOutOfRange}},
		{"future and photo", AddPieces{Form: sp("mug"), Size: sp("small"), Thickness: sp("thin"), InitialDryness: fp(1),
			AddedAt: sp(now.Add(5 * time.Minute).Format(time.RFC3339)), PhotoID: new(int64)},
			map[string]string{"added_at": CodeInFuture, "photo_id": CodePhotoNotFound}},
		{"bad time and long label", AddPieces{Form: sp("mug"), Size: sp("small"), Thickness: sp("thin"), InitialDryness: fp(1),
			AddedAt: sp("yesterday"), Label: sp(string(make([]rune, 81)))},
			map[string]string{"added_at": CodeInvalidTime, "label": CodeTooLong}},
	}
	for _, tc := range tests {
		_, err := f.s.Add(f.ctx, tc.in)
		if got := fieldCodes(err); !mapsEqual(got, tc.want) {
			t.Errorf("%s: fields = %v, want %v", tc.name, got, tc.want)
		}
	}
	// 5.04 rounds to 5.0 and is accepted; −0.04 rounds to 0.
	for _, d := range []float64{5.04, -0.04} {
		if _, err := f.s.Add(f.ctx, AddPieces{Form: sp("mug"), Size: sp("small"), Thickness: sp("thin"), InitialDryness: fp(d)}); err != nil {
			t.Errorf("dryness %v refused: %v", d, err)
		}
	}
}

func mapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func TestNumbering_ReuseAndUndo(t *testing.T) {
	f := newFixture(t)
	ps := f.add(t, 3, 1, 10)
	trimmed := f.patch(t, ps[1].ID, PiecePatch{State: opt(StateTrimmed)})
	if trimmed.EndDryness == nil || *trimmed.EndDryness != 3.0 {
		t.Errorf("trim end_dryness = %v, want default 3.0", trimmed.EndDryness)
	}
	next := f.add(t, 1, 1, 1)[0]
	if next.Number != 2 {
		t.Errorf("new piece number = %d, want the freed 2", next.Number)
	}
	back := f.patch(t, ps[1].ID, PiecePatch{State: opt(StateIn)})
	if back.Number != 4 || back.State != StateIn || back.EndDryness != nil || back.EndedAt != nil {
		t.Errorf("undone piece = number %d state %s end %v", back.Number, back.State, back.EndDryness)
	}
	shelf, _ := f.s.OnShelf(f.ctx)
	if got := numbers(shelf); !slices.Equal(got, []int64{1, 2, 3, 4}) {
		t.Errorf("shelf numbers = %v", got)
	}
}

// setup puts a fresh piece into a state. flippedFirst ends it after a flip.
func (f *fixture) setup(t *testing.T, st string, flippedFirst bool) PieceView {
	p := f.add(t, 1, 1, 10)[0]
	if st == StateFlipped || flippedFirst {
		p = f.patch(t, p.ID, PiecePatch{State: opt(StateFlipped), FlippedAt: opt(ago(5)), FlipDryness: opt(2.0)})
	}
	if st == StateTrimmed || st == StateDiscarded {
		p = f.patch(t, p.ID, PiecePatch{State: opt(st), EndedAt: opt(ago(2))})
	}
	return p
}

func TestTransitions_EveryPair(t *testing.T) {
	states := []string{StateIn, StateFlipped, StateTrimmed, StateDiscarded}
	invalid := map[[2]string]bool{
		{StateTrimmed, StateFlipped}:   true, // ended from in: undo goes back to in only
		{StateDiscarded, StateFlipped}: true,
	}
	for _, flippedFirst := range []bool{false, true} {
		for _, from := range states {
			for _, to := range states {
				if flippedFirst && (from == StateIn || from == StateFlipped) {
					continue
				}
				name := fmt.Sprintf("%s->%s flipped=%v", from, to, flippedFirst)
				t.Run(name, func(t *testing.T) {
					f := newFixture(t)
					p := f.setup(t, from, flippedFirst)
					_, err := f.s.Update(f.ctx, []int64{p.ID}, PiecePatch{State: opt(to)}, false)
					wantErr := invalid[[2]string{from, to}]
					if flippedFirst {
						wantErr = (from == StateTrimmed || from == StateDiscarded) && to == StateIn
					}
					if wantErr {
						if !errors.Is(err, ErrInvalidTransition) {
							t.Fatalf("err = %v, want invalid transition", err)
						}
						return
					}
					if err != nil {
						t.Fatalf("err = %v", err)
					}
					got, _ := f.s.Get(f.ctx, p.ID)
					if got.State != to {
						t.Errorf("state = %s, want %s", got.State, to)
					}
				})
			}
		}
	}
}

func TestTransitions_SideEffects(t *testing.T) {
	f := newFixture(t)
	p := f.add(t, 1, 1, 10)[0]

	p = f.patch(t, p.ID, PiecePatch{State: opt(StateFlipped), FlippedAt: opt(ago(4)), FlipDryness: opt(2.04)})
	if p.FlippedAt == nil || *p.FlippedAt != ago(4) || p.FlipDryness == nil || *p.FlipDryness != 2.0 || p.ObservationCount != 2 {
		t.Fatalf("after flip: %+v", p)
	}
	// Same state again is a no-op that applies corrections.
	p = f.patch(t, p.ID, PiecePatch{State: opt(StateFlipped), FlipDryness: opt(2.2)})
	if *p.FlipDryness != 2.2 || p.ObservationCount != 2 {
		t.Errorf("flip correction: %v, %d obs", *p.FlipDryness, p.ObservationCount)
	}
	p = f.patch(t, p.ID, PiecePatch{State: opt(StateIn)})
	if p.FlippedAt != nil || p.FlipDryness != nil || p.ObservationCount != 1 {
		t.Errorf("undo flip left %+v", p)
	}

	// Discard without a dryness writes no end observation; mistake hides it.
	p = f.patch(t, p.ID, PiecePatch{State: opt(StateDiscarded), Mistake: opt(true), EndNote: opt("never existed")})
	if p.EndDryness != nil || !p.Mistake || p.EndNote != "never existed" || p.ObservationCount != 1 {
		t.Errorf("discard: %+v", p)
	}
	if page, _ := f.s.ListPieces(f.ctx, true, "", 20); len(page.Items) != 0 {
		t.Error("a mistake shows up in the list")
	}
	// Correct the kind of end: discarded → trimmed adds the trim's end
	// observation and clears the mistake.
	p = f.patch(t, p.ID, PiecePatch{State: opt(StateTrimmed)})
	if p.EndDryness == nil || *p.EndDryness != 3.0 || p.Mistake {
		t.Errorf("discarded→trimmed: %+v", p)
	}
}

func TestTransitions_FieldAndTimeRules(t *testing.T) {
	f := newFixture(t)
	p := f.add(t, 1, 1, 10)[0]
	f.s.Observe(f.ctx, NewObservations{ObservedAt: sp(ago(3)), Items: &[]NewObsItem{{PieceID: &p.ID, Dryness: fp(2)}}})

	cases := []struct {
		name  string
		patch PiecePatch
		want  map[string]string
	}{
		{"field not in the transition", PiecePatch{State: opt(StateFlipped), EndDryness: opt(3.0)}, map[string]string{"end_dryness": CodeInvalidValue}},
		{"mistake on a trim", PiecePatch{State: opt(StateTrimmed), Mistake: opt(true)}, map[string]string{"mistake": CodeInvalidValue}},
		{"end before added", PiecePatch{State: opt(StateTrimmed), EndedAt: opt(ago(11))}, map[string]string{"ended_at": CodeBeforeAdded}},
		{"end before last check", PiecePatch{State: opt(StateTrimmed), EndedAt: opt(ago(4))}, map[string]string{"ended_at": CodeInvalidTime}},
		{"end in the future", PiecePatch{State: opt(StateTrimmed), EndedAt: opt(now.Add(3 * time.Minute).Format(time.RFC3339))}, map[string]string{"ended_at": CodeInFuture}},
		{"trim with null dryness", PiecePatch{State: opt(StateTrimmed), EndDryness: null[float64]()}, map[string]string{"end_dryness": CodeRequired}},
		{"flip correction while in", PiecePatch{FlippedAt: opt(ago(1))}, map[string]string{"flipped_at": CodeInvalidValue}},
		{"bad enum", PiecePatch{State: opt("covered")}, map[string]string{"state": CodeInvalidEnum}},
		{"added_at after a check", PiecePatch{AddedAt: opt(ago(2))}, map[string]string{"added_at": CodeInvalidTime}},
	}
	for _, tc := range cases {
		_, err := f.s.Update(f.ctx, []int64{p.ID}, tc.patch, false)
		if got := fieldCodes(err); !mapsEqual(got, tc.want) {
			t.Errorf("%s: fields = %v (err %v), want %v", tc.name, got, err, tc.want)
		}
	}

	// Corrections move the add observation with added_at.
	got := f.patch(t, p.ID, PiecePatch{AddedAt: opt(ago(12)), InitialDryness: opt(0.8), Label: opt("blue")})
	if got.AddedAt != ago(12) || *got.InitialDryness != 0.8 || got.Label != "blue" {
		t.Errorf("correction = %+v", got)
	}
	obs, _ := f.s.PieceObservations(f.ctx, p.ID)
	if obs[0].Source != store.SourceAdd || obs[0].ObservedAt != ago(12) || obs[0].Dryness != 0.8 {
		t.Errorf("add observation = %+v", obs[0])
	}
}

func TestBulk_AllOrNothing(t *testing.T) {
	f := newFixture(t)
	ps := f.add(t, 3, 1, 10)
	f.patch(t, ps[2].ID, PiecePatch{State: opt(StateTrimmed)})

	// The third can't be flipped: nothing changes.
	_, err := f.s.Update(f.ctx, []int64{ps[0].ID, ps[1].ID, ps[2].ID}, PiecePatch{State: opt(StateFlipped)}, true)
	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("err = %v", err)
	}
	for _, p := range ps[:2] {
		if got, _ := f.s.Get(f.ctx, p.ID); got.State != StateIn {
			t.Errorf("piece %d flipped despite the failure", p.ID)
		}
	}
	// Per-item paths for validation errors.
	_, err = f.s.Update(f.ctx, []int64{ps[0].ID, ps[1].ID}, PiecePatch{State: opt(StateFlipped), FlippedAt: opt(ago(11))}, true)
	if got := fieldCodes(err); got["ids[0].flipped_at"] != CodeBeforeAdded || got["ids[1].flipped_at"] != CodeBeforeAdded {
		t.Errorf("fields = %v", got)
	}
	_, err = f.s.Update(f.ctx, []int64{ps[0].ID, ps[0].ID}, PiecePatch{State: opt(StateFlipped)}, true)
	if got := fieldCodes(err); got["ids[1]"] != CodeDuplicate {
		t.Errorf("duplicate ids: %v", got)
	}
	_, err = f.s.Update(f.ctx, []int64{ps[0].ID, 999}, PiecePatch{State: opt(StateFlipped)}, true)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown id: %v", err)
	}
	// And a successful group flip.
	out, err := f.s.Update(f.ctx, []int64{ps[0].ID, ps[1].ID}, PiecePatch{State: opt(StateFlipped), FlipDryness: opt(2.1)}, true)
	if err != nil || len(out) != 2 || out[1].State != StateFlipped || *out[1].FlipDryness != 2.1 {
		t.Errorf("group flip: %v %+v", err, out)
	}
}

func TestObservations_FlagsFollowUpsAndRules(t *testing.T) {
	f := newFixture(t)
	p := f.add(t, 1, 1, 10)[0]
	items := func(d float64) *[]NewObsItem { return &[]NewObsItem{{PieceID: &p.ID, Dryness: fp(d)}} }

	c, err := f.s.Observe(f.ctx, NewObservations{ObservedAt: sp(ago(6)), Items: items(2.6)})
	if err != nil || len(c.Items) != 1 || len(c.FollowUps) != 0 || c.Items[0].Flag != nil || c.Items[0].Source != "user" {
		t.Fatalf("first: %v %+v", err, c)
	}
	// 0.6 wetter: flagged, no question.
	c, _ = f.s.Observe(f.ctx, NewObservations{ObservedAt: sp(ago(5)), Items: items(2.0)})
	if c.Items[0].Flag == nil || *c.Items[0].Flag != "inconsistent" || len(c.FollowUps) != 0 {
		t.Errorf("0.6 wetter: %+v", c)
	}
	// 1.5 wetter than the previous: went_wetter, asked once per piece.
	f.s.Observe(f.ctx, NewObservations{ObservedAt: sp(ago(4)), Items: items(2.6)})
	c, _ = f.s.Observe(f.ctx, NewObservations{ObservedAt: sp(ago(3)), Items: items(1.1)})
	if len(c.FollowUps) != 1 || c.FollowUps[0].Kind != "went_wetter" || c.FollowUps[0].PreviousObservationID == nil {
		t.Fatalf("follow-ups = %+v", c.FollowUps)
	}
	fu := c.FollowUps[0]
	f.s.Observe(f.ctx, NewObservations{ObservedAt: sp(ago(2)), Items: items(2.7)})
	c, _ = f.s.Observe(f.ctx, NewObservations{ObservedAt: sp(ago(1)), Items: items(1.0)})
	if len(c.FollowUps) != 0 {
		t.Errorf("asked twice: %+v", c.FollowUps)
	}
	if err := f.s.AnswerFollowUp(f.ctx, fu.ObservationID, FollowUpAnswer{Answer: sp("covered_or_removed")}); fieldCodes(err)["answer"] != CodeInvalidEnum {
		t.Errorf("wrong option: %v", err)
	}
	if err := f.s.AnswerFollowUp(f.ctx, fu.ObservationID, FollowUpAnswer{Answer: sp("its_fine")}); err != nil {
		t.Errorf("answer: %v", err)
	}
	if err := f.s.AnswerFollowUp(f.ctx, c.Items[0].ID, FollowUpAnswer{Answer: sp("its_fine")}); !errors.Is(err, ErrNotFound) {
		t.Errorf("answer without a question: %v", err)
	}
	if n, _ := store.CountSnapshots(f.ctx, f.st.DB()); n != 6 {
		t.Errorf("snapshots = %d, want one per observation (6)", n)
	}

	// Validation and conflicts.
	_, err = f.s.Observe(f.ctx, NewObservations{ObservedAt: sp(ago(11)), Items: items(2)})
	if fieldCodes(err)["observed_at"] != CodeBeforeAdded {
		t.Errorf("before added: %v", err)
	}
	_, err = f.s.Observe(f.ctx, NewObservations{Items: &[]NewObsItem{{PieceID: &p.ID, Dryness: fp(2)}, {PieceID: &p.ID, Dryness: fp(6)}}})
	if got := fieldCodes(err); got["items[1].piece_id"] != CodeDuplicate || got["items[1].dryness"] != CodeOutOfRange {
		t.Errorf("items: %v", got)
	}
	f.patch(t, p.ID, PiecePatch{State: opt(StateTrimmed)})
	if _, err := f.s.Observe(f.ctx, NewObservations{Items: items(3)}); !errors.Is(err, ErrPieceEnded) {
		t.Errorf("ended piece: %v", err)
	}
}

func TestObservations_EditDeleteUndelete(t *testing.T) {
	f := newFixture(t)
	p := f.add(t, 1, 1, 10)[0]
	obs, _ := f.s.PieceObservations(f.ctx, p.ID)
	if err := f.s.DeleteObservation(f.ctx, obs[0].ID); !errors.Is(err, ErrLastObservation) {
		t.Errorf("delete only observation: %v", err)
	}
	c, _ := f.s.Observe(f.ctx, NewObservations{ObservedAt: sp(ago(2)), Items: &[]NewObsItem{{PieceID: &p.ID, Dryness: fp(2)}}})
	id := c.Items[0].ID

	v, err := f.s.UpdateObservation(f.ctx, id, ObservationPatch{Dryness: opt(2.46), ObservedAt: opt(ago(3)), Note: opt("foot still soft")})
	if err != nil || v.Dryness != 2.5 || v.ObservedAt != ago(3) || v.Note != "foot still soft" {
		t.Errorf("patch: %v %+v", err, v)
	}
	if _, err := f.s.UpdateObservation(f.ctx, id, ObservationPatch{ObservedAt: opt(ago(11))}); fieldCodes(err)["observed_at"] != CodeBeforeAdded {
		t.Errorf("before added: %v", err)
	}
	// Moving the add observation moves added_at.
	if _, err := f.s.UpdateObservation(f.ctx, obs[0].ID, ObservationPatch{ObservedAt: opt(ago(9))}); err != nil {
		t.Fatal(err)
	}
	if got, _ := f.s.Get(f.ctx, p.ID); got.AddedAt != ago(9) {
		t.Errorf("added_at = %s, want %s", got.AddedAt, ago(9))
	}

	if err := f.s.DeleteObservation(f.ctx, id); err != nil {
		t.Fatal(err)
	}
	if err := f.s.DeleteObservation(f.ctx, id); !errors.Is(err, ErrNotFound) {
		t.Errorf("deleted counts as not found: %v", err)
	}
	if got, _ := f.s.Get(f.ctx, p.ID); got.ObservationCount != 1 {
		t.Errorf("count after delete = %d", got.ObservationCount)
	}
	if v, err := f.s.UndeleteObservation(f.ctx, id); err != nil || v.ID != id {
		t.Errorf("undelete: %v", err)
	}
	if got, _ := f.s.Get(f.ctx, p.ID); got.ObservationCount != 2 || *got.CurrentDryness != 2.5 {
		t.Errorf("after undelete: %+v", got)
	}
}

func TestSettings(t *testing.T) {
	f := newFixture(t)
	st, _ := f.s.Settings(f.ctx)
	if st.StudioTZ != "Europe/Bucharest" || len(st.Sensors) != 2 || st.Sensors[0].CalibratedAt != nil {
		t.Errorf("defaults = %+v", st)
	}
	// Rows left from the removed studio schedule are ignored.
	f.st.DB().Exec(`INSERT INTO settings (key, value) VALUES ('studio_days', '["mon"]'), ('studio_configured', 'true')`)

	_, err := f.s.UpdateSettings(f.ctx, SettingsPatch{
		StudioTZ: opt("Mars/Base"),
		Sensors:  opt([]SensorPatch{{Sensor: sp("nope"), OffsetPct: fp(11)}, {Sensor: sp("main")}}),
	})
	want := map[string]string{"studio_tz": CodeInvalidValue, "sensors[0].sensor": CodeInvalidValue,
		"sensors[0].offset_pct": CodeOutOfRange, "sensors[1].offset_pct": CodeRequired}
	if got := fieldCodes(err); !mapsEqual(got, want) {
		t.Errorf("fields = %v, want %v", got, want)
	}

	st, err = f.s.UpdateSettings(f.ctx, SettingsPatch{
		StudioTZ: opt("Asia/Tokyo"),
		Sensors:  opt([]SensorPatch{{Sensor: sp("top"), OffsetPct: fp(-1.25), Replaced: ptr(true)}}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if st.StudioTZ != "Asia/Tokyo" || f.s.mustSettings(t).loc().String() != "Asia/Tokyo" {
		t.Errorf("tz = %s", st.StudioTZ)
	}
	if st.Sensors[1].OffsetPct != -1.3 || st.Sensors[1].CalibratedAt == nil || f.off.got["top"] != -1.3 {
		t.Errorf("offset = %+v, monitor got %v", st.Sensors[1], f.off.got)
	}
	log, _ := f.s.ClimateLog(f.ctx, now.Add(-time.Hour), now.Add(time.Hour))
	if len(log) != 2 || log[0].Kind != store.LogSensorReplaced || log[1].Kind != store.LogSensorCalibrated {
		t.Errorf("climate log = %+v", log)
	}
}

func (s *Service) mustSettings(t *testing.T) Settings {
	st, err := s.Settings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func TestShelfAndStatus(t *testing.T) {
	f := newFixture(t)
	f.st.DB().Exec(`INSERT INTO climate_log (kind, value, at) VALUES ('offline', '{"from":"2026-10-25T10:00:00Z","to":"2026-10-29T10:00:00Z"}', ?)`,
		time.Date(2026, 10, 29, 10, 0, 0, 0, time.UTC).UnixMilli())
	old := f.add(t, 1, 1, 100)[0] // on the shelf before the device was off
	ready := f.add(t, 1, 1, 10)[0]
	f.s.Observe(f.ctx, NewObservations{ObservedAt: sp(ago(0.5)), Items: &[]NewObsItem{{PieceID: &ready.ID, Dryness: fp(3.0)}}})

	sh, err := f.s.Shelf(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(sh.Items) != 2 || sh.Items[1].Prediction.Status != "ready_now" {
		t.Fatalf("items = %+v", sh.Items)
	}
	var restart bool
	for _, c := range sh.CheckThese {
		if c.PieceID == old.ID && slices.Contains(c.Reasons, "not_checked_since_restart") {
			restart = true
		}
	}
	if !restart {
		t.Errorf("check_these = %+v, want the old piece as not_checked_since_restart", sh.CheckThese)
	}
	st, err := f.s.Status(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if st.Shelf.OnShelf != 2 || st.Shelf.ByStage["leather_hard"] != 1 || len(st.Shelf.ByStage) != 6 {
		t.Errorf("shelf summary = %+v", st.Shelf)
	}
	if st.Trim.ReadyNowCount != 1 || st.Trim.ReadyNowUntil == nil || st.Learning.State != "learning" || st.Learning.Needed != 5 {
		t.Errorf("trim/learning = %+v %+v", st.Trim, st.Learning)
	}
	if st.LastOffline == nil || st.LastOffline.PiecesToCheck != 1 || st.LastOffline.To != "2026-10-29T10:00:00Z" {
		t.Errorf("last_offline = %+v", st.LastOffline)
	}
}
