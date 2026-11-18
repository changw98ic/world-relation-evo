package sim

import (
	"fmt"
	"slices"
	"sort"
	"strings"
)

// ProductionSpec contains scenario budgets, not inferred physical constants.
// OutputQty is the maximum conversion of the recipe's declared inputs and
// resource draw. Biological recipes also obtain biomass from their declared
// growth process; only those recipes may use a consumable yield bonus.
type ProductionSpec struct {
	FacilitySource       string           `json:"facility_source"`
	FacilityCapacity     map[string]int64 `json:"facility_capacity"`
	FieldAreaPerBatchM2  int64            `json:"field_area_per_batch_m2"`
	FieldAreaPerFarmerM2 int64            `json:"field_area_per_farmer_m2"`
	FieldSource          string           `json:"field_source"`
}

func DefaultProductionSpec() ProductionSpec {
	return ProductionSpec{FacilitySource: "scenario equipment budget: each installed facility has one serial workstation; capacity is configured, not inferred from population", FacilityCapacity: map[string]int64{"forge": 1, "carpentry_bench": 1, "herb_rack": 1, "compost_pit": 1}, FieldAreaPerBatchM2: 1000, FieldAreaPerFarmerM2: 1000, FieldSource: "scenario budget: one 1000 m2 field per farmer; one simultaneous crop per field; not agronomic calibration"}
}

type FieldAsset struct {
	ID        string `json:"id"`
	District  string `json:"district"`
	VenueID   string `json:"venue_id,omitempty"`
	AreaM2    int64  `json:"area_m2"`
	OwnerKind string `json:"owner_kind"`
	OwnerID   string `json:"owner_id"`
	UserKind  string `json:"user_kind"`
	UserID    string `json:"user_id"`
}

type FieldView struct {
	FieldAsset
	AvailableAreaM2 int64 `json:"available_area_m2"`
}

func (w *World) availableFieldArea(field FieldAsset) int64 {
	occupied := int64(0)
	for _, growth := range w.Growth {
		if growth.FieldID == field.ID && !growth.Harvested {
			occupied += growth.AreaM2
		}
	}
	for _, actor := range w.Actors {
		if actor.Task != nil && actor.Task.FieldID == field.ID {
			occupied += w.Catalog.Production.FieldAreaPerBatchM2
		}
	}
	return max(int64(0), field.AreaM2-occupied)
}

// A care agreement is accepted by both parties. It declares whose local stocks
// pay for maintenance, whether harvest is authorized, and a share paid in kind.
// Same guild membership never creates an agreement.
type GrowthCareAgreement struct {
	GrowthID          int64  `json:"growth_id"`
	OwnerKind         string `json:"owner_kind"`
	OwnerID           string `json:"owner_id"`
	CaretakerKind     string `json:"caretaker_kind"`
	CaretakerID       string `json:"caretaker_id"`
	SupplierKind      string `json:"supplier_kind"`
	SupplierID        string `json:"supplier_id"`
	Harvest           bool   `json:"harvest"`
	SharePermille     int64  `json:"share_permille"`
	OwnerAccepted     bool   `json:"owner_accepted"`
	CaretakerAccepted bool   `json:"caretaker_accepted"`
}

func (a GrowthCareAgreement) validateTerms() error {
	if a.OwnerID == "" || (a.OwnerKind != "actor" && a.OwnerKind != "cohort") || a.CaretakerID == "" || (a.CaretakerKind != "actor" && a.CaretakerKind != "cohort") {
		return fmt.Errorf("care requires a named owner and caretaker for growth %d", a.GrowthID)
	}
	if a.SupplierKind != a.CaretakerKind || a.SupplierID != a.CaretakerID {
		return fmt.Errorf("supplies must come from the consenting local caretaker")
	}
	if a.SharePermille < 0 || a.SharePermille > 1000 {
		return fmt.Errorf("care harvest share must be in 0..1000")
	}
	return nil
}

