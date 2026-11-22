package pack

import (
	"testing"

	"github.com/changw98ic/world-relation-evo/internal/sim"
)

func TestWorldPackImportsInitialSocialStateAndNonAgrarianLifecycle(t *testing.T) {
	p := &WorldPack{
		ID:    "court-intrigue",
		Name:  "Court Intrigue",
		Genre: "historical_court",
		Map: sim.WorldMap{
			Districts: []sim.District{
				{ID: "palace", Name: "Palace", Features: []string{"public_forum", "authority_seat"}},
				{ID: "manor", Name: "Manor", Features: []string{"market"}},
			},
			Roads: []sim.Road{
				{From: "palace", To: "manor", Ticks: 1, EnergyCost: 1},
			},
		},
		Catalog: sim.Catalog{
			Items: map[string]sim.ItemSpec{
				"ration": {ID: "ration", Name: "Ration", Edible: true, Tradeable: true, MassUnits: 1, NutritionUnits: 1},
			},
			WorldEvents: []sim.WorldEventSpec{
				{ID: "imperial_edict", Name: "Imperial Edict", Category: "artifact_emergence", DurationTicks: 96, Intensity: 70},
			},
		},
		Factions: []sim.Faction{
			{ID: "crown", Name: "Crown", District: "palace", Members: []string{"prince"}},
			{ID: "council", Name: "Council", District: "manor", Members: []string{"chancellor"}},
		},
		Actors: []sim.Actor{
			{
				ID: "prince", Name: "Prince", Role: "noble", Tags: []string{"resident", "ruler"},
				Location: "palace", Health: 100, Energy: 100, Money: 50,
				Items: map[string]int{"ration": 10},
				Memories: []sim.Memory{
					{Kind: "observation", EventID: 42, RootEventID: 42, Text: "Prior assassination attempt"},
				},
			},
			{
				ID: "chancellor", Name: "Chancellor", Role: "minister", Tags: []string{"resident", "ruler"},
				Location: "manor", Health: 100, Energy: 100, Money: 50,
				Items: map[string]int{"ration": 10},
			},
		},
		Relations: &sim.RelationEvoState{
			FactionLeader:      map[string]string{"crown": "prince", "council": "chancellor"},
			ControllingFaction: map[string]string{"palace": "crown", "manor": "council"},
			Grudges: []sim.GrudgeRecord{
				{ID: "g_init", AggrievedID: "prince", OffenderID: "chancellor", Cause: "prior_betrayal", Intensity: 70},
			},
		},
		Diplomacy: &sim.DiplomacyState{
			Relations: map[string]*sim.FactionDiplomacy{
				"council|crown": {FactionA: "council", FactionB: "crown", Stance: "hostile", RelationScore: -65, Reason: "succession_crisis"},
			},
		},
		Secrets: []sim.ActorSecret{
			{OwnerID: "chancellor", Kind: "hidden_plot", SubjectID: "prince", Severity: 80, KnownBy: []string{"chancellor"}},
		},
	}

	w, err := p.Instantiate("")
	if err != nil {
		t.Fatalf("expected non-agrarian court pack with initial social state to instantiate cleanly, got: %v", err)
	}
	if w.NextID <= 42 {
		t.Fatalf("expected NextID > 42 to avoid colliding with seeded memory EventID, got %d", w.NextID)
	}
	if w.Catalog.Lifecycle.WasteItem != "" || w.Catalog.Lifecycle.CompostRecipeID != "" {
		t.Fatalf("expected non-agrarian pack not to inject nightsoil/compost defaults: %+v", w.Catalog.Lifecycle)
	}
	if len(w.DistrictStates) == 0 {
		t.Fatal("expected DistrictStates initialized when WorldEvents are declared")
	}
	if w.Relations.FactionLeader["crown"] != "prince" || len(w.Relations.Grudges) != 1 {
		t.Fatalf("initial relations were not preserved: %+v", w.Relations)
	}
	if w.Diplomacy == nil || w.Diplomacy.Relations["council|crown"].Stance != "hostile" {
		t.Fatalf("initial diplomacy was not preserved: %+v", w.Diplomacy)
	}
	if len(w.Secrets) != 1 || w.Secrets[0].OwnerID != "chancellor" {
		t.Fatalf("initial secrets were not preserved: %+v", w.Secrets)
	}
	if err := w.VerifyLedgerConservation(); err != nil {
		t.Fatalf("initial ledger conservation failed: %v", err)
	}
}
