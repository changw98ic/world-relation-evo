package pack

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/changw98ic/world-relation-evo/internal/sim"
)

func TestBlueprintPreservesExplicitZeroAndFalse(t *testing.T) {
	raw := `{
		"schema_version": 2,
		"id": "zero-semantics-test",
		"name": "Zero Semantics",
		"genre": "scifi_wasteland",
		"supply_days": 0,
		"forms": [
			{"id": "civilian_zero", "name": "Civilian", "power_tier": 0}
		],
		"custom_items": [
			{"id": "soul_bound_relic", "name": "Soul Relic", "tier": 4, "tradeable": false, "batch_price": 0, "mass_units": 0}
		],
		"districts": [
			{"id": "d1", "name": "D1", "features": []},
			{"id": "d2", "name": "D2", "features": ["market"]}
		],
		"roads": [
			{"from": "d1", "to": "d2", "ticks": 1, "energy_cost": 0, "medium": "land"}
		],
		"factions": [
			{"id": "f1", "name": "F1", "district": "d1"},
			{"id": "f2", "name": "F2", "district": "d2"}
		],
		"diplomacy": [
			{"faction_a": "f1", "faction_b": "f2", "stance": "hostile", "relation_score": 0}
		],
		"actors": [
			{"id": "pauper", "name": "Pauper", "role": "wanderer", "location": "d1", "money": 0, "age_years": 0}
		],
		"world_events": [
			{"id": "manual_only_event", "name": "Manual Event", "category": "artifact_emergence", "prob_permille_per_day": 0, "duration_days": 0, "intensity": 0},
			{"id": "event_with_defaults", "name": "Omitted Settings", "category": "artifact_emergence"}
		]
	}`
	bp, err := DecodeBlueprint(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("DecodeBlueprint failed: %v", err)
	}
	wp, err := CompileBlueprint(*bp)
	if err != nil {
		t.Fatalf("CompileBlueprint failed: %v", err)
	}
	if wp.Actors[0].Money != 0 {
		t.Fatalf("expected explicit money=0 to be preserved, got %d", wp.Actors[0].Money)
	}
	if wp.Actors[0].AgeYears != 0 {
		t.Fatalf("expected explicit age_years=0 to be preserved, got %d", wp.Actors[0].AgeYears)
	}
	if wp.Actors[0].Items["ration"] != 0 {
		t.Fatalf("expected explicit supply_days=0 to yield 0 initial rations, got %d", wp.Actors[0].Items["ration"])
	}
	if wp.Catalog.Forms["civilian_zero"].PowerTier != 0 {
		t.Fatalf("expected explicit power_tier=0 to be preserved, got %d", wp.Catalog.Forms["civilian_zero"].PowerTier)
	}
	if len(wp.Map.Districts[0].Features) != 0 {
		t.Fatalf("expected explicit empty features=[] wilderness to stay empty, got %v", wp.Map.Districts[0].Features)
	}
	if wp.Map.Roads[0].EnergyCost != 0 {
		t.Fatalf("expected explicit energy_cost=0 to be preserved, got %d", wp.Map.Roads[0].EnergyCost)
	}
	relic := wp.Catalog.Items["soul_bound_relic"]
	if relic.Tradeable || relic.BatchPrice != 0 || relic.MassUnits != 0 {
		t.Fatalf("expected tradeable=false batch_price=0 mass_units=0, got %+v", relic)
	}
	if wp.Diplomacy == nil || wp.Diplomacy.Relations["f1|f2"] == nil || wp.Diplomacy.Relations["f1|f2"].RelationScore != 0 {
		t.Fatalf("expected explicit relation_score=0 on hostile diplomacy to be preserved, got %+v", wp.Diplomacy)
	}
	if wp.Catalog.WorldEvents[0].ProbPermillePerDay != 0 {
		t.Fatalf("expected explicit prob_permille_per_day=0 to be preserved, got %d", wp.Catalog.WorldEvents[0].ProbPermillePerDay)
	}
	explicit, omitted := wp.Catalog.WorldEvents[0], wp.Catalog.WorldEvents[1]
	if explicit.DurationTicks != 0 || explicit.Intensity != 0 || omitted.DurationTicks != 5*sim.TicksPerDay || omitted.Intensity != 70 {
		t.Fatalf("blueprint event times and intensities: %+v", wp.Catalog.WorldEvents)
	}
	// Genre-neutral ASCII defaults (no hardcoded Chinese nouns on English scifi blueprint).
	if wp.Environment.CurrencyName != "coins" || wp.Environment.ActorNoun != "actor" || wp.Houses[0].Name != "Pauper Quarters" {
		t.Fatalf("expected genre-neutral ASCII defaults, got currency=%q noun=%q house=%q",
			wp.Environment.CurrencyName, wp.Environment.ActorNoun, wp.Houses[0].Name)
	}
}

