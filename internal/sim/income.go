package sim

import (
	"fmt"
	"math"
	"sort"
)

type PayCycle string

const (
	PayDaily   PayCycle = "daily"
	PayWeekly  PayCycle = "weekly"
	PayMonthly PayCycle = "monthly"
)

func (p PayCycle) days() int64 {
	switch p {
	case PayDaily:
		return 1
	case PayWeekly:
		return 7
	case PayMonthly:
		// The simulation calendar has seasons. A monthly contract settles
		// after each accounting period of thirty simulation days.
		return 30
	}
	return 0
}

const paydayOffset = 14 * TicksPerHour // Each pay period closes at 20:00.

func (p PayCycle) nextPayday(tick int64) int64 {
	days := p.days()
	end := ((tick/TicksPerDay/days+1)*days-1)*TicksPerDay + paydayOffset
	if end <= tick {
		end += days * TicksPerDay
	}
	return end
}

// WorkOrder is an employer's standing offer for a specified service. An
// accepted action carries its ID, and only the completion event earns pay.
type WorkOrder struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	EmployerID      string   `json:"employer_id"`
	ActionKind      string   `json:"action_kind"`
	CompletionKind  string   `json:"completion_kind"`
	RequiredFeature string   `json:"required_feature,omitempty"`
	District        string   `json:"district,omitempty"`
	RecipeID        string   `json:"recipe_id,omitempty"`
	SkillID         string   `json:"skill_id,omitempty"`
	Wage            int      `json:"wage"`
	PayCycle        PayCycle `json:"pay_cycle"`
}

func (c Catalog) WorkOrder(id string) (WorkOrder, bool) {
	for _, order := range c.WorkOrders {
		if order.ID == id {
			return order, true
		}
	}
	return WorkOrder{}, false
}

func (order WorkOrder) applies(a Actor, x Action, m WorldMap) bool {
	return x.Kind == order.ActionKind && (order.RecipeID == "" || order.RecipeID == x.RecipeID) &&
		(order.SkillID == "" || order.SkillID == x.SkillID) &&
		(order.District == "" || order.District == a.Location) &&
		(order.RequiredFeature == "" || m.VenueOrDistrictHasFeature(a.VenueID, a.Location, order.RequiredFeature))
}

func (order WorkOrder) PayAt(completion int64) int64 {
	return order.PayCycle.nextPayday(completion)
}

func (p Perception) WorkOrdersFor(x Action) []WorkOrder {
	var orders []WorkOrder
	for _, order := range p.Catalog.WorkOrders {
		if order.applies(p.Self, x, p.Map) {
			orders = append(orders, order)
		}
	}
	return orders
}

type EarnedWage struct {
	OrderID     string   `json:"order_id"`
	WorkerID    string   `json:"worker_id"`
	EmployerID  string   `json:"employer_id"`
	WorkEventID int64    `json:"work_event_id"`
	DueTick     int64    `json:"due_tick"`
	PayCycle    PayCycle `json:"pay_cycle"`
	Coins       int64    `json:"coins"`
}

// DividendAccount distributes realized liquid surplus to registered owners.
// Capital and obligations are excluded from distributable cash.
type DividendAccount struct {
	BusinessID     string         `json:"business_id"`
	PayCycle       PayCycle       `json:"pay_cycle"`
	Ownership      map[string]int `json:"ownership_permille"`
	Capital        int64          `json:"capital"`
	OpeningNetCash int64          `json:"opening_net_cash"`
	LastClosedTick int64          `json:"last_closed_tick,omitempty"`
}

func (w *World) earnWage(ev Event) []Event {
	if ev.Action.WorkOrderID == "" {
		return nil
	}
	order, ok := w.Catalog.WorkOrder(ev.Action.WorkOrderID)
	if !ok || order.Wage == 0 || ev.Kind != order.CompletionKind || ev.Kind == "action_failed" {
		return nil
	}
	a := w.actor(ev.ActorID)
	if a == nil || !order.applies(*a, ev.Action, w.Map) {
		return nil
	}
	claim := EarnedWage{OrderID: order.ID, EmployerID: order.EmployerID, WorkerID: a.ID,
		WorkEventID: ev.ID, DueTick: order.PayAt(ev.Tick), PayCycle: order.PayCycle, Coins: int64(order.Wage)}
	w.EarnedWages = append(w.EarnedWages, claim)
	return []Event{w.record(a, ev.Action, "wage_earned", map[string]any{
		"text":           fmt.Sprintf("%s earned %d coins for %s", a.Name, claim.Coins, order.Name),
		"cause_event_id": ev.ID, "work_order_id": order.ID, "employer": claim.EmployerID,
		"coins": claim.Coins, "due_tick": claim.DueTick, "pay_cycle": claim.PayCycle,
	})}
}

