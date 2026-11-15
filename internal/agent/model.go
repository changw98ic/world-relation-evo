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

const prompt = `Write the spoken Chinese dialogue for this character's already-selected speak action and listener. Choose the wording, tone, disclosures and assertions from the speaker's current needs, personality, goal, relationships and remembered experiences in perception, and the actual listener. Identity and stock are circumstances, not a prescribed stance: do not assume a profession always lies, a buyer always exaggerates, or authority always makes someone understate. The character may be candid, uncertain, mistaken, evasive or deceptive when their circumstances support it; no fixed quantity multiplier applies. You generate speech, not actions or state changes. Output only JSON {"text":"words spoken to the listener","basis_event_id":integer,"statement":{"topic":"item_produced","item":"item_id","location":"district_id","quantity":integer}}. Omit statement and basis_event_id if not reporting or retelling item production/supply news. A statement is a character assertion, never an objective fact or inventory update; statement.quantity is the claimed quantity, not an action quantity. When citing a memory, set basis_event_id ONLY to an integer present in interaction.citable_memory_event_ids (the memory's own event_id, not its root_event_id or basis_event_id). Preserve the distinction between witnessed experiences and things heard from others. Keep statement.quantity consistent with the number spoken in text. Do not add behavior, target or state fields.`

func (m ContentModel) Generate(ctx context.Context, p sim.Perception, action sim.Action) (Content, error) {
	listener, _ := findNeighbor(p.Nearby, action.TargetID)
	topicItem := PrimaryTopicItem(p.Catalog, p.Self, listener)
	citableIDs := make([]int64, 0, len(p.Self.Memories))
	for _, mem := range p.Self.Memories {
		citableIDs = append(citableIDs, mem.EventID)
	}
	payload := map[string]any{
		"perception":      p,
		"selected_action": action,
		"interaction": map[string]any{
			"speaker": map[string]any{
				"id":               p.Self.ID,
				"name":             p.Self.Name,
				"role":             p.Self.Role,
				"tags":             p.Self.Tags,
				"attributes":       p.Self.Attributes,
				"professions":      p.Self.Professions,
				"profession_ranks": p.Self.ProfessionRanks,
				"level":            p.Self.Level,
				"guilds":           p.Self.Guilds,
				"location":         p.Self.Location,
				"inside_house":     p.Self.InsideHouse,
				"energy":           p.Self.Energy,
				"sleep_trait":      p.Self.SleepTrait,
				"items":            p.Self.Items,
				"personality":      p.Self.Personality,
				"goal":             p.Self.Goal,
			},
			"listener":                  listener,
			"speaker_trust_to_listener": p.Self.Trust[action.TargetID],
			"citable_memory_event_ids":  citableIDs,
			"primary_topic_item":        topicItem,
		},
	}
	b, _ := json.Marshal(payload)
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
	contentText := strings.TrimSpace(out.Choices[0].Message.Content)
	if strings.HasPrefix(contentText, "```") {
		if idx := strings.Index(contentText, "\n"); idx >= 0 {
			contentText = contentText[idx+1:]
		}
		contentText = strings.TrimSuffix(strings.TrimSpace(contentText), "```")
		contentText = strings.TrimSpace(contentText)
	}
	var a Content
	decoder := json.NewDecoder(strings.NewReader(contentText))
	decoder.DisallowUnknownFields()
	if e = decoder.Decode(&a); e != nil {
		return a, fmt.Errorf("invalid content JSON: %w", e)
	}
	if a.BasisEventID != 0 {
		validBasis := false
		for _, id := range citableIDs {
			if id == a.BasisEventID {
				validBasis = true
				break
			}
		}
		if !validBasis {
			a.BasisEventID = 0
		}
	}
	return a, nil
}