func (a GrowthCareAgreement) validateSigned() error {
	if !a.OwnerAccepted || !a.CaretakerAccepted {
		return fmt.Errorf("care requires both parties' assent for growth %d", a.GrowthID)
	}
	return a.validateTerms()
}

// CaretakerShare returns whole units due on the actual harvest. Split before
// multiplying so even a full int64 inventory can be divided without overflow.
func (a GrowthCareAgreement) CaretakerShare(quantity int64) int64 {
	return (quantity/1000)*a.SharePermille + (quantity%1000)*a.SharePermille/1000
}

type ProductionState struct {
	WorkerRoster    []WorkerRun                 `json:"worker_roster,omitempty"`
	Fields          []FieldAsset                `json:"fields,omitempty"`
	CareAgreements  []GrowthCareAgreement       `json:"care_agreements,omitempty"`
	CareProposals   []GrowthCareAgreement       `json:"care_proposals,omitempty"`
	ToolWear        map[string]map[string]int64 `json:"tool_wear,omitempty"`
	EnergyRemainder map[string]int64            `json:"energy_remainder,omitempty"`
	YieldCredit     map[string]int64            `json:"yield_credit,omitempty"`
	LivingCare      map[string]LivingCareState  `json:"living_care,omitempty"`
	LaborTick       int64                       `json:"labor_tick"`
	LaborUsed       map[string]int64            `json:"labor_used,omitempty"`
}

func (w *World) growthEvent(g GrowthBatch, kind string, payload map[string]any) Event {
	payload["growth_id"] = g.ID
	e := Event{ID: w.NextID, Tick: w.Tick, ActorID: g.OwnerID, Kind: kind, Action: Action{Kind: "wait", RecipeID: g.RecipeID}, Payload: payload}
	w.NextID++
	return e
}

func (w *World) productionFreeWorkers(care GrowthBatch) int64 {
	if !w.growthOwnerPresent(care) {
		return 0
	}
	key := holderKey(holderKind(care.Cohort), care.OwnerID)
	var available int64
	if care.Cohort {
		c := w.findCohort(care.OwnerID)
		available = int64(w.anonymousCohort(*c).EffectivePopulation() - w.ecologyReservedWorkers(c.ID))
		for _, g := range w.Growth {
			cg := w.growthCaretaker(g)
			r, _ := w.Catalog.RecipeByID(g.RecipeID)
			if cg.Cohort && cg.OwnerID == c.ID && !g.Ready && g.LaborBookedTick != w.Tick+1 && (r.Growth == nil || g.PreparationRemaining > 0 || w.Tick < g.PreparationUntil) {
				available -= int64(g.Workers)
			}
		}
	} else {
		a := w.actor(care.OwnerID)
		if a.Task != nil {
			return 0
		}
		available = 1
	}
	if w.Production.LaborTick == w.Tick {
		available -= w.Production.LaborUsed[key]
	}
	return max(int64(0), available)
}

func (w *World) reserveProductionWorkers(care GrowthBatch, workers int64) {
	if w.Production.LaborTick != w.Tick || w.Production.LaborUsed == nil {
		w.Production.WorkerRoster = nil
		w.Production.LaborTick = w.Tick
		w.Production.LaborUsed = map[string]int64{}
	}
	w.Production.LaborUsed[holderKey(holderKind(care.Cohort), care.OwnerID)] += workers
}

func (w *World) canHarvestGrowth(g GrowthBatch, id string, cohort bool) bool {
	if !g.Ready {
		return false
	}
	caller := GrowthBatch{OwnerID: id, Cohort: cohort, District: g.District, VenueID: g.VenueID}
	if !w.growthOwnerPresent(caller) || w.productionFreeWorkers(caller) < 1 {
		return false
	}
	if id == g.OwnerID && cohort == g.Cohort {
		return true
	}
	agreement := w.careAgreement(g)
	return !g.Harvested && agreement != nil && agreement.Harvest && agreement.CaretakerID == id && agreement.CaretakerKind == holderKind(cohort)
}

