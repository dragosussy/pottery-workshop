package app

import (
	"context"
	"database/sql"
	"log"
	"math"
	"strconv"
	"sync"
	"time"

	"atelier/apps/humidity/internal/hw"

	"atelier/shared/kit"
)

const (
	pollEvery = 5 * time.Second
	freshFor  = 30 * time.Second
)

// sensorValue is the latest from one sensor. at is zero before its first
// good read.
type sensorValue struct {
	addr        string
	offset      float64
	rawRH, temp float64
	at          time.Time
	err         string
}

// Live is what the sensor loop, the pump loop and the HTTP handlers share:
// the latest sensor values, the offsets and the target.
type Live struct {
	mu      sync.Mutex
	sensors []sensorValue // in SENSOR_ADDRS order
	target  int           // 0: none
}

// NewLive starts with no readings and the target and offsets saved in the
// settings table. addrs are formatted like "0x44".
func NewLive(db *sql.DB, addrs []string) (*Live, error) {
	settings, err := loadSettings(db)
	if err != nil {
		return nil, err
	}
	l := &Live{}
	l.target, _ = strconv.Atoi(settings["target"])
	for _, addr := range addrs {
		off, _ := strconv.ParseFloat(settings["offset:"+addr], 64)
		l.sensors = append(l.sensors, sensorValue{addr: addr, offset: off})
	}
	return l, nil
}

// Room is the room's state from the fresh sensors, offsets applied.
type Room struct {
	RH, Temp float64   // mean
	Max      float64   // highest RH, for the safety cutoff
	At       time.Time // newest read
}

// Room returns the room from the sensors read in the last 30 s; false if
// there are none.
func (l *Live) Room(now time.Time) (Room, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	var r Room
	n := 0
	for _, s := range l.sensors {
		if s.at.IsZero() || now.Sub(s.at) > freshFor {
			continue
		}
		rh := s.rawRH + s.offset
		r.RH += rh
		r.Temp += s.temp
		if n == 0 || rh > r.Max {
			r.Max = rh
		}
		if s.at.After(r.At) {
			r.At = s.at
		}
		n++
	}
	if n == 0 {
		return Room{}, false
	}
	r.RH /= float64(n)
	r.Temp /= float64(n)
	return r, true
}

func (l *Live) Target() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.target
}

func (l *Live) setTarget(pct int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.target = pct
}

func (l *Live) hasSensor(addr string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, s := range l.sensors {
		if s.addr == addr {
			return true
		}
	}
	return false
}

func (l *Live) setOffset(addr string, offset float64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for i := range l.sensors {
		if l.sensors[i].addr == addr {
			l.sensors[i].offset = offset
		}
	}
}

// record stores the result of reading sensor i. A failed read keeps the
// last good values, which go stale after 30 s.
func (l *Live) record(i int, rh, temp float64, at time.Time, err error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	s := &l.sensors[i]
	if err != nil {
		if s.err != err.Error() {
			log.Printf("sensor %s: %v", s.addr, err)
		}
		s.err = err.Error()
		return
	}
	if s.err != "" {
		log.Printf("sensor %s: reading again", s.addr)
	}
	s.rawRH, s.temp, s.at, s.err = rh, temp, at, ""
}

type roomJSON struct {
	RH   float64   `json:"rh"`
	Temp float64   `json:"temp"`
	At   time.Time `json:"at"`
}

type sensorJSON struct {
	Addr   string     `json:"addr"`
	RH     *float64   `json:"rh"`
	RawRH  *float64   `json:"raw_rh"`
	Temp   *float64   `json:"temp"`
	Offset float64    `json:"offset"`
	At     *time.Time `json:"at"`
	Error  *string    `json:"error"`
}

// sensorsJSON is the sensors part of Now.
func (l *Live) sensorsJSON() []sensorJSON {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := []sensorJSON{}
	for _, s := range l.sensors {
		j := sensorJSON{Addr: s.addr, Offset: s.offset}
		if !s.at.IsZero() {
			rh, raw, temp, at := round2(s.rawRH+s.offset), round2(s.rawRH), round2(s.temp), s.at
			j.RH, j.RawRH, j.Temp, j.At = &rh, &raw, &temp, &at
		}
		if s.err != "" {
			msg := s.err
			j.Error = &msg
		}
		out = append(out, j)
	}
	return out
}

func round2(x float64) float64 { return math.Round(x*100) / 100 }

// SensorLoop reads every sensor every 5 s, saves the room mean once a minute
// and takes the daily backup. It saves the open minute when ctx ends.
func (a *App) SensorLoop(ctx context.Context, sensors []hw.Sensor) {
	tick := time.NewTicker(pollEvery)
	defer tick.Stop()

	var minute int64 // the minute being averaged
	var sumRH, sumTemp float64
	var n int
	save := func() {
		if n == 0 {
			return
		}
		if err := saveMinute(a.DB, minute, sumRH/float64(n), sumTemp/float64(n)); err != nil {
			log.Printf("save reading: %v", err)
		}
		sumRH, sumTemp, n = 0, 0, 0
	}
	var backedUp string // day of the last backup

	for {
		now := time.Now().UTC().Truncate(time.Second)
		for i, s := range sensors {
			rh, temp, err := s.Read(ctx)
			a.Live.record(i, rh, temp, now, err)
		}

		if room, ok := a.Live.Room(now); ok {
			if m := now.Unix() / 60 * 60; m != minute {
				save()
				minute = m
			}
			sumRH += room.RH
			sumTemp += room.Temp
			n++
		}

		if day := now.Format(time.DateOnly); day != backedUp {
			if err := kit.DailySnapshot(a.DB, a.DataDir, now); err != nil {
				log.Printf("backup: %v", err) // tried again tomorrow
			}
			backedUp = day
		}

		select {
		case <-ctx.Done():
			save()
			return
		case <-tick.C:
		}
	}
}
