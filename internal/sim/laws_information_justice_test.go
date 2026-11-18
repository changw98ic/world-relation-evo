package sim

import (
	"bytes"
	"testing"
)

// K1: statements retain their speaker and causal source; repeating a statement
// never turns it into an observation or overwrites physical stock.
func TestLawInformationSpeechPreservesSourceWithoutCreatingGoods(t *testing.T) {
	w := lawWorld()
	w.NextID = 50
	w.Actors[0].Items["wood"] = 3
	w.Actors[0].Memories = []Memory{{EventID: 41, RootEventID: 40, Kind: "crafted", Source: "a", Observation: &Observation{Kind: "item_produced", Item: "wood", Quantity: 3}}}
	lawRejected(t, w.Apply("a", Action{Kind: "speak", TargetID: "b", Text: "I saw it", BasisEventID: 999}))
	if len(w.Actors[1].Memories) != 0 {
		t.Fatal("unknown basis was delivered as personal evidence")
	}
	claim := &Statement{Topic: "stock", Item: "wood", Location: "home", Quantity: 999}
	lawOK(t, w.Apply("a", Action{Kind: "speak", TargetID: "b", Text: "There are 999 wood units", Statement: claim, BasisEventID: 41}))
	if len(w.Actors[1].Memories) != 1 || len(w.Actors[2].Memories) != 0 {
		t.Fatal("private speech reached someone other than the addressed listener")
	}
	first := w.Actors[1].Memories[0]
	if first.Source != "a" || first.RootEventID != 40 || first.BasisEventID != 41 || first.Observation != nil || first.Statement == nil || first.Statement.Quantity != 999 {
		t.Fatalf("hearsay lost its source or became an observation: %+v", first)
	}
	lawOK(t, w.Apply("b", Action{Kind: "speak", TargetID: "c", Text: "a told me about the wood", Statement: claim, BasisEventID: first.EventID}))
	last := w.Actors[2].Memories[0]
	if last.Source != "b" || last.RootEventID != 40 || last.BasisEventID != first.EventID || last.Observation != nil || last.Statement == nil {
		t.Fatal("second-hand report lost the original causal chain")
	}
	if w.Actors[0].Items["wood"] != 3 || w.Actors[1].Items["wood"] != 0 || w.Actors[2].Items["wood"] != 0 || w.Actors[0].Memories[0].Observation.Quantity != 3 {
		t.Fatal("speech rewrote stock or the original observation")
	}
}

func TestLawInformationRecallSelectsOnlyOwnedHistoryAndDoesNotMutateIt(t *testing.T) {
	w := lawWorld()
	a := &w.Actors[0]
	a.Memories = []Memory{{EventID: 1, Source: "b", Kind: "speech", Statement: &Statement{Topic: "grain", Quantity: 999}}}
	for id := int64(2); id <= 40; id++ {
		a.Memories = appendMemory(a.Memories, Memory{EventID: id, Source: "a", Kind: "wait"})
	}
	before := lawJSON(t, *a)
	recalled := RecallMemories(*a, MemoryCues{Terms: []string{"grain"}}, 10)
	seen, foundClaim := map[int64]bool{}, false
	for _, m := range recalled {
		if m.EventID < 1 || m.EventID > 40 || seen[m.EventID] {
			t.Fatal("recall invented or duplicated personal evidence")
		}
		seen[m.EventID] = true
		if m.EventID == 1 {
			foundClaim = m.Observation == nil && m.Statement != nil && m.Statement.Quantity == 999
			m.Statement.Quantity = 0
		}
	}
	if !foundClaim || len(recalled) != 10 || !bytes.Equal(before, lawJSON(t, *a)) {
		t.Fatal("recall erased relevant owned history, assessed hearsay as truth, or aliased stored evidence")
	}
	if len(RecallMemories(w.Actors[1], MemoryCues{Terms: []string{"grain"}}, 10)) != 0 {
		t.Fatal("another actor recalled evidence they never received")
	}
}

