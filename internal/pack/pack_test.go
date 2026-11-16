package pack

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/changw98ic/world-relation-evo/internal/agent"
	"github.com/changw98ic/world-relation-evo/internal/sim"
)

func assertJSONRoundTrip(t *testing.T, w *sim.World, label string) {
	t.Helper()
	raw, err := json.Marshal(w)
	if err != nil {
		t.Fatalf("%s: marshal world: %v", label, err)
	}
	var decoded sim.World
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("%s: unmarshal world: %v", label, err)
	}
	if !reflect.DeepEqual(*w, decoded) {
		raw2, _ := json.Marshal(&decoded)
		t.Fatalf("%s: JSON round-trip mismatch\nbefore len=%d\nafter  len=%d", label, len(raw), len(raw2))
	}
}

func TestReferenceWorldPacksRun32TicksZeroFailuresAndRoundTrip(t *testing.T) {
	packFiles := []struct {
		file  string
		genre string
	}{
		{file: "mortal_watershed.json", genre: "historical"},
		{file: "xianxia_sect.json", genre: "xianxia"},
		{file: "wasteland_sector.json", genre: "scifi_wasteland"},
	}

	ctx := context.Background()
	decider := agent.Offline()

	for _, tc := range packFiles {
		t.Run(tc.file, func(t *testing.T) {
			path := filepath.Join("..", "..", "packs", tc.file)
			p, err := LoadFile(path)
			if err != nil {
				t.Fatalf("LoadFile(%s): %v", path, err)
			}
			if p.Genre != tc.genre {
				t.Fatalf("genre = %q, want %q", p.Genre, tc.genre)
			}

			w, err := p.Instantiate("")
			if err != nil {
				t.Fatalf("Instantiate: %v", err)
			}
			assertJSONRoundTrip(t, w, tc.file+" initial")

			wOverride, err := p.Instantiate("custom-" + p.ID)
			if err != nil {
				t.Fatalf("Instantiate override: %v", err)
			}
			if wOverride.ID != "custom-"+p.ID {
				t.Fatalf("override ID = %q, want %q", wOverride.ID, "custom-"+p.ID)
			}

			var failed []sim.Event
			totalEvents := 0
			for tick := 0; tick < 32; tick++ {
				events, err := w.Step(ctx, decider)
				if err != nil {
					t.Fatalf("tick %d Step error: %v", tick, err)
				}
				totalEvents += len(events)
				for _, ev := range events {
					if ev.Kind == "action_failed" {
						failed = append(failed, ev)
					}
				}
			}
			if len(failed) > 0 {
				t.Fatalf("got %d action_failed events over 32 ticks; first=%+v", len(failed), failed[0])
			}
			if totalEvents == 0 {
				t.Fatalf("expected events during 32 ticks, got 0")
			}
			if err := w.VerifyLedgerConservation(); err != nil {
				t.Fatalf("post-32-tick conservation error in %s: %v", tc.file, err)
			}
			if cons := sim.InspectWorldConservation(w); !cons.LedgerConserved {
				t.Fatalf("post-32-tick InspectWorldConservation failed in %s: %+v", tc.file, cons)
			}

			assertJSONRoundTrip(t, w, tc.file+" after 32 ticks")
		})
	}
}

