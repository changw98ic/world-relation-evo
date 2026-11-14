package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/changw98ic/world-relation-evo/internal/sim"
)

// Jev uses TypeSafe's typed Choice API, not a chat-completions approximation.
// Official contract: https://docs.typesafe.ai/api/system-one
// Choice schema: https://docs.typesafe.ai/primitives/choice
// Endpoint, if set, is the complete System One endpoint URL.
type Jev struct {
	Endpoint, APIKey, Name string
	Client                 *http.Client
}

func (j Jev) Select(ctx context.Context, p sim.Perception, options []Option) (string, error) {
	if len(options) == 0 || len(options) > 255 {
		return "", fmt.Errorf("Jev choice requires 1–255 options; got %d", len(options))
	}
	criteria := make(map[string]string, len(options))
	for _, o := range options {
		if o.ID == "" {
			return "", fmt.Errorf("Jev option has empty ID")
		}
		if _, exists := criteria[o.ID]; exists {
			return "", fmt.Errorf("duplicate Jev option ID %q", o.ID)
		}
		action, err := json.Marshal(o.Action)
		if err != nil {
			return "", err
		}
		criteria[o.ID] = o.Label + "; system-defined action: " + string(action)
	}
	endpoint := j.Endpoint
	if endpoint == "" {
		endpoint = "https://api.typesafe.ai/v1/systemone"
	}
	model := j.Name
	if model == "" {
		model = "jev-latest"
	}
	if j.APIKey == "" {
		return "", fmt.Errorf("Jev API key is required")
	}
	body, err := json.Marshal(map[string]any{
		"model": model, "state": p,
		"questions": map[string]any{"decision": map[string]any{
			"type":         "choice",
			"instructions": "Select the character's next available behavior and interaction target using their identity, current state, goals, relationships and memories. The options are defined by the world system. Select exactly one option; do not invent actions, targets, content or state changes. A heard statement is a claim by its speaker, not necessarily a world fact.",
			"criteria":     criteria,
		}},
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+j.APIKey)
	client := j.Client
	if client == nil {
		client = &http.Client{Timeout: 90 * time.Second}
	}
	response, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("Jev request: %w", err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return "", err
	}
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Jev HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(raw)))
	}
	var result struct {
		Answers map[string]struct {
			Type   string `json:"type"`
			Choice string `json:"choice"`
		} `json:"answers"`
	}
	if err = json.Unmarshal(raw, &result); err != nil {
		return "", fmt.Errorf("decode Jev response: %w", err)
	}
	answer, ok := result.Answers["decision"]
	if !ok || answer.Type != "choice" {
		return "", fmt.Errorf("Jev response missing decision Choice answer")
	}
	if _, ok = criteria[answer.Choice]; !ok {
		return "", fmt.Errorf("Jev returned unavailable option %q", answer.Choice)
	}
	return answer.Choice, nil
}