func TestBlock4ZeroValueFidelityEffectiveSpecsEcologyAndAgeless(t *testing.T) {
	// 1. All 5 Effective*Spec preserve explicit zero when ExplicitZero=true, while preserving defaults when false.
	cat := sim.DefaultCatalog()
	cat.Governance = sim.GovernanceSpec{ExplicitZero: true, DailyPollTaxPermille: 0, MarketTaxPermille: 0, RacketRatePermille: 0}
	if g := cat.EffectiveGovernance(); g.DailyPollTaxPermille != 0 || g.MarketTaxPermille != 0 || g.RacketRatePermille != 0 {
		t.Fatalf("GovernanceSpec ExplicitZero overwritten: %+v", g)
	}
	mc := sim.EffectiveMarketCreditSpec(sim.MarketCreditSpec{ExplicitZero: true, DailyLoanInterestPermille: 0, DailyRentPermille: 0})
	if mc.DailyLoanInterestPermille != 0 || mc.DailyRentPermille != 0 {
		t.Fatalf("MarketCreditSpec ExplicitZero overwritten: %+v", mc)
	}
	rel := sim.EffectiveRelationEvoSpec(sim.RelationEvoSpec{ExplicitZero: true, DefaultDowryCoins: 0, CharterBaseFee: 0})
	if rel.DefaultDowryCoins != 0 || rel.CharterBaseFee != 0 {
		t.Fatalf("RelationEvoSpec ExplicitZero overwritten: %+v", rel)
	}
	jr := sim.EffectiveJusticeRevoltSpec(sim.JusticeRevoltSpec{ExplicitZero: true, AppealCompensationCoins: 0, CorruptionFramingThreshold: 0})
	if jr.AppealCompensationCoins != 0 || jr.CorruptionFramingThreshold != 0 {
		t.Fatalf("JusticeRevoltSpec ExplicitZero overwritten: %+v", jr)
	}
	sr := sim.EffectiveSpotlightRumorSpec(sim.SpotlightRumorSpec{ExplicitZero: true, DailyDemotionCap: 0, DailyPromotionCap: 0})
	if sr.DailyDemotionCap != 0 || sr.DailyPromotionCap != 0 {
		t.Fatalf("SpotlightRumorSpec ExplicitZero overwritten: %+v", sr)
	}

	// Ecology.Enabled=false and Lifecycle.Ageless=true are explicit configuration.
	enabled := false
	wp := &WorldPack{
		SchemaVersion: 2,
		ID:            "v2-zero-pack",
		Name:          "V2 Zero Pack",
		Genre:         "scifi",
		Catalog: sim.Catalog{
			Lifecycle:   sim.LifecycleSpec{Ageless: true, MaxExpectancy: 0},
			WorldEvents: []sim.WorldEventSpec{{ID: "ev1", Name: "Ev1", Category: "artifact_emergence", DurationTicks: 10}},
		},
		Map:     sim.WorldMap{Districts: []sim.District{{ID: "d1", Name: "D1"}}},
		Actors:  []sim.Actor{{ID: "a1", Name: "A1", Location: "d1", Health: 100, Energy: 100}},
		Ecology: &sim.EcologySpec{Enabled: &enabled},
	}
	w, err := wp.Instantiate("v2-test")
	if err != nil {
		t.Fatalf("Instantiate failed: %v", err)
	}
	if !w.Catalog.Lifecycle.Ageless || w.Catalog.Lifecycle.MaxExpectancy != 0 {
		t.Fatalf("expected ageless lifecycle preserved, got %+v", w.Catalog.Lifecycle)
	}
	if len(w.DistrictStates) != 0 {
		t.Fatalf("expected v2 Ecology.Enabled=false to skip InitEcology, got %d DistrictStates", len(w.DistrictStates))
	}
}