func holderKind(cohort bool) string {
	if cohort {
		return "cohort"
	}
	return "actor"
}
func holderKey(kind, id string) string { return kind + ":" + id }

// nonBiologicalYieldPct keeps a fixed material conversion inside its declared
// budget. EcologicalYieldMultiplierPct may reach 180 for biological growth;
// that extra does not create matter from a non-biological recipe.
func nonBiologicalYieldPct(pct int) int {
	if pct > 100 {
		return 100
	}
	return pct
}

// fundedYieldHundredths is the fraction of declared output paid for by this
// many batches. One hundred hundredths equal one output unit.
func fundedYieldHundredths(base, pct int, batches int64) int64 {
	return int64(base) * int64(nonBiologicalYieldPct(pct)) * batches
}

// settleYieldCredit turns previously retained hundredths plus this batch's
// funded hundredths into whole units and the hundredths still owed.
func settleYieldCredit(prior, funded int64) (delivered, next int64) {
	total := prior + funded
	return total / 100, total % 100
}

func yieldCreditKey(kind, id, recipeID string) string {
	return holderKey(kind, id) + "/" + recipeID
}

func (w *World) yieldCredit(kind, id, recipeID string) int64 {
	if w.Production.YieldCredit == nil {
		return 0
	}
	return w.Production.YieldCredit[yieldCreditKey(kind, id, recipeID)]
}

func (w *World) setYieldCredit(kind, id, recipeID string, hundredths int64) {
	key := yieldCreditKey(kind, id, recipeID)
	if hundredths == 0 {
		if w.Production.YieldCredit == nil {
			return
		}
		delete(w.Production.YieldCredit, key)
		if len(w.Production.YieldCredit) == 0 {
			w.Production.YieldCredit = nil
		}
		return
	}
	if w.Production.YieldCredit == nil {
		w.Production.YieldCredit = map[string]int64{}
	}
	w.Production.YieldCredit[key] = hundredths
}

func (w *World) previewNonBiologicalYield(kind, id, district string, r Recipe) (delivered, funded int64) {
	funded = fundedYieldHundredths(r.OutputQty, w.EcologicalYieldMultiplierPct(district, r), 1)
	delivered, _ = settleYieldCredit(w.yieldCredit(kind, id, r.ID), funded)
	return delivered, funded
}
func sortedProductionKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// InitProductionAssets is scenario initialization only. It never runs at craft
// time, so growing population or repeated sowing cannot manufacture new land.
func (w *World) InitProductionAssets() {
	add := func(kind, id, district string, n int64) {
		if n == 0 || w.Catalog.Production.FieldAreaPerFarmerM2 == 0 {
			return
		}
		if !w.Map.DistrictHasFeature(district, "farmland") {
			return
		}
		key := holderKey(kind, id)
		for _, f := range w.Production.Fields {
			if f.ID == key {
				return
			}
		}
		w.Production.Fields = append(w.Production.Fields, FieldAsset{ID: key, District: district, AreaM2: n * w.Catalog.Production.FieldAreaPerFarmerM2, OwnerKind: kind, OwnerID: id, UserKind: kind, UserID: id})
	}
	for _, a := range w.Actors {
		if a.Professions["farmer"] == 0 && a.Professions["agronomist"] == 0 {
			continue
		}
		district := a.Location
		if !w.Map.DistrictHasFeature(district, "farmland") {
			for _, d := range w.Map.Districts {
				if w.Map.DistrictHasFeature(d.ID, "farmland") {
					district = d.ID
					break
				}
			}
		}
		add("actor", a.ID, district, 1)
	}
	for _, c := range w.Cohorts {
		if c.Profession == "farmer" || c.Profession == "agronomist" {
			add("cohort", c.ID, c.District, int64(c.Population))
		}
	}
}