func TestReferencePackDomainCapabilities(t *testing.T) {
	// 1. Mortal Watershed: river GeoFlowEdge + Province->County->Village PolityNetwork.
	mw, err := LoadFile(filepath.Join("..", "..", "packs", "mortal_watershed.json"))
	if err != nil {
		t.Fatalf("load mortal_watershed: %v", err)
	}
	mwWorld, err := mw.Instantiate("")
	if err != nil {
		t.Fatalf("instantiate mortal_watershed: %v", err)
	}
	if len(mwWorld.Map.GeoRegions) < 2 || len(mwWorld.Map.GeoFlows) == 0 {
		t.Fatalf("mortal_watershed missing GeoRegions/GeoFlows")
	}
	if mwWorld.Polities == nil || !mwWorld.Polities.PolityGovernsNode("qingzhou_province", "qingxi_village") {
		t.Fatalf("expected qingzhou_province to govern qingxi_village")
	}

	// 2. Xianxia Sect: divine_sense, sword_flight, sound_talisman.
	xx, err := LoadFile(filepath.Join("..", "..", "packs", "xianxia_sect.json"))
	if err != nil {
		t.Fatalf("load xianxia_sect: %v", err)
	}
	xxWorld, err := xx.Instantiate("")
	if err != nil {
		t.Fatalf("instantiate xianxia_sect: %v", err)
	}
	if _, ok := xxWorld.Catalog.SensoryModes["divine_sense"]; !ok {
		t.Fatalf("xianxia_sect missing divine_sense SensorySpec")
	}
	if _, ok := xxWorld.Catalog.LocomotionModes["sword_flight"]; !ok {
		t.Fatalf("xianxia_sect missing sword_flight LocomotionSpec")
	}
	if _, ok := xxWorld.Catalog.CommChannels["sound_talisman"]; !ok {
		t.Fatalf("xianxia_sect missing sound_talisman CommChannelSpec")
	}
	if _, ok := xxWorld.Catalog.StateSlots["qi"]; !ok {
		t.Fatalf("xianxia_sect missing qi StateSlotSpec")
	}
	if _, ok := xxWorld.Catalog.Forms["foundation_cultivator"]; !ok {
		t.Fatalf("xianxia_sect missing foundation_cultivator FormSpec")
	}
	if len(xxWorld.Catalog.CombatProfiles) == 0 {
		t.Fatalf("xianxia_sect missing CombatProfiles")
	}
	if _, ok := xxWorld.Catalog.ActionSpecs["guardian_array"]; !ok {
		t.Fatalf("xianxia_sect missing guardian_array ActionSpec")
	}
	if _, ok := xxWorld.Catalog.Currencies["sect_merit"]; !ok {
		t.Fatalf("xianxia_sect missing sect_merit CurrencySpec")
	}
	if _, ok := xxWorld.Catalog.Techs["spirit_gathering_array"]; !ok {
		t.Fatalf("xianxia_sect missing spirit_gathering_array TechSpec")
	}
	if cons := sim.InspectWorldConservation(xxWorld); !cons.LedgerConserved || cons.Currencies["sect_merit"] != 120 {
		t.Fatalf("xianxia_sect conservation mismatch: %+v", cons)
	}

	// 3. Wasteland Sector: Orbital DeJure vs Scrap Syndicate DeFacto, satellite_scan blocked by Shielding=80, radio_broadcast.
	ws, err := LoadFile(filepath.Join("..", "..", "packs", "wasteland_sector.json"))
	if err != nil {
		t.Fatalf("load wasteland_sector: %v", err)
	}
	wsWorld, err := ws.Instantiate("")
	if err != nil {
		t.Fatalf("instantiate wasteland_sector: %v", err)
	}
	chain := wsWorld.Polities.EffectivePolityChain("sector_7_scrapyard")
	if len(chain) == 0 || chain[0].ID != "scrap_syndicate" {
		t.Fatalf("expected scrap_syndicate as DeFacto controller of sector_7_scrapyard, got %+v", chain)
	}
	if wsWorld.Polities.PolityGovernsNode("orbital_authority", "sector_7_scrapyard") {
		t.Fatalf("orbital_authority should not govern seceded sector_7_scrapyard effectively")
	}
	if _, ok := wsWorld.Catalog.CommChannels["radio_broadcast"]; !ok {
		t.Fatalf("wasteland_sector missing radio_broadcast CommChannelSpec")
	}
	var scanner *sim.Actor
	for i := range wsWorld.Actors {
		for _, tag := range wsWorld.Actors[i].Tags {
			if tag == "orbital_uplink" {
				scanner = &wsWorld.Actors[i]
				break
			}
		}
		if scanner != nil {
			break
		}
	}
	if scanner == nil {
		t.Fatalf("wasteland_sector missing actor with orbital_uplink tag")
	}
	if mod, ok := wsWorld.CanSenseVenueRemotely(*scanner, "bunker_district", "bunker_core"); ok {
		t.Fatalf("expected satellite_scan into shielded bunker to be blocked by shielding, got ok=true mod=%q", mod)
	}
	if _, ok := wsWorld.Catalog.StateSlots["radiation"]; !ok {
		t.Fatalf("wasteland_sector missing radiation StateSlotSpec")
	}
	if _, ok := wsWorld.Catalog.StateSlots["battery"]; !ok {
		t.Fatalf("wasteland_sector missing battery StateSlotSpec")
	}
	if len(wsWorld.Catalog.CombatProfiles) == 0 || wsWorld.Catalog.CombatProfiles[0].DefenseByType["kinetic"].ImmunityBelowTier < 3 {
		t.Fatalf("wasteland_sector missing mech kinetic immunity CombatProfile: %+v", wsWorld.Catalog.CombatProfiles)
	}
	if _, ok := wsWorld.Catalog.ActionSpecs["emp_pulse"]; !ok {
		t.Fatalf("wasteland_sector missing emp_pulse ActionSpec")
	}
	if _, ok := wsWorld.Catalog.Techs["fusion_microcell"]; !ok {
		t.Fatalf("wasteland_sector missing fusion_microcell TechSpec")
	}
	if wsWorld.FlowGrid == nil || len(wsWorld.FlowGrid.Nodes) < 2 {
		t.Fatalf("wasteland_sector missing FlowGrid")
	}
	if _, err := wsWorld.Step(context.Background(), agent.Offline()); err != nil {
		t.Fatal(err)
	}
	for _, node := range wsWorld.FlowGrid.Nodes {
		if !node.Powered {
			t.Fatalf("expected wasteland_sector FlowGrid node %q to be powered, got %+v", node.ID, node)
		}
	}
	if cons := sim.InspectWorldConservation(wsWorld); !cons.LedgerConserved {
		t.Fatalf("wasteland_sector conservation mismatch: %+v", cons)
	}
}