func TestLawInformationResearchCostsWorkBeforePersonalDiscovery(t *testing.T) {
	w := lawWorld()
	w.Catalog.Techs = map[string]TechSpec{"joinery": {ID: "joinery", ResearchTicks: 3, ResearchEnergyCost: 4, ResearchInputs: map[string]int{"wood": 1}, FacilityName: "better_bench", ProductionBonusPct: 50}}
	w.Actors[0].Items["wood"] = 1
	lawOK(t, w.Apply("a", Action{Kind: "research_tech", TechID: "joinery"}))
	if w.Actors[0].Items["wood"] != 0 || w.Actors[0].Energy != 96 || w.KnowsTech("a", "joinery") {
		t.Fatal("research discovery preceded its declared materials, energy or work")
	}
	w.Tick++
	w.Advance("a")
	if w.KnowsTech("a", "joinery") {
		t.Fatal("three-tick research finished after two ticks")
	}
	w.Tick++
	lawOK(t, w.Advance("a"))
	if !w.KnowsTech("a", "joinery") || w.KnowsTech("b", "joinery") || w.KnowsTech("c", "joinery") || w.FacilityProductionBonusPct(w.Actors[0]) != 0 {
		t.Fatal("personal discovery became global knowledge or a built physical facility")
	}
}

func TestLawInformationTeachingRequiresKnowledgeAndMeasuredChannel(t *testing.T) {
	w := lawWorld()
	w.Catalog.Techs = map[string]TechSpec{"joinery": {ID: "joinery", FacilityName: "better_bench", ProductionBonusPct: 50}}
	w.Catalog.CommChannels = map[string]CommChannelSpec{"radio": {ID: "radio", RangeKm: 1, EnergyCost: 3}}
	w.Knowledge = &KnowledgeBook{Records: map[string]*KnowledgeRecord{"joinery": {TechID: "joinery", KnownActors: []string{"a"}}}}
	w.Actors[1].Location = "away"
	x := Action{Kind: "teach_knowledge", TargetID: "b", TechID: "joinery", Channel: "radio"}
	lawRejected(t, w.Apply("c", x))
	lawRejected(t, w.Apply("a", x)) // neither a distance nor a local path is known
	if w.KnowsTech("b", "joinery") || w.Actors[0].Energy != 100 {
		t.Fatal("unknown distance delivered knowledge or charged channel energy")
	}
	w.Map.Districts[0].GeoRegionID, w.Map.Districts[1].GeoRegionID = "r0", "r1"
	w.Map.GeoRegions = []GeoRegion{{ID: "r0"}, {ID: "r1", Coord: GeoCoord{X: 2}}}
	lawRejected(t, w.Apply("a", x))
	if w.KnowsTech("b", "joinery") || w.Actors[0].Energy != 100 {
		t.Fatal("out-of-range teaching crossed the channel boundary")
	}
	w.Map.GeoRegions[1].Coord.X = 1
	lawOK(t, w.Apply("a", x))
	if !w.KnowsTech("b", "joinery") || w.KnowsTech("c", "joinery") || w.Actors[0].Energy != 97 || w.FacilityProductionBonusPct(w.Actors[1]) != 0 {
		t.Fatal("exact-range transmission failed or built a facility merely by teaching it")
	}
}

// J1: the engine may store a court case, but an individual needs their own
// witnessed/reported lead and physical access to that court before acting on it.
func TestLawJusticeAppealUsesPersonalEvidenceAndLocalCourt(t *testing.T) {
	w := lawWorld()
	w.JusticeRevolt = NewJusticeRevoltState()
	w.Actors[0].Tags = append(w.Actors[0].Tags, "law")
	w.Actors[0].IllicitGain = 1
	w.Actors[2].Location = "away"
	w.Actors = append(w.Actors, lawPerson("witness"))
	lawOK(t, w.Apply("a", Action{Kind: "frame_suspect", TargetID: "b"}))
	if w.Actors[1].JailedUntil <= w.Tick || len(w.JusticeRevolt.CourtCases) != 1 {
		t.Fatal("fixture did not establish an actual wrongful confinement")
	}
	w.Actors[2].Location = "home"
	lawRejected(t, w.Apply("c", Action{Kind: "file_appeal", TargetID: "b"}))
	if w.Actors[1].JailedUntil <= w.Tick || w.Actors[2].Energy != 100 {
		t.Fatal("global court record was used as an unknown character's evidence")
	}
	w.Actors[3].Location = "away"
	lawRejected(t, w.Apply("witness", Action{Kind: "file_appeal", TargetID: "b"}))
	if w.Actors[1].JailedUntil <= w.Tick || w.Actors[3].Energy != 100 {
		t.Fatal("personal knowledge released a prisoner from a distant court")
	}
	w.Actors[3].Location = "home"
	lawOK(t, w.Apply("witness", Action{Kind: "file_appeal", TargetID: "b"}))
	if w.Actors[1].JailedUntil != 0 || w.JusticeRevolt.CourtCases[0].Status != "overturned" || w.Actors[3].Energy != 96 {
		t.Fatal("local informed appeal failed to release its actual victim")
	}
}
