package sim

import (
	"fmt"
	"strings"
)

const (
	TicksPerHour = 4
	TicksPerDay  = 24 * TicksPerHour // 96 quarter-hour ticks per day
	MaxHealth    = 100
	MaxEnergy    = 100
	MaxCondition = 100
)

const (
	SleepDeep      = "deep_sleeper"
	SleepLight     = "light_sleeper"
	SleepEarlyBird = "early_bird"
	SleepNightOwl  = "night_owl"
)

// Clock represents the simulation wall-clock derived from quarter-hour ticks (tick 0 = Day 1 06:00).
type Clock struct {
	Day     int    `json:"day"`
	Hour    int    `json:"hour"`
	Minute  int    `json:"minute"`
	IsNight bool   `json:"is_night"`
	Label   string `json:"label"`
}

func ClockAt(tick int64) Clock {
	total := tick + 6*TicksPerHour
	day := int(total/TicksPerDay) + 1
	inDay := int(total % TicksPerDay)
	hour := inDay / TicksPerHour
	minute := (inDay % TicksPerHour) * 15
	return Clock{
		Day:     day,
		Hour:    hour,
		Minute:  minute,
		IsNight: hour >= 22 || hour < 6,
		Label:   fmt.Sprintf("day %d %02d:%02d", day, hour, minute),
	}
}

// House is an independent property asset anchored at a village district (and optional sub-district Venue).
type House struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	District   string   `json:"district"`
	VenueID    string   `json:"venue_id,omitempty"`
	OwnerID    string   `json:"owner_id,omitempty"`
	Residents  []string `json:"residents,omitempty"`
	Level      int      `json:"level"`
	Comfort    int      `json:"comfort"`
	Condition  int      `json:"condition"`
	Shielding  int      `json:"shielding,omitempty"`
	Facilities []string `json:"facilities,omitempty"`
}

func (h House) CanAccess(actorID string) bool {
	if actorID == "" {
		return false
	}
	if h.OwnerID == actorID {
		return true
	}
	for _, r := range h.Residents {
		if r == actorID {
			return true
		}
	}
	return false
}

func (h House) HasFacility(facility string) bool {
	for _, f := range h.Facilities {
		if f == facility {
			return true
		}
	}
	return false
}

func CloneHouse(h House) House {
	out := h
	if h.Residents != nil {
		out.Residents = append([]string(nil), h.Residents...)
	}
	if h.Facilities != nil {
		out.Facilities = append([]string(nil), h.Facilities...)
	}
	return out
}

// SleepRecoveryTenths computes energy recovery in tenths of an energy point (10 = +1 Energy)
// for one 15-minute tick of sleep from four factors:
// 1. Housing asset comfort and structural condition (vs. outdoor exposure)
// 2. Personal sleep trait and circadian time-of-day alignment
// 3. Physiological interference (hunger and injury pain)
// 4. Livelihood scarcity and temperament anxiety
func SleepRecoveryTenths(a Actor, house *House, tick int64) (int, string) {
	c := ClockAt(tick)
	tenths := 24
	var notes []string

	if house != nil && a.InsideHouse == house.ID {
		if house.Level > 1 {
			tenths += (house.Level - 1) * 2
		}
		switch {
		case house.Condition >= 80:
			tenths += house.Comfort*5 + 4
			notes = append(notes, "sound_home")
		case house.Condition >= 50:
			tenths += house.Comfort * 5
			notes = append(notes, "modest_shelter")
		case house.Condition >= 25:
			tenths += house.Comfort*2 - 4
			notes = append(notes, "drafty_roof")
		default:
			tenths -= 6
			notes = append(notes, "ruined_shelter")
		}
	} else {
		tenths -= 8
		notes = append(notes, "exposed_outdoors")
	}

	switch a.SleepTrait {
	case SleepDeep:
		if c.IsNight {
			tenths += 8
		} else {
			tenths += 5
		}
		notes = append(notes, SleepDeep)
	case SleepLight:
		if c.Hour >= 7 && c.Hour < 21 {
			tenths -= 9
			notes = append(notes, "daylight_noise")
		} else {
			tenths -= 3
			notes = append(notes, SleepLight)
		}
	case SleepEarlyBird:
		if c.Hour >= 21 || c.Hour < 5 {
			tenths += 6
			notes = append(notes, "aligned_early_bird")
		} else {
			tenths -= 4
		}
	case SleepNightOwl:
		if c.Hour < 8 {
			tenths += 6
			notes = append(notes, "aligned_night_owl")
		} else if c.Hour >= 20 {
			tenths -= 4
			notes = append(notes, "too_early_for_owl")
		}
	}
	if c.Hour >= 8 && c.Hour < 20 {
		tenths -= 4
	}

	if a.Hunger > 0 {
		h := a.Hunger
		if h > 3 {
			h = 3
		}
		tenths -= 4 * h
		notes = append(notes, "hungry")
	}
	if a.Health < MaxHealth {
		pain := ((MaxHealth - a.Health + 9) / 10) * 2
		tenths -= pain
		notes = append(notes, "injured")
	}

	food := a.Items["food"]
	if food < 2 {
		tenths -= 4
		notes = append(notes, "low_food_anxiety")
	}
	if a.Money < 3 {
		tenths -= 3
		notes = append(notes, "low_coin_anxiety")
	}
	if (food < 3 || a.Money < 4) && (strings.Contains(a.Personality, "skeptical") || strings.Contains(a.Personality, "careful")) {
		tenths -= 3
		notes = append(notes, "anxious_temperament")
	}

	if tenths < 8 {
		tenths = 8
	}
	return tenths, strings.Join(notes, ",")
}

// SleepWindowAligned reports whether the current clock hour is within the actor's preferred nightly sleep window.
func SleepWindowAligned(trait string, hour int) bool {
	switch trait {
	case SleepEarlyBird:
		return hour >= 21 || hour < 5
	case SleepNightOwl:
		return hour >= 0 && hour < 8
	default:
		return hour >= 22 || hour < 6
	}
}
