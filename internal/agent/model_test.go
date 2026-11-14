package agent

import (
	"context"
	"encoding/json"
	"github.com/changw98ic/world-relation-evo/internal/sim"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestModelActionAdapter(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Error(r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": `{"kind":"speak","target_id":"p02","text":"Hello"}`}}}})
	}))
	defer s.Close()
	m := Model{Endpoint: s.URL + "/v1", Name: "test"}
	a, e := m.Decide(context.Background(), sim.Village("test").View("p01"))
	if e != nil || a.Kind != "speak" {
		t.Fatalf("%+v %v", a, e)
	}
}
