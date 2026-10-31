package api

import (
	"fmt"
	"math"
	"net/http"
	"time"

	"humidity/internal/monitor"
	"humidity/internal/predict"
	"humidity/internal/shelf"
)

// Sensor and room states.
const (
	stateOK     = "ok"
	stateStale  = "stale"
	stateNoData = "no_data"
)

type roomView struct {
	HumidityPct  *float64 `json:"humidity_pct"`
	TemperatureC *float64 `json:"temperature_c"`
	VPDKPa       *float64 `json:"vpd_kpa"`
	State        string   `json:"state"`
	ReadAt       *string  `json:"read_at"`
}

type sensorView struct {
	Sensor       string   `json:"sensor"`
	State        string   `json:"state"`
	HumidityPct  *float64 `json:"humidity_pct"`
	TemperatureC *float64 `json:"temperature_c"`
	OffsetPct    float64  `json:"offset_pct"`
	ReadAt       *string  `json:"read_at"`
	AgeSeconds   *int64   `json:"age_seconds"`
	FailingSince *string  `json:"failing_since"`
	LastError    *string  `json:"last_error"`
}

func ts(t time.Time) string { return t.UTC().Truncate(time.Second).Format(time.RFC3339) }

func tsp(t time.Time) *string {
	if t.IsZero() {
		return nil
	}
	s := ts(t)
	return &s
}

func round(x float64, digits int) *float64 {
	p := math.Pow(10, float64(digits))
	v := math.Round(x*p) / p
	return &v
}

// climate builds the room and per-sensor views from a cycle. The room is
// the mean of the ok sensors; with none ok, the mean of the stale ones.
func (s *Server) climate(c monitor.Cycle, now time.Time) (roomView, []sensorView) {
	stale := s.staleAfter()
	sensors := make([]sensorView, 0, len(c.Sensors))
	var okH, okT, stH, stT []float64
	var okAt, stAt time.Time
	for _, st := range c.Sensors {
		v := sensorView{Sensor: st.Sensor, State: stateNoData, OffsetPct: st.Offset, FailingSince: tsp(st.FailingSince)}
		if st.LastError != "" {
			e := st.LastError
			v.LastError = &e
		}
		if r := st.Reading; !r.At.IsZero() {
			age := now.Sub(r.At)
			v.HumidityPct, v.TemperatureC = round(r.Humidity, 1), round(r.Temperature, 1)
			v.ReadAt = tsp(r.At)
			v.AgeSeconds = new(int64)
			*v.AgeSeconds = max(int64(age/time.Second), 0)
			if age <= stale {
				v.State = stateOK
				okH, okT = append(okH, r.Humidity), append(okT, r.Temperature)
				if r.At.After(okAt) {
					okAt = r.At
				}
			} else {
				v.State = stateStale
				stH, stT = append(stH, r.Humidity), append(stT, r.Temperature)
				if r.At.After(stAt) {
					stAt = r.At
				}
			}
		}
		sensors = append(sensors, v)
	}
	room := roomView{State: stateNoData}
	hs, tsv, at, state := okH, okT, okAt, stateOK
	if len(hs) == 0 {
		hs, tsv, at, state = stH, stT, stAt, stateStale
	}
	if len(hs) > 0 {
		h, t := mean(hs), mean(tsv)
		room = roomView{
			HumidityPct: round(h, 1), TemperatureC: round(t, 1), VPDKPa: round(predict.VPD(t, h), 2),
			State: state, ReadAt: tsp(at),
		}
	}
	return room, sensors
}

func mean(xs []float64) float64 {
	sum := 0.0
	for _, x := range xs {
		sum += x
	}
	return sum / float64(len(xs))
}

type readingsEvent struct {
	ServerTime string       `json:"server_time"`
	Room       roomView     `json:"room"`
	Sensors    []sensorView `json:"sensors"`
}

func (s *Server) readingsEvent(c monitor.Cycle) readingsEvent {
	now := s.now()
	room, sensors := s.climate(c, now)
	return readingsEvent{ServerTime: ts(now), Room: room, Sensors: sensors}
}

