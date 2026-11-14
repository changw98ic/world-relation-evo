package sim

import (
	"context"
	"fmt"
	"sync"
)

// Observation is a directly experienced outcome; Statement is what a speaker asserts.
type Observation struct {
	Kind     string `json:"kind"`
	Location string `json:"location"`
	Quantity int    `json:"quantity"`
}
type Statement struct {
	Topic    string `json:"topic"`
	Location string `json:"location"`
	Quantity int    `json:"quantity"`
}
type Memory struct {
	BasisEventID int64        `json:"basis_event_id,omitempty"`
	RootEventID  int64        `json:"root_event_id"`
	TargetID     string       `json:"target_id,omitempty"`
	Statement    *Statement   `json:"statement,omitempty"`
	Observation  *Observation `json:"observation,omitempty"`
	Kind         string       `json:"kind"`
	Tick         int64        `json:"tick"`
	EventID      int64        `json:"event_id"`
	Source       string       `json:"source"`
	Text         string       `json:"text"`
}
type Offer struct {
	ID      int64  `json:"id"`
	Seller  string `json:"seller"`
	Buyer   string `json:"buyer"`
	Food    int    `json:"food"`
	Price   int    `json:"price"`
	Expires int64  `json:"expires"`
}
type Actor struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Role        string         `json:"role"`
	Location    string         `json:"location"`
	Food        int            `json:"food"`
	Money       int            `json:"money"`
	Hunger      int            `json:"hunger"`
	Goal        string         `json:"goal"`
	Personality string         `json:"personality"`
	Trust       map[string]int `json:"trust"`
	Memories    []Memory       `json:"memories"`
}
type World struct {
	ID     string  `json:"id"`
	Tick   int64   `json:"tick"`
	NextID int64   `json:"next_id"`
	Actors []Actor `json:"actors"`
	Offers []Offer `json:"offers"`
}
type Action struct {
	BasisEventID int64      `json:"basis_event_id,omitempty"`
	Statement    *Statement `json:"statement,omitempty"`
	Kind         string     `json:"kind"`
	TargetID     string     `json:"target_id,omitempty"`
	Location     string     `json:"location,omitempty"`
	Text         string     `json:"text,omitempty"`
	Food         int        `json:"food,omitempty"`
	Price        int        `json:"price,omitempty"`
	OfferID      int64      `json:"offer_id,omitempty"`
}
type Event struct {
	ID       int64          `json:"id"`
	Tick     int64          `json:"tick"`
	ActorID  string         `json:"actor_id"`
	TargetID string         `json:"target_id,omitempty"`
	Kind     string         `json:"kind"`
	Action   Action         `json:"action"`
	Payload  map[string]any `json:"payload"`
}
type Neighbor struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Role string `json:"role"`
}
type Perception struct {
	Tick      int64      `json:"tick"`
	Self      Actor      `json:"self"`
	Nearby    []Neighbor `json:"nearby"`
	Offers    []Offer    `json:"offers"`
	Locations []string   `json:"locations"`
}
type Decider interface {
	Decide(context.Context, Perception) (Action, error)
}

var Locations = []string{"homes", "fields", "market", "well"}