func (w *World) payIncome(payer, recipient string, coins int64, category string) int64 {
	var funds int64
	var source *PopulationCohort
	if payer == "treasury" {
		funds = w.PublicTreasury
	} else if source = w.findCohort(payer); source != nil {
		funds = sharedCohortCash(source)
	}
	a, target := w.actor(recipient), w.findCohort(recipient)
	if payer == recipient || a == nil && target == nil {
		return 0
	}
	paid := min(funds, coins)
	var residentCohort *PopulationCohort
	if a != nil {
		paid = min(paid, int64(math.MaxInt-a.Money))
		residentCohort = w.storedActorCohort(a.ID)
		if residentCohort != nil && residentCohort != source {
			paid = min(paid, math.MaxInt64-residentCohort.MoneyPool)
		}
	} else {
		paid = min(paid, math.MaxInt64-target.MoneyPool)
	}
	if paid <= 0 {
		return 0
	}
	if source != nil {
		w.addCohortCurrency(source, "coins", -paid)
	} else {
		w.debitTreasury(paid, category)
	}
	if a != nil {
		w.adjustActorCurrency(a, "coins", paid, false)
		if residentCohort != nil {
			w.addCohortCurrency(residentCohort, "coins", paid)
		}
		w.setWake(a, w.Tick, "income_paid")
	} else {
		w.addCohortCurrency(target, "coins", paid)
	}
	return paid
}

func sharedCohortCash(c *PopulationCohort) int64 {
	cash := c.MoneyPool
	for _, a := range c.Members {
		cash -= int64(a.Money)
	}
	return cash
}

func (w *World) incomeEvent(actor, payer, kind string, payload map[string]any) Event {
	x := Action{Kind: kind, TargetID: payer}
	if a := w.actor(actor); a != nil {
		return w.record(a, x, kind, payload)
	}
	ev := Event{ID: w.NextID, Tick: w.Tick, ActorID: actor, TargetID: payer, Kind: kind, Action: x, Payload: payload}
	w.NextID++
	return ev
}

func (w *World) settleWages() []Event {
	var events []Event
	pending := w.EarnedWages[:0]
	for _, claim := range w.EarnedWages {
		if claim.DueTick <= w.Tick {
			paid := w.payIncome(claim.EmployerID, claim.WorkerID, claim.Coins, "payroll")
			if paid > 0 {
				events = append(events, w.incomeEvent(claim.WorkerID, claim.EmployerID, "wage_paid", map[string]any{
					"text":  fmt.Sprintf("Paid %d coins for completed work", paid),
					"coins": paid, "payer_id": claim.EmployerID, "payee_id": claim.WorkerID,
					"pay_cycle": claim.PayCycle, "work_order_id": claim.OrderID,
					"cause_event_id": claim.WorkEventID,
				}))
				claim.Coins -= paid
			}
			if claim.Coins > 0 {
				events = append(events, w.incomeEvent(claim.WorkerID, claim.EmployerID, "wage_unpaid", map[string]any{
					"text":      fmt.Sprintf("%d coins in wages remain payable", claim.Coins),
					"coins_due": claim.Coins, "work_order_id": claim.OrderID, "cause_event_id": claim.WorkEventID,
				}))
				claim.DueTick = claim.PayCycle.nextPayday(w.Tick)
			}
		}
		if claim.Coins > 0 {
			pending = append(pending, claim)
		}
	}
	w.EarnedWages = pending
	return events
}

func (w *World) businessNetCash(c *PopulationCohort) int64 {
	cash := sharedCohortCash(c)
	if w.MarketCredit != nil {
		for _, debt := range w.MarketCredit.CohortDebts {
			if debt.DebtorID == c.ID && debt.Status == "active" {
				cash = max(0, cash-max(0, debt.Principal-debt.Repaid))
				cash = max(0, cash-max(0, debt.AccruedInterest-max(0, debt.Repaid-debt.Principal)))
			}
		}
	}
	for _, claim := range w.EarnedWages {
		if claim.EmployerID == c.ID {
			cash = max(0, cash-claim.Coins)
		}
	}
	return cash
}

