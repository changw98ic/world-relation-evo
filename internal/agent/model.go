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

type ContentModel struct {
	Endpoint, APIKey, Name string
	Client                 *http.Client
}

const prompt = `Write the concrete content for this character's already-selected action and interlocutor. You do NOT choose actions, targets, quantities, prices, routes, damage, healing or state changes. Output only JSON {"text":"words spoken or action description","basis_event_id":integer,"statement":{"topic":"food_produced","location":"fields","quantity":integer}}. Omit statement and basis if not relevant. A statement is a character assertion, never an objective fact or resource update. Cite only a memory event_id visible to this character when retelling it. Speech memories are claims by their source; observations are direct experiences. You may word or deliberately distort a retelling according to character identity, motivation and personality; do not apply automatic random distortion. Keep structured assertions consistent with the words you generate. Do not add behavior, target or state fields.`

func (m ContentModel) Generate(ctx context.Context, p sim.Perception, action sim.Action) (Content, error) {
	b, _ := json.Marshal(map[string]any{"perception": p, "selected_action": action})
	body, _ := json.Marshal(map[string]any{"model": m.Name, "messages": []map[string]string{{"role": "system", "content": prompt}, {"role": "user", "content": string(b)}}, "response_format": map[string]string{"type": "json_object"}})
	req, e := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(m.Endpoint, "/")+"/chat/completions", bytes.NewReader(body))
	if e != nil {
		return Content{}, e
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
		return Content{}, e
	}
	defer resp.Body.Close()
	raw, e := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if e != nil {
		return Content{}, e
	}
	if resp.StatusCode != 200 {
		return Content{}, fmt.Errorf("model HTTP %d: %s", resp.StatusCode, string(raw))
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
		return Content{}, e
	}
	if len(out.Choices) == 0 {
		return Content{}, fmt.Errorf("model returned no choices")
	}
	var a Content
	decoder := json.NewDecoder(strings.NewReader(out.Choices[0].Message.Content))
	decoder.DisallowUnknownFields()
	if e = decoder.Decode(&a); e != nil {
		return a, fmt.Errorf("invalid content JSON: %w", e)
	}
	return a, nil
}