func (w *World) fieldAvailable(kind, id, district, venue string, r Recipe) (string, int64) {
	if r.Growth == nil || r.NodeFeature != "farmland" {
		return "", int64(^uint64(0) >> 1)
	}
	per := w.Catalog.Production.FieldAreaPerBatchM2
	if per <= 0 {
		return "", 0
	}
	bestID := ""
	var best int64
	for _, f := range w.Production.Fields {
		if f.UserKind != kind || f.UserID != id || f.District != district || (f.VenueID != "" && f.VenueID != venue) {
			continue
		}
		free := w.availableFieldArea(f) / per
		if free > best {
			bestID, best = f.ID, free
		}
	}
	return bestID, best
}

func (w *World) ProductionStartBlockReasons(a Actor) map[string]string {
	var reasons map[string]string
	for _, r := range w.Catalog.Recipes {
		if reason := w.productionStartBlockReason(a, r); reason != "" {
			if reasons == nil {
				reasons = map[string]string{}
			}
			reasons[r.ID] = reason
		}
	}
	return reasons
}

// RecipeBlockReason evaluates a start using the same stock, site and recipe
// checks as settlement. The perception contains the site's assessment at the
// start of the tick; Apply reassesses it after earlier actions have settled.
func (p Perception) RecipeBlockReason(r Recipe) string {
	if blocked, reason := IsRecipeBlockedByWorldLaw(p.WorldLaws, r); blocked {
		return reason
	}
	var stock int64
	if p.DistrictState != nil {
		stock = p.DistrictState.AvailableResource(r.ResourcePool)
	}
	return recipeBlockReason(p.Catalog, p.Map, p.CurrentHouse, p.Self, r, p.ProductionBlockReasons[r.ID], stock)
}

func recipeBlockReason(cat Catalog, worldMap WorldMap, house *House, a Actor, r Recipe, siteReason string, stock int64) string {
	if ok, reason := CheckRecipe(cat, worldMap, house, a, r); !ok {
		return reason
	}
	if siteReason != "" {
		return siteReason
	}
	if r.ResourcePool != "" && stock < int64(max(1, r.ResourceDraw)) {
		return "insufficient ecological resource: " + r.ResourcePool
	}
	return ""
}

func (w *World) productionStartBlockReason(a Actor, r Recipe) string {
	if w.EcologicalYieldMultiplierPct(a.Location, r) == 0 {
		return "current environment prevents production"
	}
	if !w.productionSiteOperational(a.Location, a.VenueID, a.InsideHouse, r) {
		return "required production facility is unavailable at the current site"
	}
	if _, free := w.productionFacility(a.Location, a.VenueID, a.InsideHouse, r); free < 1 {
		return "production facility has no free workstation"
	}
	if _, free := w.fieldAvailable("actor", a.ID, a.Location, a.VenueID, r); free < 1 {
		return "no free authorized field area"
	}
	if w.productionFreeWorkers(GrowthBatch{OwnerID: a.ID, District: a.Location, VenueID: a.VenueID}) < 1 {
		return "labor is already committed to production care"
	}
	return ""
}

func (w *World) productionSiteOperational(district, venue, houseID string, r Recipe) bool {
	if w.isVenuePowerBlocked(venue) {
		return false
	}
	if r.HouseFacility == "" {
		return true
	}
	if houseID != "" {
		h := w.house(houseID)
		return h != nil && h.District == district && h.Condition > 0 && h.HasFacility(r.HouseFacility) && !w.isVenuePowerBlocked(h.VenueID)
	}
	for _, h := range w.Houses {
		if h.District == district && (venue == "" || h.VenueID == venue) && h.Condition > 0 && h.HasFacility(r.HouseFacility) && !w.isVenuePowerBlocked(h.VenueID) {
			return true
		}
	}
	return false
}

func (w *World) facilityFree(h House, r Recipe) int64 {
	capacity := w.Catalog.Production.FacilityCapacity[r.HouseFacility]
	if !w.productionSiteOperational(h.District, h.VenueID, h.ID, r) {
		return 0
	}
	for _, g := range w.Growth {
		if g.HouseID == h.ID && !g.Harvested {
			active, _ := w.Catalog.RecipeByID(g.RecipeID)
			if active.HouseFacility == r.HouseFacility {
				capacity -= g.Batches
			}
		}
	}
	for _, a := range w.Actors {
		if a.InsideHouse == h.ID && a.Task != nil && a.Task.Action.Kind == "craft" {
			rr, ok := w.Catalog.RecipeByID(a.Task.Action.RecipeID)
			if ok && rr.HouseFacility == r.HouseFacility {
				capacity--
			}
		}
	}
	return max(int64(0), capacity)
}