// ValidateIncome checks authored work terms and persisted payment accounts at
// entry. Existing claims keep their own payer, amount and settlement period.
func (w *World) ValidateIncome() error {
	employerExists := func(id string) bool {
		return id == "treasury" || w.findCohort(id) != nil
	}
	ids := make(map[string]bool, len(w.Catalog.WorkOrders))
	for _, order := range w.Catalog.WorkOrders {
		_, actionKnown := w.Catalog.ActionMetadataByKind(order.ActionKind)
		if order.ID == "" || ids[order.ID] || order.Name == "" || !actionKnown ||
			order.Wage < 0 || order.PayCycle.days() == 0 || !employerExists(order.EmployerID) {
			return fmt.Errorf("invalid work order %q", order.ID)
		}
		switch order.CompletionKind {
		case "", "action_failed", "task_started", "dormant_started":
			return fmt.Errorf("work order %q requires a completion event", order.ID)
		}
		if order.RecipeID != "" {
			if _, ok := w.Catalog.RecipeByID(order.RecipeID); !ok {
				return fmt.Errorf("work order %q refers to unknown recipe %q", order.ID, order.RecipeID)
			}
		}
		ids[order.ID] = true
	}
	seen := make(map[int64]bool, len(w.EarnedWages))
	for _, wage := range w.EarnedWages {
		if wage.Coins <= 0 || wage.DueTick < 0 || wage.WorkEventID <= 0 ||
			seen[wage.WorkEventID] || wage.PayCycle.days() == 0 {
			return fmt.Errorf("invalid wage for work event %d", wage.WorkEventID)
		}
		seen[wage.WorkEventID] = true
	}
	businesses := make(map[string]bool, len(w.DividendAccounts))
	for _, account := range w.DividendAccounts {
		if businesses[account.BusinessID] || w.findCohort(account.BusinessID) == nil ||
			account.PayCycle.days() == 0 || account.Capital < 0 || account.OpeningNetCash < 0 {
			return fmt.Errorf("invalid dividend account for %q", account.BusinessID)
		}
		businesses[account.BusinessID] = true
		var shares int
		for owner, amount := range account.Ownership {
			if owner == account.BusinessID || amount < 0 || amount > 1000-shares ||
				w.actor(owner) == nil && w.findCohort(owner) == nil {
				return fmt.Errorf("invalid dividend ownership for %q", account.BusinessID)
			}
			shares += amount
		}
	}
	return nil
}

func (w *World) RegisterDividends(accounts []DividendAccount) error {
	candidate := *w
	candidate.DividendAccounts = append([]DividendAccount(nil), accounts...)
	if err := candidate.ValidateIncome(); err != nil {
		return err
	}
	w.DividendAccounts = candidate.DividendAccounts
	for i := range w.DividendAccounts {
		account := &w.DividendAccounts[i]
		account.Ownership = cloneIntMap(account.Ownership)
		account.OpeningNetCash = w.businessNetCash(w.findCohort(account.BusinessID))
		account.LastClosedTick = w.Tick
	}
	return nil
}

func (w *World) settleDividends() []Event {
	var events []Event
	for i := range w.DividendAccounts {
		account := &w.DividendAccounts[i]
		if account.LastClosedTick == w.Tick || (w.Tick/TicksPerDay+1)%account.PayCycle.days() != 0 {
			continue
		}
		business := w.findCohort(account.BusinessID)
		if business == nil {
			continue
		}
		netCash := w.businessNetCash(business)
		surplus := max(0, netCash-max(account.Capital, account.OpeningNetCash))
		var owners []string
		for owner := range account.Ownership {
			owners = append(owners, owner)
		}
		sort.Strings(owners)
		var share, allocated int64
		for _, owner := range owners {
			share += int64(account.Ownership[owner])
			through := surplus/1000*share + surplus%1000*share/1000
			paid := w.payIncome(business.ID, owner, through-allocated, "dividend")
			allocated = through
			if paid > 0 {
				events = append(events, w.incomeEvent(owner, business.ID, "dividend_paid", map[string]any{
					"text":  fmt.Sprintf("%s distributed %d coins of its surplus to %s", business.Name, paid, owner),
					"coins": paid, "ownership_permille": account.Ownership[owner], "distributable_coins": surplus,
				}))
			}
		}
		account.OpeningNetCash = w.businessNetCash(business)
		account.LastClosedTick = w.Tick
	}
	return events
}
