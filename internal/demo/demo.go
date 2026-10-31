// Package demo fills an empty database with backdated, realistic data so
// every screen state of the web app can be seen during development
// (DEMO_SEED, sim driver only):
//
//   - two weeks of climate readings, with a 3-day offline period that
//     ended 3 days ago
//   - finished pieces (trimmed, one covered early, one cracked, one entered
//     by mistake); "calibrated" mode has enough of them to leave the
//     learning period, "learning" mode has 2
//   - two pieces left on the shelf across the offline period
//   - the current session: a flipped group of 4 mugs sharing a photo, and
//     pieces that are drying, probably ready, ready now, may be past and
//     past trim, plus an inconsistent observation
//   - a sensor calibration
package demo

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"log/slog"
	"math"
	"time"

	"humidity/internal/config"
	"humidity/internal/photos"
	"humidity/internal/predict"
	"humidity/internal/recorder"
	"humidity/internal/shelf"
	"humidity/internal/store"
)

// Deps are what the seed writes through.
type Deps struct {
	Store   *store.Store
	Shelf   *shelf.Service
	Photos  *photos.Service
	Dose    *recorder.DoseIndex
	Sensors []string
	Mode    string // config.DemoLearning | config.DemoCalibrated
	Log     *slog.Logger
	Now     func() time.Time // defaults to time.Now
}

// studioFactor makes the demo studio dry a bit faster than the prior, so
// the calibrated model visibly learns something.
const studioFactor = 1.3

type seeder struct {
	Deps
	ctx context.Context
	now time.Time
}

