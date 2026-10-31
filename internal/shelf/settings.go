package shelf

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"time"

	"humidity/internal/predict"
	"humidity/internal/store"
)

// Settings keys. Rows left from the removed studio schedule
// (studio_days, studio_hours, studio_configured) are ignored.
const (
	keyTZ      = "studio_tz"
	keyOffsets = "sensor_offsets"
)

// SensorSetting is one sensor's calibration.
type SensorSetting struct {
	Sensor       string  `json:"sensor"`
	OffsetPct    float64 `json:"offset_pct"`
	CalibratedAt *string `json:"calibrated_at"`
}

// Settings is the GET /settings body (api-contract §3.10). The studio time
// zone is used for display and studio-local day boundaries.
type Settings struct {
	StudioTZ string          `json:"studio_tz"`
	Sensors  []SensorSetting `json:"sensors"`
}

// SettingsPatch is the PATCH /settings body.
type SettingsPatch struct {
	StudioTZ Opt[string]        `json:"studio_tz"`
	Sensors  Opt[[]SensorPatch] `json:"sensors"`
}

// SensorPatch calibrates (and optionally marks as replaced) one sensor.
type SensorPatch struct {
	Sensor    *string  `json:"sensor"`
	OffsetPct *float64 `json:"offset_pct"`
	Replaced  *bool    `json:"replaced"`
}

type storedOffset struct {
	OffsetPct    float64 `json:"offset_pct"`
	CalibratedAt int64   `json:"calibrated_at"` // unix ms
}

// Settings returns the current settings, with defaults for anything never
// saved.
func (s *Service) Settings(ctx context.Context) (Settings, error) {
	return s.settings(ctx, s.st.DB())
}

func (s *Service) settings(ctx context.Context, q store.Querier) (Settings, error) {
	out := Settings{StudioTZ: s.defaultTZ, Sensors: []SensorSetting{}}
	if err := store.GetSetting(ctx, q, keyTZ, &out.StudioTZ); err != nil && !errors.Is(err, store.ErrNotFound) {
		return Settings{}, err
	}
	offsets, err := s.storedOffsets(ctx, q)
	if err != nil {
		return Settings{}, err
	}
	for _, name := range s.sensors {
		ss := SensorSetting{Sensor: name}
		if o, ok := offsets[name]; ok {
			ss.OffsetPct = o.OffsetPct
			ss.CalibratedAt = tsp(ptr(time.UnixMilli(o.CalibratedAt)))
		}
		out.Sensors = append(out.Sensors, ss)
	}
	return out, nil
}

func (s *Service) storedOffsets(ctx context.Context, q store.Querier) (map[string]storedOffset, error) {
	offsets := map[string]storedOffset{}
	if err := store.GetSetting(ctx, q, keyOffsets, &offsets); err != nil && !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	return offsets, nil
}

// Offsets returns the calibration offsets to apply at startup.
func (s *Service) Offsets(ctx context.Context) (map[string]float64, error) {
	stored, err := s.storedOffsets(ctx, s.st.DB())
	if err != nil {
		return nil, err
	}
	out := make(map[string]float64, len(stored))
	for k, v := range stored {
		out[k] = v.OffsetPct
	}
	return out, nil
}

// loc returns the studio time zone. Settings are validated on save, so a
// load error falls back to UTC.
func (st Settings) loc() *time.Location {
	loc, err := time.LoadLocation(st.StudioTZ)
	if err != nil {
		return time.UTC
	}
	return loc
}

// UpdateSettings applies a patch (api-contract §3.10).
func (s *Service) UpdateSettings(ctx context.Context, patch SettingsPatch) (Settings, error) {
	v := &ValidationError{}
	now := s.Now()

	var tz string
	if patch.StudioTZ.Set {
		t, ok := patch.StudioTZ.Value()
		if _, err := time.LoadLocation(t); !ok || t == "" || err != nil {
			v.add("studio_tz", CodeInvalidValue, "must be an IANA time zone, e.g. Europe/Bucharest")
		}
		tz = t
	}

	var sensors []SensorPatch
	if patch.Sensors.Set {
		sensors, _ = patch.Sensors.Value()
		seen := map[string]bool{}
		for i, sp := range sensors {
			field := fmt.Sprintf("sensors[%d]", i)
			switch {
			case sp.Sensor == nil:
				v.add(field+".sensor", CodeRequired, "required")
			case !slices.Contains(s.sensors, *sp.Sensor):
				v.add(field+".sensor", CodeInvalidValue, "no sensor with this name")
			case seen[*sp.Sensor]:
				v.add(field+".sensor", CodeDuplicate, "listed twice")
			default:
				seen[*sp.Sensor] = true
			}
			if sp.OffsetPct == nil {
				v.add(field+".offset_pct", CodeRequired, "required")
			} else if math.Abs(*sp.OffsetPct) > 10 {
				v.add(field+".offset_pct", CodeOutOfRange, "must be between -10 and 10")
			}
		}
	}
	if err := v.err(); err != nil {
		return Settings{}, err
	}

	scopes := []string{ScopeSettings}
	var offsets map[string]float64
	err := s.st.Tx(ctx, func(q store.Querier) error {
		if patch.StudioTZ.Set {
			if err := store.PutSetting(ctx, q, keyTZ, tz); err != nil {
				return err
			}
		}
		if len(sensors) == 0 {
			return nil
		}
		stored, err := s.storedOffsets(ctx, q)
		if err != nil {
			return err
		}
		for _, sp := range sensors {
			name := *sp.Sensor
			if sp.Replaced != nil && *sp.Replaced {
				if _, err := store.InsertLog(ctx, q, store.LogSensorReplaced, map[string]string{"sensor": name}, now); err != nil {
					return err
				}
			}
			prev := stored[name].OffsetPct
			off := round1(*sp.OffsetPct) + 0
			stored[name] = storedOffset{OffsetPct: off, CalibratedAt: now.UnixMilli()}
			value := map[string]any{"sensor": name, "offset_pct": off, "previous_offset_pct": prev}
			if _, err := store.InsertLog(ctx, q, store.LogSensorCalibrated, value, now); err != nil {
				return err
			}
		}
		offsets = make(map[string]float64, len(stored))
		for k, o := range stored {
			offsets[k] = o.OffsetPct
		}
		scopes = append(scopes, ScopeClimateLog)
		return store.PutSetting(ctx, q, keyOffsets, stored)
	})
	if err != nil {
		return Settings{}, err
	}
	if offsets != nil {
		s.offsets.SetOffsets(offsets)
	}
	if patch.StudioTZ.Set {
		scopes = append(scopes, ScopePredictions)
	}
	s.changed(scopes, nil)
	return s.Settings(ctx)
}

// intervals extracts offline periods and sensor gaps from the climate log,
// oldest first.
func intervals(entries []store.LogEntry) (offline, gaps []predict.Interval) {
	for _, e := range entries {
		var v struct{ From, To string }
		if e.Kind != store.LogOffline && e.Kind != store.LogSensorGap {
			continue
		}
		if json.Unmarshal(e.Value, &v) != nil {
			continue
		}
		from, err1 := time.Parse(time.RFC3339, v.From)
		to, err2 := time.Parse(time.RFC3339, v.To)
		if err1 != nil || err2 != nil {
			continue
		}
		iv := predict.Interval{From: from, To: to}
		if e.Kind == store.LogOffline {
			offline = append(offline, iv)
		} else {
			gaps = append(gaps, iv)
		}
	}
	slices.SortFunc(offline, func(a, b predict.Interval) int { return a.To.Compare(b.To) })
	return offline, gaps
}
