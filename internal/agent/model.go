package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/changw98ic/world-relation-evo/internal/sim"
	"io"
	"net/http"
	"strings"
	"time"
)

type Model struct {
	Endpoint, APIKey, Name string
	Client                 *http.Client
}

const prompt = `You are one person in a persistent village. Decide ONE action from your own perception, goals, memories and personality. You are not a narrator and cannot create world events. Do not invent knowledge of people outside your perception. Speech is what you choose to tell a listener, not objective truth. Memories with kind speech are statements by their source, not verified facts; other kinds are outcomes you experienced. You may lie or refuse; do not add random distortion. One tick is one hour. Reply with one JSON object matching: {"kind":"wait|move|work|eat|speak|offer|accept|reject","target_id":"person ID","location":"location","text":"speech","food":integer,"price":integer,"offer_id":integer,"basis_event_id":integer,"statement":{"topic":"food_produced","location":"fields","quantity":integer}}. Omit irrelevant fields. Work at fields produces food (farmers 2, others 1). Eat consumes 1 food and reduces hunger. Offer proposes selling your food for coins to a nearby person; the buyer must separately accept before settlement. Accept/reject reference an offer addressed to you. You cannot directly alter another person's state. When acting on a memory or retelling it, set basis_event_id to that memory event_id. Speech may include a structured statement describing what you assert; it is not a fact. Keep its quantity/location consistent with your words, including deliberate exaggeration. Memory observation is a direct experience; memory statement is a received assertion. Never cite unseen event IDs. No prose outside JSON.`

func (m Model) Decide(ctx context.Context, p sim.Perception) (sim.Action, error) {
	b, _ := json.Marshal(p)
	body, _ := json.Marshal(map[string]any{"model": m.Name, "messages": []map[string]string{{"role": "system", "content": prompt}, {"role": "user", "content": string(b)}}, "response_format": map[string]string{"type": "json_object"}})
	req, e := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(m.Endpoint, "/")+"/chat/completions", bytes.NewReader(body))
	if e != nil {
		return sim.Action{}, e
	}
	req.Header.Set("Content-Type", "application/json")
	if m.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+m.APIKey)
	}
	client := m.Client
	if client == nil {
		client = &http.Client{Timeout: 90 * time.Second}
	}
	resp, e := client.Do(req)
	if e != nil {
		return sim.Action{}, e
	}
	defer resp.Body.Close()
	raw, e := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if e != nil {
		return sim.Action{}, e
	}
	if resp.StatusCode != 200 {
		return sim.Action{}, fmt.Errorf("model HTTP %d: %s", resp.StatusCode, string(raw))
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			Finish string `json:"finish_reason"`
		} `json:"choices"`
	}
	if e = json.Unmarshal(raw, &out); e != nil {
		return sim.Action{}, e
	}
	if len(out.Choices) == 0 {
		return sim.Action{}, fmt.Errorf("model returned no choices")
	}
	var a sim.Action
	if e = json.Unmarshal([]byte(out.Choices[0].Message.Content), &a); e != nil {
		return a, fmt.Errorf("invalid action JSON: %w", e)
	}
	return a, nil
}