// Seed writes the demo data if the database is empty, and does nothing
// otherwise.
func Seed(ctx context.Context, d Deps) error {
	q := d.Store.DB()
	n, err := store.CountPieces(ctx, q)
	if err != nil {
		return err
	}
	last, err := store.LastMinute(ctx, q)
	if err != nil {
		return err
	}
	if n > 0 || !last.IsZero() {
		d.Log.Info("demo seed skipped: database not empty")
		return nil
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	s := &seeder{Deps: d, ctx: ctx, now: d.Now().UTC().Truncate(time.Minute)}
	steps := []func() error{s.climate, s.history, s.leftOnShelf, s.session, s.settings}
	for _, step := range steps {
		if err := step(); err != nil {
			return err
		}
	}
	d.Log.Info("demo data seeded", "mode", d.Mode)
	return nil
}

func (s *seeder) ago(d time.Duration) time.Time { return s.now.Add(-d) }

func day(n float64) time.Duration  { return time.Duration(n * 24 * float64(time.Hour)) }
func hour(n float64) time.Duration { return time.Duration(n * float64(time.Hour)) }

// Offline period: the device was switched off with pieces on the shelf.
func (s *seeder) offline() (time.Time, time.Time) { return s.ago(day(6)), s.ago(day(3)) }

// climate writes 14 days of minutes (room and each sensor) around a damp
// tent, minus the offline period, and logs that period.
func (s *seeder) climate() error {
	offFrom, offTo := s.offline()
	var rows []store.Minute
	for t := s.ago(day(14)); t.Before(s.now); t = t.Add(time.Minute) {
		if !t.Before(offFrom) && t.Before(offTo) {
			continue
		}
		hours := float64(t.Unix()) / 3600
		temp := 18 + 1.5*math.Sin(2*math.Pi*hours/24)
		hum := 81 - 4*math.Sin(2*math.Pi*hours/24) + 2*math.Sin(2*math.Pi*hours/71)
		vpd := predict.VPD(temp, hum)
		m := store.Minute{Minute: t, HumAvg: hum, HumMin: hum - 0.3, HumMax: hum + 0.3, TempAvg: temp, VPDAvg: vpd, N: 12}
		m.Sensor = recorder.RoomSeries
		rows = append(rows, m)
		for _, name := range s.Sensors {
			m.Sensor = name
			rows = append(rows, m)
		}
	}
	err := s.Store.Tx(s.ctx, func(q store.Querier) error {
		if err := store.InsertMinutes(s.ctx, q, rows); err != nil {
			return err
		}
		value := map[string]string{"from": rfc(offFrom), "to": rfc(offTo)}
		_, err := store.InsertLog(s.ctx, q, store.LogOffline, value, offTo)
		return err
	})
	if err != nil {
		return err
	}
	return s.Dose.Load(s.ctx, s.Store.DB())
}

// piece describes one demo piece.
type piece struct {
	form, size, thickness, label string
	initial                      float64
}

// trueRate is how fast a demo piece "really" dries, per effective hour.
func trueRate(p piece, jitter float64) float64 {
	return predict.PriorRate(p.thickness, p.size) * studioFactor * jitter
}

// since returns when a piece must have been added at d0 to be at dNow now.
func (s *seeder) since(d0, dNow, rate float64) time.Time {
	need := (dNow - d0) / rate
	lo, hi := s.now.Add(-day(30)), s.now
	for hi.Sub(lo) > time.Minute {
		mid := lo.Add(hi.Sub(lo) / 2)
		if e, _ := s.Dose.Dose(mid, s.now); e >= need {
			lo = mid
		} else {
			hi = mid
		}
	}
	return lo.Truncate(time.Minute)
}

// reach returns when a piece added at start with dryness d0 reaches target.
func (s *seeder) reach(start time.Time, d0, target, rate float64) time.Time {
	need := (target - d0) / rate
	lo, hi := start, s.now.Add(day(30))
	for hi.Sub(lo) > time.Minute {
		mid := lo.Add(hi.Sub(lo) / 2)
		if e, _ := s.Dose.Dose(start, mid); e >= need {
			hi = mid
		} else {
			lo = mid
		}
	}
	return hi.Truncate(time.Minute)
}

func (s *seeder) add(p piece, qty int, at time.Time, photo *int64) ([]shelf.PieceView, error) {
	in := shelf.AddPieces{
		Quantity: &qty, Form: &p.form, Size: &p.size, Thickness: &p.thickness,
		InitialDryness: &p.initial, AddedAt: ptr(rfc(at)), PhotoID: photo,
	}
	if p.label != "" {
		in.Label = &p.label
	}
	return s.Shelf.Add(s.ctx, in)
}

func (s *seeder) observe(id int64, d float64, at time.Time) error {
	d = math.Round(d*10) / 10
	items := []shelf.NewObsItem{{PieceID: &id, Dryness: &d}}
	_, err := s.Shelf.Observe(s.ctx, shelf.NewObservations{ObservedAt: ptr(rfc(at)), Items: &items})
	return err
}

func (s *seeder) patch(ids []int64, p shelf.PiecePatch) error {
	_, err := s.Shelf.Update(s.ctx, ids, p, len(ids) > 1)
	return err
}

func set[T any](v T) shelf.Opt[T] { return shelf.Opt[T]{Set: true, V: v} }

// finished adds a piece that dried normally and was trimmed just past the
// target, so it yields a training sample.
func (s *seeder) finished(p piece, at time.Time, jitter float64, flip bool) error {
	added, err := s.add(p, 1, at, nil)
	if err != nil {
		return err
	}
	id, rate := added[0].ID, trueRate(p, jitter)
	mid := s.reach(at, p.initial, 2.0, rate)
	if err := s.observe(id, 2.0, mid); err != nil {
		return err
	}
	if flip {
		if err := s.patch([]int64{id}, shelf.PiecePatch{State: set("flipped"), FlippedAt: set(rfc(mid.Add(10 * time.Minute)))}); err != nil {
			return err
		}
	}
	near := s.reach(at, p.initial, 2.7, rate)
	if err := s.observe(id, 2.7, near); err != nil {
		return err
	}
	end := s.reach(at, p.initial, 3.1, rate)
	return s.patch([]int64{id}, shelf.PiecePatch{State: set("trimmed"), EndedAt: set(rfc(end)), EndDryness: set(3.1)})
}

// history is the earlier sessions, all off the shelf by now.
func (s *seeder) history() error {
	pieces := []piece{
		{"bowl", "medium", "medium", "", 1.0},
		{"mug", "small", "thin", "", 1.2},
		{"plate", "large", "medium", "", 0.8},
		{"vase", "medium", "thick", "", 1.0},
		{"bowl", "large", "medium", "", 1.1},
		{"mug", "medium", "medium", "", 1.0},
		{"other", "small", "medium", "teapot lid", 1.3},
	}
	jitter := []float64{1.0, 0.9, 1.1, 1.05, 0.95, 1.0, 0.92}
	n := len(pieces)
	if s.Mode == config.DemoLearning {
		n = 2
	}
	start := s.ago(day(13))
	for i := 0; i < n; i++ {
		at := start.Add(hour(float64(i) * 2))
		if err := s.finished(pieces[i], at, jitter[i], i%2 == 0); err != nil {
			return fmt.Errorf("finished piece %d: %w", i, err)
		}
	}

	// Covered early: trimmed at 2.4, no training sample.
	covered, err := s.add(piece{"bowl", "small", "thin", "", 1.0}, 1, start.Add(hour(3)), nil)
	if err != nil {
		return err
	}
	if err := s.patch([]int64{covered[0].ID}, shelf.PiecePatch{State: set("trimmed"), EndedAt: set(rfc(start.Add(hour(20)))), EndDryness: set(2.4),
		EndNote: set("covered to trim on Saturday")}); err != nil {
		return err
	}
	// Cracked, and one entered by mistake.
	cracked, err := s.add(piece{"plate", "xlarge", "thin", "", 1.0}, 1, start.Add(hour(4)), nil)
	if err != nil {
		return err
	}
	if err := s.patch([]int64{cracked[0].ID}, shelf.PiecePatch{State: set("discarded"), EndedAt: set(rfc(start.Add(hour(30)))), EndDryness: set(2.2),
		EndNote: set("cracked rim, reclaimed")}); err != nil {
		return err
	}
	oops, err := s.add(piece{"mug", "medium", "medium", "", 1.0}, 1, start.Add(hour(5)), nil)
	if err != nil {
		return err
	}
	return s.patch([]int64{oops[0].ID}, shelf.PiecePatch{State: set("discarded"), EndedAt: set(rfc(start.Add(hour(5.5)))), Mistake: set(true)})
}

// leftOnShelf adds pieces that were still listed when the device was
// switched off, and haven't been checked since it came back.
func (s *seeder) leftOnShelf() error {
	offFrom, _ := s.offline()
	at := offFrom.Add(-day(1))
	vase, err := s.add(piece{"vase", "large", "thick", "tall vase", 0.6}, 1, at, nil)
	if err != nil {
		return err
	}
	if err := s.observe(vase[0].ID, 1.1, offFrom.Add(-hour(3))); err != nil {
		return err
	}
	_, err = s.add(piece{"plate", "xlarge", "thick", "platter", 0.5}, 1, at.Add(hour(1)), nil)
	return err
}

// session is the current throwing session.
func (s *seeder) session() error {
	photo, err := s.Photos.Upload(s.ctx, testImage(color.RGBA{176, 120, 88, 255}))
	if err != nil {
		return err
	}
	start := s.ago(day(2))

	// A group of 4 identical mugs sharing a photo, flipped together. This
	// batch dries slower than usual; it is about 2.3 now.
	mug := piece{"mug", "medium", "medium", "", 1.0}
	rate := trueRate(mug, 0.75)
	added := s.since(mug.initial, 2.3, rate)
	group, err := s.add(mug, 4, added, &photo.ID)
	if err != nil {
		return err
	}
	ids := make([]int64, len(group))
	for i, p := range group {
		ids[i] = p.ID
	}
	flipAt := s.reach(added, mug.initial, 2.0, rate)
	if err := s.patch(ids, shelf.PiecePatch{State: set("flipped"), FlippedAt: set(rfc(flipAt)), FlipDryness: set(2.0)}); err != nil {
		return err
	}
	if err := s.observe(ids[0], 2.2, s.ago(hour(1.5))); err != nil {
		return err
	}

	// Ready now: a thick bowl checked an hour ago at 2.9.
	bowl, err := s.add(piece{"bowl", "large", "thick", "big serving bowl", 1.4}, 1, start.Add(hour(1)), nil)
	if err != nil {
		return err
	}
	if err := s.observe(bowl[0].ID, 2.2, s.ago(hour(20))); err != nil {
		return err
	}
	if err := s.observe(bowl[0].ID, 2.9, s.ago(hour(1))); err != nil {
		return err
	}

	// Past trim: a plate found at 3.8.
	plate, err := s.add(piece{"plate", "small", "thin", "", 1.2}, 1, start.Add(hour(2)), nil)
	if err != nil {
		return err
	}
	if err := s.observe(plate[0].ID, 3.8, s.ago(hour(6))); err != nil {
		return err
	}

	// Drying slowly, with an inconsistent (0.6 wetter) check.
	vase, err := s.add(piece{"vase", "large", "thick", "", 0.8}, 1, s.ago(day(1)), nil)
	if err != nil {
		return err
	}
	if err := s.observe(vase[0].ID, 1.4, s.ago(hour(12))); err != nil {
		return err
	}
	if err := s.observe(vase[0].ID, 0.8, s.ago(hour(3))); err != nil {
		return err
	}

	// May be past: a small thin cup nobody has checked since it was added.
	if _, err := s.add(piece{"mug", "small", "thin", "espresso cup", 1.0}, 1, s.ago(day(1.5)), nil); err != nil {
		return err
	}

	// Slow, thick pieces added an hour ago: days from leather hard.
	if _, err := s.add(piece{"bowl", "medium", "thick", "", 1.0}, 1, s.ago(hour(1)), nil); err != nil {
		return err
	}
	if _, err := s.add(piece{"plate", "large", "thick", "", 1.0}, 1, s.ago(hour(1)), nil); err != nil {
		return err
	}

	// Probably ready: added at 1.5, at a time chosen so the model now
	// expects it just past 2.8.
	cup, err := s.add(piece{"bowl", "small", "medium", "", 1.5}, 1, s.ago(hour(4)), nil)
	if err != nil {
		return err
	}
	return s.tune(cup[0].ID, predict.StatusProbablyReady)
}

// tune moves a piece's added_at (with its add observation) until the model
// gives it the wanted status. Earlier means drier.
func (s *seeder) tune(id int64, want string) error {
	lo, hi := s.ago(day(3)), s.ago(time.Minute) // hi: too fresh, lo: too dry
	for i := 0; i < 20; i++ {
		v, err := s.Shelf.Get(s.ctx, id)
		if err != nil {
			return err
		}
		switch v.Prediction.Status {
		case want:
			return nil
		case predict.StatusDrying:
			hi, _ = time.Parse(time.RFC3339, v.AddedAt)
		default:
			lo, _ = time.Parse(time.RFC3339, v.AddedAt)
		}
		mid := lo.Add(hi.Sub(lo) / 2).Truncate(time.Minute)
		if err := s.patch([]int64{id}, shelf.PiecePatch{AddedAt: set(rfc(mid))}); err != nil {
			return err
		}
	}
	s.Log.Warn("demo: couldn't tune piece", "id", id, "want", want)
	return nil
}

// settings calibrates the first sensor, so the climate log has an entry.
func (s *seeder) settings() error {
	if len(s.Sensors) == 0 {
		return nil
	}
	name, off := s.Sensors[0], 1.5
	sensors := []shelf.SensorPatch{{Sensor: &name, OffsetPct: &off}}
	_, err := s.Shelf.UpdateSettings(s.ctx, shelf.SettingsPatch{Sensors: set(sensors)})
	return err
}

// testImage draws a simple stand-in photo: a glaze-coloured pot on a bat.
func testImage(c color.RGBA) []byte {
	const w, h = 800, 600
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			bg := color.RGBA{uint8(225 - y/12), uint8(220 - y/12), uint8(210 - y/12), 255}
			dx, dy := float64(x-w/2)/220, float64(y-h/2)/170
			switch {
			case dx*dx+dy*dy < 1 && y > h/4:
				img.Set(x, y, color.RGBA{c.R - uint8(y/20), c.G - uint8(y/20), c.B - uint8(y/30), 255})
			case y > h*5/6:
				img.Set(x, y, color.RGBA{140, 110, 80, 255})
			default:
				img.Set(x, y, bg)
			}
		}
	}
	var b bytes.Buffer
	jpeg.Encode(&b, img, &jpeg.Options{Quality: 85})
	return b.Bytes()
}

func rfc(t time.Time) string { return t.UTC().Truncate(time.Second).Format(time.RFC3339) }

func ptr[T any](v T) *T { return &v }
