package store

import (
	"context"
	"fmt"
	"github.com/changw98ic/world-relation-evo/internal/sim"
	"os"
	"reflect"
	"testing"
	"time"
)

func TestPostgresResumesPersonalContinuity(t *testing.T) {
	dsn := os.Getenv("WRE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set WRE_TEST_DATABASE_URL to run PostgreSQL integration test")
	}
	ctx := context.Background()
	s, err := Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err = s.Init(ctx); err != nil {
		t.Fatal(err)
	}
	id := fmt.Sprintf("continuity-%d", time.Now().UnixNano())
	defer func() {
		s.db.ExecContext(ctx, `DELETE FROM world_events WHERE world_id=$1`, id)
		s.db.ExecContext(ctx, `DELETE FROM worlds WHERE id=$1`, id)
	}()
	w := sim.Village(id)
	w.Actors[0].Plan = &sim.PersonalPlan{RecipeID: "farm_grain", Item: "grain", Quantity: 4, Produced: 2, Stage: "produce", Status: "active", StartedTick: 1}
	w.Actors[0].Memories = make([]sim.Memory, 100)
	w.Actors[0].Memories[0] = sim.Memory{EventID: 1, Kind: "healed", Source: w.Actors[1].ID, TargetID: w.Actors[0].ID, Text: "helped during illness"}
	w.Continuity = &sim.ContinuityState{NextAgreementID: 1, Agreements: []sim.Agreement{{ID: 1, SupplierID: w.Actors[0].ID, BuyerID: w.Actors[1].ID, Item: "grain", Quantity: 4, Price: 3, Status: "active", DueTick: 200}}, DialogueByActor: map[string][]sim.DialogueTurn{w.Actors[0].ID: {{EventID: 1, SpeakerID: w.Actors[0].ID, ListenerID: w.Actors[1].ID, Text: "Tomorrow", Kind: "speech"}}}}
	w.MarketCredit = sim.NewMarketCreditState()
	w.MarketCredit.LoanRequests = []sim.LoanRequest{{ID: 1, BorrowerID: w.Actors[0].ID, LenderID: w.Actors[1].ID, Principal: 12, Status: "pending"}}
	if err = s.Save(ctx, w, nil); err != nil {
		t.Fatal(err)
	}
	got, err := s.Load(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Actors[0].Plan, w.Actors[0].Plan) || !reflect.DeepEqual(got.Actors[0].Memories, w.Actors[0].Memories) || !reflect.DeepEqual(got.Continuity, w.Continuity) || !reflect.DeepEqual(got.MarketCredit.LoanRequests, w.MarketCredit.LoanRequests) {
		t.Fatal("persistent personal state changed on PostgreSQL resume")
	}
}