type healthBody struct {
	Status      string            `json:"status"`
	Version     string            `json:"version"`
	Commit      string            `json:"commit"`
	BuiltAt     *string           `json:"built_at"`
	APIVersion  int               `json:"api_version"`
	ServerTime  string            `json:"server_time"`
	ClockSynced bool              `json:"clock_synced"`
	StartedAt   string            `json:"started_at"`
	Checks      map[string]string `json:"checks"`
	Features    map[string]bool   `json:"features"`
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	if err := s.Store.Ping(r.Context()); err != nil {
		s.log.Error("health: database", "err", err)
		writeError(w, http.StatusServiceUnavailable, "unavailable", "the database is not reachable")
		return
	}
	now := s.now()
	_, sensors := s.climate(s.Monitor.Last(), now)
	sensorState := stateOK
	for _, v := range sensors {
		if v.State == stateNoData || v.State == stateStale && sensorState == stateOK {
			sensorState = v.State
		}
	}
	body := healthBody{
		Status: "ok", Version: s.Version, Commit: s.Commit, APIVersion: 1,
		ServerTime: ts(now), ClockSynced: s.ClockSynced(), StartedAt: ts(s.StartedAt),
		Checks:   map[string]string{"database": "ok", "sensors": sensorState},
		Features: map[string]bool{"control": false},
	}
	if s.BuiltAt != "" {
		body.BuiltAt = &s.BuiltAt
	}
	if sensorState != stateOK {
		body.Status = "degraded"
	}
	writeJSON(w, http.StatusOK, body)
}

type statusBody struct {
	ServerTime          string             `json:"server_time"`
	PollIntervalSeconds int64              `json:"poll_interval_seconds"`
	StaleAfterSeconds   int64              `json:"stale_after_seconds"`
	Room                roomView           `json:"room"`
	Sensors             []sensorView       `json:"sensors"`
	Shelf               shelf.ShelfSummary `json:"shelf"`
	Trim                shelf.TrimSummary  `json:"trim"`
	Learning            shelf.LearningView `json:"learning"`
	LastOffline         *shelf.OfflineView `json:"last_offline"`
	Control             *struct{}          `json:"control"`
}

func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	parts, err := s.Shelf.Status(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	now := s.now()
	room, sensors := s.climate(s.Monitor.Last(), now)
	writeJSON(w, http.StatusOK, statusBody{
		ServerTime:          ts(now),
		PollIntervalSeconds: int64(s.Monitor.Interval() / time.Second),
		StaleAfterSeconds:   int64(s.staleAfter() / time.Second),
		Room:                room,
		Sensors:             sensors,
		Shelf:               parts.Shelf,
		Trim:                parts.Trim,
		Learning:            parts.Learning,
		LastOffline:         parts.LastOffline,
	})
}

// stream serves Server-Sent Events (api-contract §4).
func (s *Server) stream(w http.ResponseWriter, r *http.Request) {
	c, err := s.Hub.register()
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "too many open streams")
		return
	}
	defer s.Hub.unregister(c)

	rc := http.NewResponseController(w)
	rc.SetWriteDeadline(time.Time{}) // the server's WriteTimeout doesn't apply to streams
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	send := func(event string, data []byte) error {
		_, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data)
		return err
	}
	now := s.now()
	hello, _ := jsonBytes(map[string]any{
		"server_time":           ts(now),
		"version":               s.Version,
		"api_version":           1,
		"poll_interval_seconds": int64(s.Monitor.Interval() / time.Second),
		"stale_after_seconds":   int64(s.staleAfter() / time.Second),
		"clock_synced":          s.ClockSynced(),
	})
	readings, _ := jsonBytes(s.readingsEvent(s.Monitor.Last()))
	fmt.Fprint(w, "retry: 5000\n\n")
	send("hello", hello)
	send("readings", readings)
	if rc.Flush() != nil {
		return
	}

	ping := time.NewTicker(20 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-c.done:
			return
		case <-ping.C:
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil || rc.Flush() != nil {
				return
			}
		case <-c.wake:
			for _, m := range c.take() {
				if send(m.event, m.data) != nil {
					return
				}
			}
			if rc.Flush() != nil {
				return
			}
		}
	}
}