func Village(id string) *World {
	w := &World{ID: id, NextID: 1}
	names := []string{"林禾", "陈谷", "王麦", "李田", "赵耕", "周穗", "吴苗", "郑稻", "孙粟", "钱垄", "冯耘", "褚桑", "卫豆", "蒋黍", "沈稷", "韩仓", "杨米", "朱秤", "秦布", "尤盐", "许柴", "何薪", "吕泉", "施陶", "张木", "孔石", "曹鱼", "严药", "华织", "金铁"}
	for i, n := range names {
		role := "artisan"
		goal := "Earn money and secure food through trade"
		if i < 15 {
			role = "farmer"
			goal = "Grow food and sell surplus"
		}
		w.Actors = append(w.Actors, Actor{ID: fmt.Sprintf("p%02d", i+1), Name: n, Role: role, Location: "homes", Food: 4 + i%3, Money: 12 + i%5, Hunger: 0, Goal: goal, Personality: []string{"careful and candid", "sociable and generous", "ambitious and skeptical"}[i%3], Trust: map[string]int{}})
	}
	return w
}
func (w *World) actor(id string) *Actor {
	for i := range w.Actors {
		if w.Actors[i].ID == id {
			return &w.Actors[i]
		}
	}
	return nil
}
func (w *World) View(id string) Perception {
	a := *w.actor(id)
	a.Trust = cloneTrust(a.Trust)
	start := len(a.Memories) - 32
	if start < 0 {
		start = 0
	}
	a.Memories = append([]Memory(nil), a.Memories[start:]...)
	for i := range a.Memories {
		m := &a.Memories[i]
		if m.Statement != nil {
			v := *m.Statement
			m.Statement = &v
		}
		if m.Observation != nil {
			v := *m.Observation
			m.Observation = &v
		}
	}
	p := Perception{Tick: w.Tick, Self: a, Locations: append([]string(nil), Locations...)}
	for _, b := range w.Actors {
		if b.ID != id && b.Location == a.Location {
			p.Nearby = append(p.Nearby, Neighbor{b.ID, b.Name, b.Role})
		}
	}
	for _, o := range w.Offers {
		if (o.Buyer == id || o.Seller == id) && o.Expires >= w.Tick {
			p.Offers = append(p.Offers, o)
		}
	}
	return p
}
func cloneTrust(m map[string]int) map[string]int {
	r := map[string]int{}
	for k, v := range m {
		r[k] = v
	}
	return r
}

