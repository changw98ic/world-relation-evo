package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/changw98ic/world-relation-evo/internal/modelreply"
	"github.com/changw98ic/world-relation-evo/internal/runtrace"
	"github.com/changw98ic/world-relation-evo/internal/sim"
)

type ContentModel struct {
	Endpoint, APIKey, Name string
	Client                 *http.Client
	Trace                  *runtrace.Writer
}

const prompt = `Write the spoken Chinese dialogue for this character's already-selected speak action and listener. Choose the wording, tone, disclosures and assertions from the speaker's current needs, personality, goal, relationships and remembered experiences in perception, and the actual listener. Identity and stock are circumstances, not a prescribed stance: do not assume a profession always lies, a buyer always exaggerates, or authority always makes someone understate. The character may be candid, uncertain, mistaken, evasive or deceptive when their circumstances support it; no fixed quantity multiplier applies. Return exactly one JSON object. The only top-level field names are "text", "basis_event_id" and "statement". "text" is the spoken dialogue as a string. For example, an ordinary reply is {"text":"让我再想想。"}.
A reply can discuss several things in its text. Choose at most one claim to encode in "statement", a single JSON object. Keep any additional claims in the text only. For example, a speaker who mentions both a stock and a new price could return {"text":"我还有粮。三份收两枚钱，你愿意吗？","statement":{"topic":"trade_negotiation","item":"food","location":"market_street","quantity":3}}.
If citing a memory, put its integer event ID in "basis_event_id" at the top level alongside the single statement. For example, if and only if event 17 in the speaker's citable memories is a production observation or reported production of this item, a possible reply is {"text":"我听说南畴收了两份粮。","basis_event_id":17,"statement":{"topic":"item_produced","item":"food","location":"south_fields","quantity":2}}. Use the current speaker's actual items, locations and citable IDs in the response. Return just the JSON object. Use trade_negotiation for proposed quantities, prices, and counteroffers (preserve the complete terms in the spoken words), stock for an inventory claim, and item_produced for output actually claimed to have been produced. A negotiation or stock memory does not document production. For a cited production claim, cite a production observation or another speaker's production claim for that item. Use an uncited assertion for a speaker's own unsupported claim. When citing, use the character's own memory event_id from interaction.citable_memory_event_ids. Keep hearsay attributed to its speaker. Contracts, completed work, shipments, and payments have separate settled events. Use the scenario's units for quantities.`

func (m ContentModel) Generate(ctx context.Context, p sim.Perception, action sim.Action) (content Content, err error) {
	defer func() {
		err = recordModelError(m.Trace, "content_error", p, m.Endpoint, m.APIKey, err)
	}()
	listener, _ := findNeighbor(p.Nearby, action.TargetID)
	topicItem := PrimaryTopicItem(p.Catalog, p.Self, listener)
	citableIDs := make([]int64, 0, len(p.Self.Memories))
	for _, mem := range p.Self.Memories {
		citableIDs = append(citableIDs, mem.EventID)
	}
	payload := map[string]any{
		"perception":      contextForModel(p),
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
	response, e := m.request(ctx, p, prompt, payload, "content")
	if e != nil {
		return Content{}, e
	}
	a, e := modelreply.Decode(response)
	if e != nil {
		return a, e
	}
	if err := sim.ValidateCitation(p.Self.Memories, a.BasisEventID, a.Statement); err != nil {
		return a, fmt.Errorf("content citation: %w", err)
	}
	return a, nil
}

func (m ContentModel) request(ctx context.Context, p sim.Perception, instructions string, payload any, kind string) (string, error) {
	b, e := json.Marshal(payload)
	if e != nil {
		return "", e
	}
	body, e := json.Marshal(map[string]any{"model": m.Name, "messages": []map[string]string{{"role": "system", "content": instructions}, {"role": "user", "content": string(b)}}, "response_format": map[string]string{"type": "json_object"}})
	if e != nil {
		return "", e
	}
	if e = m.Trace.Record(kind+"_request", p.Tick, p.Self.ID, map[string]any{"model": m.Name, "body": string(body)}); e != nil {
		return "", e
	}
	req, e := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(m.Endpoint, "/")+"/chat/completions", bytes.NewReader(body))
	if e != nil {
		return "", e
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
		return "", e
	}
	defer resp.Body.Close()
	raw, readErr := io.ReadAll(resp.Body)
	if e = m.Trace.Record(kind+"_response", p.Tick, p.Self.ID, map[string]any{"status": resp.StatusCode, "body": string(raw)}); e != nil {
		return "", errors.Join(readErr, e)
	}
	if readErr != nil {
		return "", readErr
	}
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("model HTTP %d: %s", resp.StatusCode, string(raw))
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
		return "", e
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("model returned no choices")
	}
	return out.Choices[0].Message.Content, nil
}

func (m ContentModel) Quote(ctx context.Context, p sim.Perception, draft sim.Action) (terms sim.TradeTerms, err error) {
	defer func() { err = recordModelError(m.Trace, "quote_error", p, m.Endpoint, m.APIKey, err) }()
	instructions := `Draft explicit trade terms for the character's selected proposal. Return exactly one JSON object containing quantity (positive integer), price (nonnegative integer total, not unit price) and currency (accepted item currency, coins if unspecified). Choose freely from the character's circumstances and remembered conversation; no fixed discount, markup or profession-based stance. This is a proposal, not speech and not settlement. Do not change parties, item or original offer ID. The same character will explicitly confirm or decline the concrete proposal afterwards. When this character is the seller, quantity cannot exceed their own held stock. When the character is the buyer making a counteroffer, quantity is a request and the seller's actual stock will be checked before acceptance. Zero total price is valid. Do not add other fields.`
	response, err := m.request(ctx, p, instructions, map[string]any{"perception": contextForModel(p), "selected_draft": draft, "item_spec": p.Catalog.Items[draft.Item]}, "quote")
	if err != nil {
		return terms, err
	}
	var parsed *struct {
		Quantity *int    `json:"quantity"`
		Price    *int    `json:"price"`
		Currency *string `json:"currency"`
	}
	decoder := json.NewDecoder(strings.NewReader(response))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&parsed); err != nil {
		return terms, fmt.Errorf("invalid quote JSON: %w", err)
	}
	var trailing any
	if err = decoder.Decode(&trailing); err != io.EOF {
		return terms, fmt.Errorf("quote must contain exactly one JSON object")
	}
	if parsed == nil || parsed.Quantity == nil || parsed.Price == nil || parsed.Currency == nil {
		return terms, fmt.Errorf("quote requires explicit quantity, total price and currency")
	}
	terms = sim.TradeTerms{Quantity: *parsed.Quantity, Price: *parsed.Price, Currency: *parsed.Currency}
	_, err = sim.QuoteAction(p, draft, terms)
	return terms, err
}