func TestWorldPackValidateErrors(t *testing.T) {
	base, err := LoadFile(filepath.Join("..", "..", "packs", "mortal_watershed.json"))
	if err != nil {
		t.Fatalf("load base pack: %v", err)
	}
	clonePack := func() *WorldPack {
		raw, _ := json.Marshal(base)
		var out WorldPack
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatalf("clone unmarshal: %v", err)
		}
		return &out
	}

	t.Run("missing id", func(t *testing.T) {
		p := clonePack()
		p.ID = ""
		if err := p.Validate(); err == nil {
			t.Fatalf("expected error for empty id")
		}
	})

	t.Run("invalid venue district", func(t *testing.T) {
		p := clonePack()
		p.Map.Venues[0].DistrictID = "missing_district"
		if err := p.Validate(); err == nil || !strings.Contains(err.Error(), "missing_district") {
			t.Fatalf("expected venue district error, got %v", err)
		}
	})

	t.Run("invalid venue geo_region", func(t *testing.T) {
		p := clonePack()
		p.Map.Venues[0].GeoRegionID = "missing_geo"
		if err := p.Validate(); err == nil || !strings.Contains(err.Error(), "missing_geo") {
			t.Fatalf("expected venue geo_region error, got %v", err)
		}
	})

	t.Run("invalid actor location", func(t *testing.T) {
		p := clonePack()
		p.Actors[0].Location = "nowhere_district"
		if err := p.Validate(); err == nil || !strings.Contains(err.Error(), "nowhere_district") {
			t.Fatalf("expected actor location error, got %v", err)
		}
	})

	t.Run("invalid actor venue", func(t *testing.T) {
		p := clonePack()
		p.Actors[0].VenueID = "nowhere_venue"
		if err := p.Validate(); err == nil || !strings.Contains(err.Error(), "nowhere_venue") {
			t.Fatalf("expected actor venue error, got %v", err)
		}
	})

	t.Run("duplicate actor id", func(t *testing.T) {
		p := clonePack()
		dup := p.Actors[0]
		p.Actors = append(p.Actors, dup)
		if err := p.Validate(); err == nil || !strings.Contains(err.Error(), "duplicate actor") {
			t.Fatalf("expected duplicate actor error, got %v", err)
		}
	})

	t.Run("duplicate venue id", func(t *testing.T) {
		p := clonePack()
		dup := p.Map.Venues[0]
		p.Map.Venues = append(p.Map.Venues, dup)
		if err := p.Validate(); err == nil || !strings.Contains(err.Error(), "duplicate venue") {
			t.Fatalf("expected duplicate venue error, got %v", err)
		}
	})

	t.Run("duplicate polity id", func(t *testing.T) {
		p := clonePack()
		dup := p.Polities.Nodes[0]
		p.Polities.Nodes = append(p.Polities.Nodes, dup)
		if err := p.Validate(); err == nil || !strings.Contains(err.Error(), "duplicate polity") {
			t.Fatalf("expected duplicate polity error, got %v", err)
		}
	})
}