func (w *World) productionFacility(district, venue, houseID string, r Recipe) (string, int64) {
	if r.HouseFacility == "" {
		return "", int64(^uint64(0) >> 1)
	}
	for _, h := range w.Houses {
		if h.District != district || (houseID != "" && h.ID != houseID) || (houseID == "" && venue != "" && h.VenueID != venue) || !h.HasFacility(r.HouseFacility) {
			continue
		}
		if free := w.facilityFree(h, r); free > 0 {
			return h.ID, free
		}
	}
	return "", 0
}

// Tool wear keeps the remainder instead of rounding each small job up. The
// denominator is the item's declared number of batches per tool, shared by
// individual and cohort work. No extra output is created by possessing a tool.
func (w *World) wearProductionTool(kind, id string, r Recipe, batches int64) {
	var tools []string
	for item, spec := range w.Catalog.Items {
		if spec.DurabilityBatches > 0 && w.growthStock(GrowthBatch{OwnerID: id, Cohort: kind == "cohort"}, item) > 0 {
			tools = append(tools, item)
		}
	}
	sort.Strings(tools)
	if len(tools) == 0 || (r.ResourcePool == "" && r.HouseFacility == "") {
		return
	}
	w.wearToolItem(kind, id, tools[0], batches)
}

// wearToolItem advances tool wear for a single item type and deducts consumed whole tools.
func (w *World) wearToolItem(kind, id, item string, batches int64) {
	if item == "" || batches <= 0 {
		return
	}
	life := w.Catalog.Items[item].DurabilityBatches
	if life <= 0 {
		return
	}
	key := holderKey(kind, id)
	if w.Production.ToolWear == nil {
		w.Production.ToolWear = map[string]map[string]int64{}
	}
	if w.Production.ToolWear[key] == nil {
		w.Production.ToolWear[key] = map[string]int64{}
	}
	stock := w.growthStock(GrowthBatch{OwnerID: id, Cohort: kind == "cohort"}, item)
	var equippedOwner *Actor
	var equippedCount int64
	if kind == "actor" {
		if act := w.actor(id); act != nil {
			for _, eq := range act.Equipped {
				if eq == item {
					equippedCount++
					equippedOwner = act
				}
			}
		}
	}
	totalAvail := stock + equippedCount
	wear := min(totalAvail*life, w.Production.ToolWear[key][item]+batches)
	spent := min(totalAvail, wear/life)
	w.Production.ToolWear[key][item] = wear - spent*life
	if spent > 0 {
		fromStock := min(stock, spent)
		if fromStock > 0 {
			w.changeGrowthStock(GrowthBatch{OwnerID: id, Cohort: kind == "cohort"}, item, -fromStock, true)
		}
		fromEquipped := spent - fromStock
		if fromEquipped > 0 && equippedOwner != nil {
			slots := make([]string, 0, len(equippedOwner.Equipped))
			for slot, eq := range equippedOwner.Equipped {
				if eq == item {
					slots = append(slots, slot)
				}
			}
			sort.Strings(slots)
			rem := fromEquipped
			for _, slot := range slots {
				if rem == 0 {
					break
				}
				if w.removeActorEquippedItem(equippedOwner, slot, true) {
					rem--
				}
			}
		}
	}
}

