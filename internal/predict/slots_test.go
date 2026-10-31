package predict

import (
	"testing"
	"time"
)

func mustLoc(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

func TestSlots_NowThenHalfHoursAroundTheClock(t *testing.T) {
	loc := mustLoc(t, "Europe/Bucharest")
	now := time.Date(2026, 10, 14, 22, 10, 40, 0, loc)
	slots := Slots(now, loc)

	if !slots[0].Equal(now) {
		t.Errorf("first slot = %v, want now", slots[0])
	}
	if want := time.Date(2026, 10, 14, 22, 30, 0, 0, loc); !slots[1].Equal(want) {
		t.Errorf("second slot = %v, want %v", slots[1], want)
	}
	// Night hours are slots too.
	if want := time.Date(2026, 10, 15, 3, 0, 0, 0, loc); !slots[10].Equal(want) {
		t.Errorf("slot 10 = %v, want %v", slots[10], want)
	}
	// 22:30 on day 0 to 22:00 on day 7: 7 × 48 half hours.
	if want := 1 + 7*48; len(slots) != want {
		t.Errorf("len(slots) = %d, want %d", len(slots), want)
	}
	for i, s := range slots {
		if s.Before(now) || !s.Before(now.Add(Horizon)) {
			t.Errorf("slot %v outside [now, now+7d)", s)
		}
		if i > 0 && (s.In(loc).Minute()%30 != 0 || s.Second() != 0) {
			t.Errorf("slot %v off the :00/:30 grid", s.In(loc))
		}
	}
}

func TestSlots_NowOnTheGridIsNotDuplicated(t *testing.T) {
	now := time.Date(2026, 10, 14, 9, 0, 0, 0, time.UTC)
	slots := Slots(now, time.UTC)
	if !slots[0].Equal(now) || !slots[1].Equal(now.Add(SlotStep)) {
		t.Errorf("slots start %v, %v", slots[0], slots[1])
	}
}

func TestSlots_GridFollowsLocalMarks(t *testing.T) {
	// Kathmandu is UTC+05:45: local :00 and :30 are UTC :15 and :45.
	loc := mustLoc(t, "Asia/Kathmandu")
	now := time.Date(2026, 10, 14, 10, 5, 0, 0, loc)
	slots := Slots(now, loc)
	if want := time.Date(2026, 10, 14, 10, 30, 0, 0, loc); !slots[1].Equal(want) {
		t.Errorf("second slot = %v, want %v", slots[1].In(loc), want)
	}
}

func TestSlots_DST(t *testing.T) {
	loc := mustLoc(t, "Europe/Bucharest")
	perDay := func(slots []time.Time, day int) int {
		n := 0
		for _, s := range slots {
			if s.In(loc).Day() == day {
				n++
			}
		}
		return n
	}
	check := func(t *testing.T, slots []time.Time) {
		for i := 1; i < len(slots); i++ {
			if i > 1 && slots[i].Sub(slots[i-1]) != SlotStep {
				t.Errorf("slots %v and %v not 30 min apart", slots[i-1], slots[i])
			}
			if l := slots[i].In(loc); l.Minute()%30 != 0 {
				t.Errorf("slot %v off the local grid", l)
			}
		}
	}

	t.Run("fall back: a 25-hour day", func(t *testing.T) {
		// 2026-10-25 04:00 EEST → 03:00 EET.
		slots := Slots(time.Date(2026, 10, 24, 12, 0, 0, 0, loc), loc)
		check(t, slots)
		if n := perDay(slots, 25); n != 50 {
			t.Errorf("slots on the 25th = %d, want 50", n)
		}
	})
	t.Run("spring forward: a 23-hour day", func(t *testing.T) {
		// 2026-03-29 03:00 EET → 04:00 EEST.
		slots := Slots(time.Date(2026, 3, 28, 12, 0, 0, 0, loc), loc)
		check(t, slots)
		if n := perDay(slots, 29); n != 46 {
			t.Errorf("slots on the 29th = %d, want 46", n)
		}
	})
}
