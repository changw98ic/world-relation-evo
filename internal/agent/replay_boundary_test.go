package agent

import (
	"context"
	"testing"

	"github.com/changw98ic/world-relation-evo/internal/sim"
)

func TestRecordedPlanningReplaysFromNonzeroTick(t *testing.T) {
	w := boundaryWorld()
	w.Tick = 37
	w.Actors = w.Actors[:2]
	checkpoint := []byte(boundaryJSON(t, w))
	planner := Planner{}
	var logs []sim.TickDecisionLog
	var originalEvents []sim.Event
	for i := 0; i < 2; i++ {
		events, log, err := w.StepRecorded(context.Background(), planner)
		if err != nil {
			t.Fatal(err)
		}
		logs = append(logs, log)
		originalEvents = append(originalEvents, events...)
	}
	if logs[0].Tick != 37 || len(logs[0].Decisions) != 2 {
		t.Fatal("planning was not recorded at the checkpoint tick")
	}
	replayed, events, err := sim.ReplayFromCheckpoint(checkpoint, logs, w.Tick)
	if err != nil {
		t.Fatal(err)
	}
	if boundaryJSON(t, replayed) != boundaryJSON(t, w) || boundaryJSON(t, events) != boundaryJSON(t, originalEvents) {
		t.Fatal("replay changed recorded state, dialogue, or event order")
	}
	if _, _, err := sim.ReplayFromCheckpoint(checkpoint, logs[1:], w.Tick); err == nil {
		t.Fatal("missing first checkpoint decision silently became a successful replay")
	}
}

func TestRecordedAndReplayedDecisionsOwnTheirNestedClaimsAndContractTerms(t *testing.T) {
	view := sim.Perception{Tick: 37, Self: sim.Actor{ID: "alice"}}
	chosen := sim.Action{Kind: "propose_growth_care", TargetID: "bob", Care: &sim.GrowthCareAgreement{GrowthID: 1, OwnerKind: "actor", OwnerID: "alice", CaretakerKind: "actor", CaretakerID: "bob", SharePermille: 500}, Statement: &sim.Statement{Quantity: 3}}
	expected := boundaryJSON(t, chosen)
	recorder := sim.NewRecordingDecider(boundaryDecider(func(context.Context, sim.Perception) (sim.Action, error) { return chosen, nil }))
	returned, err := recorder.Decide(context.Background(), view)
	if err != nil {
		t.Fatal(err)
	}
	returned.Care.SharePermille, returned.Statement.Quantity = 0, 999
	log, ok := recorder.LogForTick(37)
	if !ok || boundaryJSON(t, log.Decisions["alice"]) != expected {
		t.Fatal("changing the settled action rewrote the recorded decision")
	}
	log.Decisions["alice"].Care.SharePermille = 1000
	log.Decisions["alice"].Statement.Quantity = 999
	fresh, ok := recorder.LogForTick(37)
	if !ok || boundaryJSON(t, fresh.Decisions["alice"]) != expected {
		t.Fatal("changing a returned log rewrote recorder-owned contract terms")
	}
	replay := sim.NewReplayDecider([]sim.TickDecisionLog{fresh})
	fresh.Decisions["alice"].Care.SharePermille = 0
	fresh.Decisions["alice"].Statement.Quantity = 999
	first, err := replay.Decide(context.Background(), view)
	if err != nil {
		t.Fatal(err)
	}
	if boundaryJSON(t, first) != expected {
		t.Fatal("changing input logs rewrote replay's recorded contract")
	}
	first.Care.SharePermille, first.Statement.Quantity = 0, 999
	again, err := replay.Decide(context.Background(), view)
	if err != nil {
		t.Fatal(err)
	}
	if boundaryJSON(t, again) != expected {
		t.Fatal("a previous replay caller rewrote the next replayed decision")
	}
}

func TestPlannerReportsCancelledDecision(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (Planner{}).Decide(ctx, boundaryWorld().View("alice")); err != context.Canceled {
		t.Fatalf("expected cancellation, got %v", err)
	}
}