func (w *World) transferGrowingAssets(fromKind, fromID, toKind, toID string) {
	preservedMember := false
	if fromKind == "actor" && toKind == "cohort" {
		if target := w.findCohort(toID); target != nil {
			for _, member := range target.Members {
				if member.ID == fromID {
					preservedMember = true
					break
				}
			}
		}
	}
	// A stored member is still the same person with the same private property.
	if preservedMember {
		return
	}
	for i := range w.Growth {
		g := &w.Growth[i]
		if holderKind(g.Cohort) == fromKind && g.OwnerID == fromID {
			g.OwnerID, g.Cohort = toID, toKind == "cohort"
		}
	}
	for i := range w.Production.Fields {
		f := &w.Production.Fields[i]
		if f.OwnerKind == fromKind && f.OwnerID == fromID {
			f.OwnerKind, f.OwnerID = toKind, toID
		}
		if f.UserKind == fromKind && f.UserID == fromID {
			f.UserKind, f.UserID = toKind, toID
		}
	}
	for i := range w.Production.CareAgreements {
		a := &w.Production.CareAgreements[i]
		if a.OwnerKind == fromKind && a.OwnerID == fromID {
			a.OwnerKind, a.OwnerID = toKind, toID
		}
		if a.CaretakerKind == fromKind && a.CaretakerID == fromID {
			a.CaretakerKind, a.CaretakerID = toKind, toID
		}
		if a.SupplierKind == fromKind && a.SupplierID == fromID {
			a.SupplierKind, a.SupplierID = toKind, toID
		}
	}
	for i := range w.Production.CareProposals {
		a := &w.Production.CareProposals[i]
		if a.OwnerKind == fromKind && a.OwnerID == fromID {
			a.OwnerKind, a.OwnerID = toKind, toID
		}
		if a.CaretakerKind == fromKind && a.CaretakerID == fromID {
			a.CaretakerKind, a.CaretakerID = toKind, toID
		}
		if a.SupplierKind == fromKind && a.SupplierID == fromID {
			a.SupplierKind, a.SupplierID = toKind, toID
		}
	}
	from, to := holderKey(fromKind, fromID), holderKey(toKind, toID)
	if w.Production.YieldCredit != nil {
		fromPrefix, toPrefix := from+"/", to+"/"
		for key, n := range w.Production.YieldCredit {
			if !strings.HasPrefix(key, fromPrefix) {
				continue
			}
			w.Production.YieldCredit[toPrefix+strings.TrimPrefix(key, fromPrefix)] += n
			delete(w.Production.YieldCredit, key)
		}
		if len(w.Production.YieldCredit) == 0 {
			w.Production.YieldCredit = nil
		}
	}
	if old := w.Production.ToolWear[from]; old != nil {
		if w.Production.ToolWear[to] == nil {
			w.Production.ToolWear[to] = map[string]int64{}
		}
		for item, n := range old {
			w.Production.ToolWear[to][item] += n
		}
		delete(w.Production.ToolWear, from)
	}
	// A transfer keeps care debt; it does not reset a hungry animal's clock.
	for key, s := range w.Production.LivingCare {
		if s.HolderKind == fromKind && s.HolderID == fromID {
			s.HolderKind, s.HolderID = toKind, toID
			delete(w.Production.LivingCare, key)
			next := key // In-process animals retain their batch-specific clock.
			if key == from+"/"+s.Item {
				next = to + "/" + s.Item
			}
			if other, ok := w.Production.LivingCare[next]; ok {
				s.ShortageTicks = max(s.ShortageTicks, other.ShortageTicks)
				s.LastTick = min(s.LastTick, other.LastTick)
			}
			w.Production.LivingCare[next] = s
		}
	}
}

func (w *World) careAgreement(g GrowthBatch) *GrowthCareAgreement {
	for i := range w.Production.CareAgreements {
		a := &w.Production.CareAgreements[i]
		if a.GrowthID == g.ID && a.OwnerKind == holderKind(g.Cohort) && a.OwnerID == g.OwnerID && a.OwnerAccepted && a.CaretakerAccepted {
			return a
		}
	}
	return nil
}

