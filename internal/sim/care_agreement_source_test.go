package sim

import (
	"encoding/json"
	"math"
	"testing"
)

func TestSignedAgreementSuppliesBothPerceptionAndPhysicalCareAfterRestore(t *testing.T) {
	w := lawWorld()
	r := lawCrop()
	w.Catalog.Recipes = []Recipe{r}
	w.Actors[0].Location = "away"
	w.Actors[1].Items["water"] = 1
	w.Growth = []GrowthBatch{{ID: 9, OwnerID: "a", District: "home", Item: "grain", Quantity: 3, Batches: 1, RecipeID: r.ID, Rule: *r.Growth}}
	terms := GrowthCareAgreement{GrowthID: 9, OwnerKind: "actor", OwnerID: "a", CaretakerKind: "actor", CaretakerID: "b", SupplierKind: "actor", SupplierID: "b", Harvest: true, OwnerAccepted: true, CaretakerAccepted: true, SharePermille: 333}
	if err := w.SetGrowthCare(terms); err != nil {
		t.Fatal(err)
	}
	// A snapshot from before contracts became authoritative could contain a stale
	// caretaker on the batch. The signed terms identify who pays and does the work.
	var archive map[string]json.RawMessage
	if err := json.Unmarshal(lawJSON(t, w), &archive); err != nil {
		t.Fatal(err)
	}
	var growing []map[string]json.RawMessage
	if err := json.Unmarshal(archive["growth"], &growing); err != nil {
		t.Fatal(err)
	}
	growing[0]["caretaker_id"] = json.RawMessage(`"c"`)
	archive["growth"] = lawJSON(t, growing)
	var restored World
	if err := json.Unmarshal(lawJSON(t, archive), &restored); err != nil {
		t.Fatal(err)
	}
	view := restored.View("b")
	if len(view.Growth) != 1 || len(view.CareAgreements) != 1 || view.CareAgreements[0] != terms || len(restored.View("c").Growth) != 0 {
		t.Fatal("view did not reflect only the signed care agreement")
	}
	restored.Tick = 1
	restored.AdvanceGrowth()
	if restored.Growth[0].ProgressTicks != 1 || restored.Actors[1].Items["water"] != 0 {
		t.Fatal("wrong holder paid for care after loading the snapshot")
	}
}

func TestHarvestShareRetainsFullIntegerRange(t *testing.T) {
	a := GrowthCareAgreement{SharePermille: 1000}
	if a.CaretakerShare(math.MaxInt64) != math.MaxInt64 {
		t.Fatal("full harvest lost units")
	}
	a.SharePermille = 500
	if a.CaretakerShare(math.MaxInt64) != math.MaxInt64/2 {
		t.Fatal("partial share lost units")
	}
}

func TestCheckpointRejectsConflictingSignedCaretakers(t *testing.T) {
	w := lawWorld()
	terms := GrowthCareAgreement{GrowthID: 9, OwnerID: "a", OwnerKind: "actor", CaretakerKind: "actor", CaretakerID: "b", SupplierKind: "actor", SupplierID: "b", OwnerAccepted: true, CaretakerAccepted: true, Harvest: true, SharePermille: 1000}
	w.Production.CareAgreements = []GrowthCareAgreement{terms}
	if _, _, err := ReplayFromCheckpoint(lawJSON(t, w), nil, w.Tick); err != nil {
		t.Fatalf("single signed full share: %v", err)
	}
	terms.CaretakerID, terms.SupplierID = "c", "c"
	w.Production.CareAgreements = append(w.Production.CareAgreements, terms)
	if _, _, err := ReplayFromCheckpoint(lawJSON(t, w), nil, w.Tick); err == nil {
		t.Fatal("duplicate signed agreements selected an arbitrary caretaker")
	}
}
