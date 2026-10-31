package shelf

import (
	"encoding/json"
	"math"
	"time"
)

// Opt is a PATCH field with merge semantics: absent (Set false), null
// (Null true), or a value.
type Opt[T any] struct {
	Set  bool
	Null bool
	V    T
}

// UnmarshalJSON records that the field was present. encoding/json calls it
// for null too, which is how null is told apart from absent.
func (o *Opt[T]) UnmarshalJSON(b []byte) error {
	o.Set = true
	if string(b) == "null" {
		o.Null = true
		return nil
	}
	return json.Unmarshal(b, &o.V)
}

// Value returns the value when set and not null.
func (o Opt[T]) Value() (T, bool) { return o.V, o.Set && !o.Null }

// ts formats a time as RFC 3339 UTC with second precision.
func ts(t time.Time) string { return t.UTC().Truncate(time.Second).Format(time.RFC3339) }

// tsp formats an optional time.
func tsp(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := ts(*t)
	return &s
}

// round1 rounds a dryness for output.
func round1(x float64) float64 { return math.Round(x*10) / 10 }

func round2(x float64) float64 { return math.Round(x*100) / 100 }

func ptr[T any](v T) *T { return &v }

// futureSlack is how far in the future a user-supplied time may be.
const futureSlack = 2 * time.Minute

// parseTime parses an RFC 3339 time with any offset. It records invalid_time
// or in_future on v.
func parseTime(v *ValidationError, field, s string, now time.Time) (time.Time, bool) {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		v.add(field, CodeInvalidTime, "must be an RFC 3339 time")
		return time.Time{}, false
	}
	if t.After(now.Add(futureSlack)) {
		v.add(field, CodeInFuture, "must not be in the future")
		return time.Time{}, false
	}
	return t.UTC().Truncate(time.Second), true
}

// dryness rounds to 0.1 and checks the range, recording out_of_range on v.
func dryness(v *ValidationError, field string, d float64) (float64, bool) {
	r := round1(d)
	if r < 0 || r > 5 {
		v.add(field, CodeOutOfRange, "must be between 0 and 5")
		return 0, false
	}
	return r + 0, true // + 0 turns -0 into 0
}
