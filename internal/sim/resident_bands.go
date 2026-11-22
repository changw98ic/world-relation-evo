package sim

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// ResidentBand describes Count consecutive residents with identical physiology,
// per-person inventory, and availability state.
// Origin and ordinal follow the resident across cohort boundaries.
type ResidentBand struct {
	Origin           string         `json:"origin"`
	First            int            `json:"first"`
	Count            int            `json:"count"`
	NutritionReserve int64          `json:"nutrition_reserve,omitempty"`
	Hunger           int            `json:"hunger"`
	Health           int            `json:"health"`
	Items            map[string]int `json:"items,omitempty"`
	Sick             bool           `json:"sick,omitempty"`
	Jailed           bool           `json:"jailed,omitempty"`
}

func (b ResidentBand) ID(offset int) string {
	return fmt.Sprintf("%s_a%04d", b.Origin, b.First+offset)
}

func cloneResidentBand(b ResidentBand) ResidentBand {
	b.Items = cloneIntMap(b.Items)
	return b
}

func equalIntMaps(a, b map[string]int) bool {
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

func appendResidentBand(dst []ResidentBand, b ResidentBand) []ResidentBand {
	if b.Count == 0 {
		return dst
	}
	b.Items = cloneIntMap(b.Items)
	if len(dst) > 0 {
		last := &dst[len(dst)-1]
		if last.Origin == b.Origin && last.First+last.Count == b.First &&
			last.Hunger == b.Hunger && last.NutritionReserve == b.NutritionReserve && last.Health == b.Health &&
			last.Sick == b.Sick && last.Jailed == b.Jailed &&
			equalIntMaps(last.Items, b.Items) {
			last.Count += b.Count
			return dst
		}
	}
	return append(dst, b)
}

// ResidentSeq is the last ordinal issued by this origin. It remains on the
// issuing cohort even after all its residents move or die.
func (c *PopulationCohort) issueResidents(count, hunger int) {
	c.Residents = appendResidentBand(c.Residents, ResidentBand{Origin: c.ID, First: c.ResidentSeq + 1, Count: count, Hunger: hunger, Health: MaxHealth})
	c.ResidentSeq += count
}

func (c *PopulationCohort) ensureResidents(population int) {
	if len(c.Residents) != 0 || population == 0 || c.ResidentSeq != 0 {
		return
	}
	c.seedResidents(population, c.HydratedSeq)
}

func (c *PopulationCohort) seedResidents(population, issued int) {
	c.ResidentSeq = issued
	credit := int(c.HungerReliefRemainder)
	c.issueResidents(credit, c.AvgHunger-1)
	c.issueResidents(population-credit, c.AvgHunger)
}

// InitCohortResidents reserves existing personal IDs before allocating the starting census.
func (w *World) InitCohortResidents() error {
	issued := map[string]int{}
	var occupied []ResidentBand
	reserveActor := func(id string) {
		k := strings.LastIndex(id, "_a")
		if k < 0 {
			return
		}
		n, err := strconv.Atoi(id[k+2:])
		if err != nil || n < 1 {
			return
		}
		b := ResidentBand{Origin: id[:k], First: n, Count: 1}
		if b.ID(0) != id {
			return
		}
		occupied = append(occupied, b)
		issued[b.Origin] = max(issued[b.Origin], n)
	}
	for _, a := range w.Actors {
		reserveActor(a.ID)
	}
	for _, c := range w.Cohorts {
		for _, a := range c.Members {
			reserveActor(a.ID)
		}
		for _, b := range c.Residents {
			if b.Origin == "" || b.First < 1 || b.Count < 1 || b.Count-1 > math.MaxInt-b.First || b.Health < 1 || b.Health > MaxHealth || b.Hunger < 0 || b.NutritionReserve < 0 {
				return fmt.Errorf("cohort %s has invalid resident band %+v", c.ID, b)
			}
			occupied = append(occupied, b)
			issued[b.Origin] = max(issued[b.Origin], b.First+(b.Count-1))
		}
	}
	sort.Slice(occupied, func(i, j int) bool {
		a, b := occupied[i], occupied[j]
		if a.Origin != b.Origin {
			return a.Origin < b.Origin
		}
		return a.First < b.First
	})
	for i := 1; i < len(occupied); i++ {
		a, b := occupied[i-1], occupied[i]
		if a.Origin == b.Origin && b.First-a.First < a.Count {
			return fmt.Errorf("overlapping resident ID %s", b.ID(0))
		}
	}
	for i := range w.Cohorts {
		c := &w.Cohorts[i]
		pop := w.anonymousCohort(*c).Population
		if err := c.ValidateHungerBalance(); err != nil {
			return err
		}
		last := max(issued[c.ID], c.ResidentSeq)
		if len(c.Residents) == 0 && c.ResidentSeq == 0 {
			last = max(last, c.HydratedSeq)
			if pop > math.MaxInt-last {
				return fmt.Errorf("cohort %s starting resident IDs overflow", c.ID)
			}
			c.seedResidents(pop, last)
			continue
		}
		remaining := pop
		var hunger int64
		for _, b := range c.Residents {
			if b.Count > remaining {
				return fmt.Errorf("cohort %s has more resident IDs than its anonymous census", c.ID)
			}
			remaining -= b.Count
			hunger += int64(b.Count) * int64(b.Hunger)
		}
		if remaining != 0 || hunger != c.hungerUnits(pop) {
			return fmt.Errorf("cohort %s resident bands disagree with anonymous population or hunger", c.ID)
		}
		c.ResidentSeq = last
	}
	return nil
}

// updateResidentBandState updates up to count matching residents in-place,
// splitting a band only when count < b.Count and coalescing adjacent identical bands.
func (c *PopulationCohort) updateResidentBandState(count int, match func(ResidentBand) bool, apply func(*ResidentBand)) int {
	if c == nil || count <= 0 || len(c.Residents) == 0 {
		return 0
	}
	remaining := count
	var out []ResidentBand
	for _, b := range c.Residents {
		if remaining > 0 && match(b) {
			take := min(remaining, b.Count)
			changed := cloneResidentBand(b)
			changed.Count = take
			apply(&changed)
			out = appendResidentBand(out, changed)
			remaining -= take
			if take < b.Count {
				rest := cloneResidentBand(b)
				rest.First += take
				rest.Count -= take
				out = appendResidentBand(out, rest)
			}
			continue
		}
		out = appendResidentBand(out, b)
	}
	c.Residents = out
	return count - remaining
}

// popHealthyFreeResident extracts a single healthy, non-sick, non-jailed resident
// when available (falling back to index 0 only if no matching band exists).
func (c *PopulationCohort) popHealthyFreeResident() ResidentBand {
	pick := 0
	for i, b := range c.Residents {
		if b.Count > 0 && !b.Jailed && !b.Sick && b.Health >= MaxHealth {
			pick = i
			break
		}
	}
	b := c.Residents[pick]
	out := cloneResidentBand(b)
	out.Count = 1
	c.Residents[pick].First++
	c.Residents[pick].Count--
	if c.Residents[pick].Count == 0 {
		c.Residents = append(c.Residents[:pick], c.Residents[pick+1:]...)
	}
	if len(c.Residents) == 0 {
		c.Residents = nil
	}
	return out
}

// moveResidents removes a deterministic prefix. A nil destination retires the IDs
// and returns any personally held ResidentBand.Items back into c.ItemPool so
// item conservation holds across all non-starvation cohort deaths.
func (c *PopulationCohort) moveResidents(to *PopulationCohort, count int) (hunger int64) {
	for count > 0 {
		b := c.Residents[0]
		b.Count = min(b.Count, count)
		if to != nil {
			to.Residents = appendResidentBand(to.Residents, b)
		} else if len(b.Items) > 0 && b.Count > 0 {
			if c.ItemPool == nil {
				c.ItemPool = map[string]int64{}
			}
			for itemID, qty := range b.Items {
				if qty > 0 {
					c.ItemPool[itemID] += int64(qty) * int64(b.Count)
				}
			}
		}
		hunger += int64(b.Hunger) * int64(b.Count)
		c.Residents[0].First += b.Count
		c.Residents[0].Count -= b.Count
		if c.Residents[0].Count == 0 {
			c.Residents = c.Residents[1:]
		}
		count -= b.Count
	}
	if len(c.Residents) == 0 {
		c.Residents = nil
	}
	return hunger
}