func TestWorldPackValidatesNamedFieldsAndPreservesDeclaredValues(t *testing.T) {
	const base = `{"id":"config-test","map":{"districts":[{"id":"place"}]},"actors":[]`
	for _, tc := range []struct{ settings, field string }{
		{`,"governance":{"daily_poll_tax_permillle":0}`, "daily_poll_tax_permillle"},
		{`,"market_credit":{"daily_loan_interest_permille":-1}`, "daily_loan_interest_permille"},
		{`,"market_credit":{"max_tenant_share_permille":1001}`, "max_tenant_share_permille"},
		{`,"catalog":{"world_events":[{"id":"instant","category":"terrain_shift","duration_ticks":-1}]}`, "duration_ticks"},
	} {
		if _, err := Decode(strings.NewReader(base + tc.settings + "}")); err == nil || !strings.Contains(err.Error(), tc.field) {
			t.Errorf("expected error for %s, got %v", tc.settings, err)
		}
	}
	for _, share := range []string{"980", "1000"} {
		raw := base + `,"market_credit":{"max_tenant_share_permille":` + share + `},
			"environment":{"forum_features":[],"enforcement_tags":[],"oversight_tags":[],"outlaw_tags":[]},
			"catalog":{"world_events":[{"id":"instant","category":"terrain_shift","duration_ticks":0,"intensity":0},
			{"id":"default","category":"terrain_shift"}]}}`
		p, err := Decode(strings.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		w, err := p.Instantiate("")
		if err != nil {
			t.Fatal(err)
		}
		snapshot, err := json.Marshal(w)
		if err != nil {
			t.Fatal(err)
		}
		w, err = sim.DecodeWorldSnapshot(snapshot)
		if err != nil {
			t.Fatal(err)
		}
		env := w.Catalog.EffectiveEnvironment()
		for _, tags := range [][]string{env.ForumFeatures, env.EnforcementTags, env.OversightTags, env.OutlawTags} {
			if tags == nil || len(tags) != 0 {
				t.Fatalf("expected explicit empty lists after snapshot, got %+v", env)
			}
		}
		if got, want := w.MarketCredit.Spec.MaxTenantSharePermille, p.MarketCredit.MaxTenantSharePermille; got != want {
			t.Fatalf("tenant share = %d, want %d", got, want)
		}
		w.TriggerWorldEvent("instant", "place")
		w.TriggerWorldEvent("default", "place")
		if len(w.ActiveWorldEvents) != 2 {
			t.Fatalf("expected both configured events: %+v", w.ActiveWorldEvents)
		}
		instant, normal := w.ActiveWorldEvents[0], w.ActiveWorldEvents[1]
		if instant.Intensity != 0 || instant.StartedTick != instant.ExpiresTick || normal.Intensity != 55 || normal.ExpiresTick-normal.StartedTick != 3*sim.TicksPerDay {
			t.Fatalf("expected explicit zeros and omitted defaults: %+v", w.ActiveWorldEvents)
		}
	}
}

func TestV2WorldPackPartialNumericZeroOverrides(t *testing.T) {
	const raw = `{
		"schema_version": 2,
		"id": "partial-zero", "name": "Partial Zero", "genre": "generic",
		"catalog": {},
		"map": {"districts": [{"id": "d1", "name": "D1"}]},
		"actors": [{"id": "a1", "name": "A1", "location": "d1", "health": 100, "energy": 100}],
		"governance": {"daily_poll_tax_permille": 0},
		"market_credit": {"daily_loan_interest_permille": 0},
		"relations": {"spec": {"charter_base_fee": 0}},
		"justice_revolt": {"appeal_compensation_coins": 0},
		"spotlight_rumor": {"daily_promotion_cap": 0}
	}`
	wp, err := Decode(strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	packJSON, err := json.Marshal(wp)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := Decode(bytes.NewReader(packJSON))
	if err != nil {
		t.Fatal(err)
	}
	for name, pack := range map[string]*WorldPack{"decoded": wp, "reopened": reopened} {
		t.Run(name, func(t *testing.T) {
			w, err := pack.Instantiate("")
			if err != nil {
				t.Fatal(err)
			}
			checkPartialNumericZeroSpecs(t, w)
			snapshot, err := json.Marshal(w)
			if err != nil {
				t.Fatal(err)
			}
			restored, err := sim.DecodeWorldSnapshot(snapshot)
			if err != nil {
				t.Fatal(err)
			}
			checkPartialNumericZeroSpecs(t, restored)
		})
	}
}

func checkPartialNumericZeroSpecs(t *testing.T, w *sim.World) {
	t.Helper()
	tests := []struct {
		zeroKey       string
		got, defaults any
	}{
		{"daily_poll_tax_permille", w.Catalog.EffectiveGovernance(), sim.DefaultGovernanceSpec()},
		{"daily_loan_interest_permille", sim.EffectiveMarketCreditSpec(w.MarketCredit.Spec), sim.DefaultMarketCreditSpec()},
		{"charter_base_fee", sim.EffectiveRelationEvoSpec(w.Relations.Spec), sim.DefaultRelationEvoSpec()},
		{"appeal_compensation_coins", sim.EffectiveJusticeRevoltSpec(*w.JusticeRevolt.Spec), sim.DefaultJusticeRevoltSpec()},
		{"daily_promotion_cap", sim.EffectiveSpotlightRumorSpec(w.SpotlightRumor.Spec), sim.DefaultSpotlightRumorSpec()},
	}
	fields := func(spec any) map[string]json.RawMessage {
		t.Helper()
		data, err := json.Marshal(spec)
		if err != nil {
			t.Fatal(err)
		}
		var result map[string]json.RawMessage
		if err := json.Unmarshal(data, &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	for _, test := range tests {
		want, got := fields(test.defaults), fields(test.got)
		want[test.zeroKey] = json.RawMessage("0")
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s should override just one field. Got %v, want %v", test.zeroKey, got, want)
		}
	}
}

func TestWorldPackDistinguishesDisabledEcologyAndExplicitLifecycleZero(t *testing.T) {
	const raw = `{"schema_version":2,"id":"zero-policies","catalog":{"calendar":{"seasons":[]},"lifecycle":{"annual_birth_permille":0,"max_expectancy":0}},"map":{"districts":[{"id":"a"},{"id":"b"}]},"actors":[],"ecology":{"enabled":false,"district_states":{"a":{"sanitation":0},"b":{"weather":"clear"}}}}`
	p, err := Decode(strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	w, err := p.Instantiate("")
	if err != nil {
		t.Fatal(err)
	}
	if len(w.DistrictStates) > 0 || w.Catalog.Lifecycle.AnnualBirthPermille != 0 ||
		w.Catalog.Lifecycle.MaxExpectancy != 0 || !w.Catalog.Lifecycle.Ageless ||
		w.Catalog.Calendar.Seasons == nil || len(w.Catalog.Calendar.Seasons) != 0 {
		t.Fatalf("instantiation changed explicit overrides: %+v", w.Catalog)
	}
	*p.Ecology.Enabled = true
	w, err = p.Instantiate("")
	if err != nil {
		t.Fatal(err)
	}
	if w.DistrictStates["a"].Sanitation != 0 || w.DistrictStates["b"].Sanitation == 0 {
		t.Fatalf("omitted sanitation and explicit zero were treated identically: %+v", w.DistrictStates)
	}
}