// Decisions observe the same starting state; settlement follows stable actor order.
// A failed decision leaves the entire step unapplied.
func (w *World) Step(ctx context.Context, d Decider) ([]Event, error) {
	actions := make([]Action, len(w.Actors))
	errs := make([]error, len(w.Actors))
	jobs := make(chan int)
	var workers sync.WaitGroup
	for worker := 0; worker < 4; worker++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for i := range jobs {
				actions[i], errs[i] = d.Decide(ctx, w.View(w.Actors[i].ID))
			}
		}()
	}
	for i := range w.Actors {
		jobs <- i
	}
	close(jobs)
	workers.Wait()
	for i, err := range errs {
		if err != nil {
			return nil, fmt.Errorf("actor %s: %w", w.Actors[i].ID, err)
		}
	}
	var events []Event
	for i, a := range actions {
		events = append(events, w.Apply(w.Actors[i].ID, a)...)
	}
	w.Tick++
	if w.Tick%6 == 0 {
		for i := range w.Actors {
			w.Actors[i].Hunger++
		}
	}
	var offers []Offer
	for _, o := range w.Offers {
		if o.Expires >= w.Tick {
			offers = append(offers, o)
		}
	}
	w.Offers = offers
	return events, nil
}
func (w *World) record(a *Actor, x Action, kind string, data map[string]any, observers ...*Actor) Event {
	e := Event{w.NextID, w.Tick, a.ID, x.TargetID, kind, x, data}
	w.NextID++
	root := e.ID
	if x.BasisEventID != 0 {
		for _, m := range a.Memories {
			if m.EventID == x.BasisEventID {
				root = m.RootEventID
				break
			}
		}
	}
	data["basis_event_id"] = x.BasisEventID
	data["root_event_id"] = root
	m := Memory{Kind: kind, Tick: w.Tick, EventID: e.ID, Source: a.ID, TargetID: x.TargetID, Text: fmt.Sprint(data["text"]), BasisEventID: x.BasisEventID, RootEventID: root}
	if kind == "speech" && x.Statement != nil {
		v := *x.Statement
		m.Statement = &v
		data["statement"] = v
	}
	if kind == "harvested" || kind == "gathered" {
		quantity := 1
		if kind == "harvested" {
			quantity = 2
		}
		m.Observation = &Observation{Kind: "food_produced", Location: a.Location, Quantity: quantity}
		data["observation"] = *m.Observation
	}
	a.Memories = append(a.Memories, m)
	for _, b := range observers {
		if b != nil && b.ID != a.ID {
			b.Memories = append(b.Memories, m)
		}
	}
	return e
}
func (w *World) Apply(id string, x Action) []Event {
	a := w.actor(id)
	if a == nil {
		return nil
	}
	b := w.actor(x.TargetID)
	emit := func(kind, text string, observers ...*Actor) []Event {
		return []Event{w.record(a, x, kind, map[string]any{"text": text}, observers...)}
	}
	fail := func(reason string) []Event { return emit("action_failed", reason) }
	if x.BasisEventID != 0 {
		known := false
		for _, m := range a.Memories {
			if m.EventID == x.BasisEventID {
				known = true
				break
			}
		}
		if !known {
			x.BasisEventID = 0
			return fail("basis event was not experienced or heard by this actor")
		}
	}
	switch x.Kind {
	case "wait":
		return nil
	case "move":
		for _, l := range Locations {
			if x.Location == l {
				a.Location = l
				return emit("moved", a.Name+" moved to "+l)
			}
		}
		return fail("unknown location")
	case "work":
		if a.Role == "farmer" {
			if a.Location != "fields" {
				return fail("farming requires fields")
			}
			a.Food += 2
			return emit("harvested", a.Name+" harvested 2 food")
		}
		if a.Location != "fields" {
			return fail("gathering requires fields")
		}
		a.Food++
		return emit("gathered", a.Name+" gathered 1 food")
	case "eat":
		if a.Food < 1 {
			return fail("no food")
		}
		if a.Hunger == 0 {
			return nil
		}
		a.Food--
		a.Hunger--
		return emit("ate", a.Name+" ate 1 food")
	case "speak":
		if b == nil || b.ID == id || b.Location != a.Location {
			return fail("listener is not nearby")
		}
		if x.Text == "" {
			return fail("empty speech")
		}
		return emit("speech", x.Text, b)
	case "offer":
		if b == nil || b.ID == id || b.Location != a.Location {
			return fail("buyer is not nearby")
		}
		if x.Food <= 0 || x.Price < 0 || a.Food < x.Food {
			return fail("invalid offer or insufficient food")
		}
		e := w.record(a, x, "trade_offered", map[string]any{"text": fmt.Sprintf("%s offers %d food for %d coins", a.Name, x.Food, x.Price)}, b)
		w.Offers = append(w.Offers, Offer{e.ID, id, b.ID, x.Food, x.Price, w.Tick + 6})
		return []Event{e}
	case "accept", "reject":
		for i, o := range w.Offers {
			if o.ID != x.OfferID || o.Buyer != id {
				continue
			}
			seller := w.actor(o.Seller)
			x.TargetID = o.Seller
			if x.Kind == "reject" {
				w.Offers = append(w.Offers[:i], w.Offers[i+1:]...)
				return emit("trade_rejected", a.Name+" rejected offer", seller)
			}
			if o.Expires < w.Tick || seller == nil || seller.Location != a.Location || seller.Food < o.Food || a.Money < o.Price {
				return fail("offer cannot settle")
			}
			seller.Food -= o.Food
			a.Food += o.Food
			a.Money -= o.Price
			seller.Money += o.Price
			a.Trust[seller.ID]++
			seller.Trust[a.ID]++
			w.Offers = append(w.Offers[:i], w.Offers[i+1:]...)
			return emit("trade_completed", fmt.Sprintf("%s bought %d food from %s for %d coins", a.Name, o.Food, seller.Name, o.Price), seller)
		}
		return fail("offer not found")
	default:
		return fail("unknown action: " + x.Kind)
	}
}
