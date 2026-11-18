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

	"github.com/changw98ic/world-relation-evo/internal/runtrace"
	"github.com/changw98ic/world-relation-evo/internal/sim"
)

// Jev uses TypeSafe's typed Choice API, not a chat-completions approximation.
// Official contract: https://docs.typesafe.ai/api/system-one
// Choice schema: https://docs.typesafe.ai/primitives/choice
// Endpoint, if set, is the complete System One endpoint URL.
type Jev struct {
	Endpoint, APIKey, Name string
	Client                 *http.Client
	Trace                  *runtrace.Writer
}

// TypeSafe's Choice protocol limit, not a limit on a character's available actions.
// https://docs.typesafe.ai/primitives/choice
const jevChoiceLimit = 255

func (j Jev) Select(ctx context.Context, p sim.Perception, options []Option) (choice string, err error) {
	defer func() {
		err = recordModelError(j.Trace, "jev_error", p, j.Endpoint, j.APIKey, err)
	}()
	if len(options) == 0 {
		return "", fmt.Errorf("Jev choice requires at least one option")
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
	if j.APIKey == "" {
		return "", fmt.Errorf("Jev API key is required")
	}
	p.Self.Plan = sim.ClonePersonalPlan(p.Self.Plan)
	sim.SyncActorPlan(&p.Self, p.Tick, p.Catalog)
	if p.ActivePlan == nil && p.Self.Plan != nil && p.Self.Plan.Status == "active" {
		p.ActivePlan = sim.BuildPlanDirective(p)
	}
	instructions := "Select the character's next behavior and target using their identity tags, age_years, attributes, base and advanced professions, profession levels and rank titles, character level (the sum of all profession levels), guilds and guild levels, role, goal, self.plan, active_plan, clock time, calendar (year, season, day), district_state (weather, regenerating resource pools, sanitation, active_disasters, threat_activity, public infrastructure), energy, sleep_trait, hunger, thirst, waste, disease_id, items and their tiers, house asset levels, accessible_houses, connected roads or venue_routes, relationships, and memories. Each tick is 15 minutes. Leaving a house steps onto its district street; road and venue_route options connect adjacent places (check leads_toward and weather-adjusted energy costs). Indoor facilities and outdoor district/venue features unlock profession craft recipes; higher profession levels, advanced career promotions, seasonal yield modifiers, and resource pool reserves govern output yield. When self.plan is active (status == \"active\"), actively drive its inventory-coupled stage after urgent survival (drink, eat, relieve, medicate, sleep/safety) and open bilateral responses (accept/reject, accept_agreement/reject_agreement, fulfill_agreement): in stage \"produce\" (stock is insufficient for the next catalog trade lot), follow active_plan.next_step and the matching available action. Collect ready output, acquire missing supplies, or travel to work as indicated. If production is already underway or blocked, attend to care or other available work; in stage \"sell\" (stock covers the next catalog trade lot, even when the seasonal sales target is larger), fulfill open delivery agreements, offer or propose_agreement for plan.item to nearby buyers, or step outside and travel toward a market or buyer until remaining_to_sell reaches 0; only choose abandon_plan if blocked or deliberately changing livelihood. Join a guild, learn a base profession, or advance into a higher-tier profession only at that guild's hall district when your attribute panel and parent profession level meet the listed minimums. Drink water when thirsty, eat when hungry, use sanitation facilities (relieve) when waste accumulates, treat wounds or illness with medicinal items, maintain public infrastructure (maintain_infra) when degraded, hunt regional threats (hunt_threat) when capable and healthy, and perform rituals (perform_ritual) at shrines during seasonal rites or disasters. Keep enough energy (>= 15) to return home, and sleep indoors during your sleep window rather than rough sleeping outdoors. Never transfer_house ownership away unless intentionally gifting surplus property. Select exactly one system-defined option; a heard statement is a speaker's assertion, not guaranteed objective fact."
	if ap := p.ActivePlan; ap != nil {
		instructions += fmt.Sprintf(" Active plan directive: stage=%s item=%s recipe=%s target=%d sold=%d remaining_to_sell=%d held=%d deficit_to_produce=%d next_step=%s.",
			ap.Stage, ap.Item, ap.RecipeID, ap.TargetQuantity, ap.SoldQuantity, ap.RemainingToSell, ap.HeldQuantity, ap.DeficitToProduce, ap.NextStep)
	}
	remaining := options
	for len(remaining) > jevChoiceLimit {
		var groups [][]Option
		kinds := map[string]int{}
		for _, option := range remaining {
			index, exists := kinds[option.Action.Kind]
			if !exists {
				index = len(groups)
				kinds[option.Action.Kind] = index
				groups = append(groups, nil)
			}
			groups[index] = append(groups[index], option)
		}
		if len(groups) == 1 || len(groups) > jevChoiceLimit {
			// Balanced ranges also support large catalogs of a single action kind.
			// Each level shrinks the selected set; every original option remains reachable.
			count := min(jevChoiceLimit, (len(remaining)-1)/jevChoiceLimit+1)
			size := (len(remaining)-1)/count + 1
			groups = nil
			for start := 0; start < len(remaining); start += size {
				groups = append(groups, remaining[start:min(start+size, len(remaining))])
			}
		}
		groupCriteria := make(map[string]string, len(groups))
		members := make(map[string][]Option, len(groups))
		for index, group := range groups {
			id := fmt.Sprintf("group-%03d", index)
			var description strings.Builder
			fmt.Fprintf(&description, "Group of %d available actions:\n", len(group))
			for _, option := range group {
				fmt.Fprintf(&description, "%s: %s\n", option.ID, criteria[option.ID])
			}
			groupCriteria[id], members[id] = description.String(), group
		}
		selected, err := j.selectChoice(ctx, p, groupCriteria, instructions+" This is a grouping step: choose the group containing the best next action. All members are listed. The group itself is never executed; a following question selects its concrete action.", "group")
		if err != nil {
			return "", err
		}
		remaining = members[selected]
	}
	leafCriteria := make(map[string]string, len(remaining))
	for _, option := range remaining {
		leafCriteria[option.ID] = criteria[option.ID]
	}
	return j.selectChoice(ctx, p, leafCriteria, instructions, "action")
}

func (j Jev) selectChoice(ctx context.Context, p sim.Perception, criteria map[string]string, instructions, stage string) (string, error) {
	endpoint := j.Endpoint
	if endpoint == "" {
		endpoint = "https://api.typesafe.ai/v1/systemone"
	}
	model := j.Name
	if model == "" {
		model = "jev-latest"
	}
	body, err := json.Marshal(map[string]any{
		"model": model, "state": contextForModel(p),
		"questions": map[string]any{"decision": map[string]any{
			"type":         "choice",
			"instructions": instructions,
			"criteria":     criteria,
		}},
	})
	if err != nil {
		return "", err
	}
	if err = j.Trace.Record("jev_request", p.Tick, p.Self.ID, map[string]any{"model": model, "body": string(body), "selection_stage": stage}); err != nil {
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
	raw, readErr := io.ReadAll(response.Body)
	if err = j.Trace.Record("jev_response", p.Tick, p.Self.ID, map[string]any{"status": response.StatusCode, "body": string(raw)}); err != nil {
		return "", errors.Join(readErr, err)
	}
	if readErr != nil {
		return "", readErr
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