// SetGrowthCare records explicit assent from both named parties. Callers may
// use an existing negotiated agreement to supply these terms.
func (w *World) SetGrowthCare(a GrowthCareAgreement) error {
	if err := a.validateSigned(); err != nil {
		return err
	}
	for i := range w.Growth {
		g := &w.Growth[i]
		if g.ID != a.GrowthID {
			continue
		}
		if a.OwnerKind != holderKind(g.Cohort) || a.OwnerID != g.OwnerID {
			return fmt.Errorf("care owner does not own growth")
		}
		if !w.growthOwnerPresent(GrowthBatch{OwnerID: a.CaretakerID, Cohort: a.CaretakerKind == "cohort", District: g.District, VenueID: g.VenueID}) {
			return fmt.Errorf("caretaker is not available at the production site")
		}
		if g.Harvested || w.careAgreement(*g) != nil {
			return fmt.Errorf("growth %d is harvested or already under contract", g.ID)
		}
		w.Production.CareAgreements = append(w.Production.CareAgreements, a)
		w.Production.CareProposals = slices.DeleteFunc(w.Production.CareProposals, func(p GrowthCareAgreement) bool { return p.GrowthID == g.ID })
		if len(w.Production.CareProposals) == 0 {
			w.Production.CareProposals = nil
		}
		return nil
	}
	return fmt.Errorf("unknown growth batch")
}

func (p Perception) districtHasRecipeResource(districtID string, r Recipe) bool {
	if r.ResourcePool == "" {
		return true
	}
	d, ok := p.Map.DistrictByID(districtID)
	if !ok {
		return false
	}
	capacity, explicit := d.ResourceCapacities[r.ResourcePool]
	placed := capacity > 0
	if !explicit {
		for _, f := range p.Catalog.Resources[r.ResourcePool].NodeFeatures {
			if p.Map.DistrictHasFeature(d.ID, f) {
				placed = true
			}
		}
	}
	if d.ID == p.Self.Location && p.DistrictState != nil {
		placed = p.DistrictState.AvailableResource(r.ResourcePool) >= int64(max(1, r.ResourceDraw))
	}
	return placed
}

func (p Perception) RecipeResourceVenue(r Recipe) (string, bool) {
	a := p.Self
	best := ""
	bestCost := int(^uint(0) >> 1)
	for _, v := range p.Map.Venues {
		if r.Growth != nil && r.NodeFeature == "farmland" && !p.hasAvailableFieldAt(v.DistrictID, v.ID) {
			continue
		}
		if !p.Map.VenueOrDistrictHasFeature(v.ID, v.DistrictID, r.NodeFeature) {
			continue
		}
		if !p.districtHasRecipeResource(v.DistrictID, r) {
			continue
		}
		cost := 0
		if v.ID != a.VenueID {
			_, n, _, ok := p.Map.NextVenueHopForActor(p.Catalog, nil, a, a.VenueID, v.ID)
			if !ok {
				continue
			}
			cost = n
		}
		if cost < bestCost {
			best, bestCost = v.ID, cost
		}
	}
	return best, best != ""
}

func (p Perception) RecipeResourceDistrict(r Recipe) (string, bool) {
	best := ""
	bestCost := int(^uint(0) >> 1)
	for _, d := range p.Map.Districts {
		if r.Growth != nil && r.NodeFeature == "farmland" && !p.hasAvailableFieldAt(d.ID, "") {
			continue
		}
		if !p.Map.DistrictHasFeature(d.ID, r.NodeFeature) {
			continue
		}
		if !p.districtHasRecipeResource(d.ID, r) {
			continue
		}
		cost := 0
		if d.ID != p.Self.Location {
			_, n, ok := p.Map.NextHop(p.Self.Location, d.ID)
			if !ok {
				continue
			}
			cost = n
		}
		if cost < bestCost {
			best, bestCost = d.ID, cost
		}
	}
	return best, best != ""
}

func (p Perception) hasAvailableFieldAt(district, venue string) bool {
	for _, field := range p.Fields {
		if field.District == district && (field.VenueID == "" || field.VenueID == venue) && field.AvailableAreaM2 >= p.Catalog.Production.FieldAreaPerBatchM2 {
			return true
		}
	}
	return false
}
