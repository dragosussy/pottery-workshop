package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

// Minute is one aggregated minute of one series (a sensor name, or "room").
type Minute struct {
	Sensor  string
	Minute  time.Time // start of the minute
	HumAvg  float64
	HumMin  float64
	HumMax  float64
	TempAvg float64
	VPDAvg  float64
	N       int
}

// InsertMinutes writes closed minutes. A minute that already exists is left
// alone, so a restart within the same minute can't overwrite it.
func InsertMinutes(ctx context.Context, q Querier, rows []Minute) error {
	stmt, err := q.PrepareContext(ctx, `INSERT INTO readings_1m
		(sensor, minute, hum_avg, hum_min, hum_max, temp_avg, vpd_avg, n) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (sensor, minute) DO NOTHING`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, m := range rows {
		_, err := stmt.ExecContext(ctx, m.Sensor, ms(m.Minute), m.HumAvg, m.HumMin, m.HumMax, m.TempAvg, m.VPDAvg, m.N)
		if err != nil {
			return err
		}
	}
	return nil
}

// LastMinute returns the latest stored minute of any series, or the zero
// time if nothing has been recorded.
func LastMinute(ctx context.Context, q Querier) (time.Time, error) {
	var v sql.NullInt64
	if err := q.QueryRowContext(ctx, `SELECT max(minute) FROM readings_1m`).Scan(&v); err != nil {
		return time.Time{}, err
	}
	if !v.Valid {
		return time.Time{}, nil
	}
	return fromMS(v.Int64), nil
}

// MinutesBetween returns a series' minutes in [from, to), oldest first.
func MinutesBetween(ctx context.Context, q Querier, sensor string, from, to time.Time) ([]Minute, error) {
	rows, err := q.QueryContext(ctx, `SELECT minute, coalesce(hum_avg, 0), coalesce(hum_min, 0), coalesce(hum_max, 0),
		coalesce(temp_avg, 0), vpd_avg, n FROM readings_1m
		WHERE sensor = ? AND minute >= ? AND minute < ? ORDER BY minute`, sensor, ms(from), ms(to))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Minute
	for rows.Next() {
		m := Minute{Sensor: sensor}
		var minute int64
		if err := rows.Scan(&minute, &m.HumAvg, &m.HumMin, &m.HumMax, &m.TempAvg, &m.VPDAvg, &m.N); err != nil {
			return nil, err
		}
		m.Minute = fromMS(minute)
		out = append(out, m)
	}
	return out, rows.Err()
}

// Climate log kinds.
const (
	LogOffline          = "offline"
	LogSensorGap        = "sensor_gap"
	LogSensorCalibrated = "sensor_calibrated"
	LogSensorReplaced   = "sensor_replaced"
	LogSetpointSet      = "setpoint_set"
)

// LogEntry is one row of climate_log. Value is raw JSON.
type LogEntry struct {
	ID    int64
	Kind  string
	Value json.RawMessage
	At    time.Time
}

// InsertLog appends an entry, marshalling value to JSON.
func InsertLog(ctx context.Context, q Querier, kind string, value any, at time.Time) (int64, error) {
	b, err := json.Marshal(value)
	if err != nil {
		return 0, err
	}
	res, err := q.ExecContext(ctx, `INSERT INTO climate_log (kind, value, at) VALUES (?, ?, ?)`, kind, string(b), ms(at))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// LogEntries returns every entry, oldest first. The log is small (a few
// rows per session), so callers filter in memory.
func LogEntries(ctx context.Context, q Querier) ([]LogEntry, error) {
	rows, err := q.QueryContext(ctx, `SELECT id, kind, value, at FROM climate_log ORDER BY at, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LogEntry
	for rows.Next() {
		var e LogEntry
		var v string
		var at int64
		if err := rows.Scan(&e.ID, &e.Kind, &v, &at); err != nil {
			return nil, err
		}
		e.Value = json.RawMessage(v)
		e.At = fromMS(at)
		out = append(out, e)
	}
	return out, rows.Err()
}

// GetSetting unmarshals the value stored under key into dst. It returns
// ErrNotFound if the key was never saved.
func GetSetting(ctx context.Context, q Querier, key string, dst any) error {
	var v string
	err := q.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(v), dst)
}

// PutSetting stores value as JSON under key.
func PutSetting(ctx context.Context, q Querier, key string, value any) error {
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = q.ExecContext(ctx, `INSERT INTO settings (key, value) VALUES (?, ?)
		ON CONFLICT (key) DO UPDATE SET value = excluded.value`, key, string(b))
	return err
}
