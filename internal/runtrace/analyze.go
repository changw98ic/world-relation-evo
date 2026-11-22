package runtrace

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/changw98ic/world-relation-evo/internal/sim"
)

// Finding distinguishes an observed contract violation from behavior requiring
// investigation. A completed run and an empty finding list are not a verdict
// about the physical or social validity of the simulation.
type Finding struct {
	Rule     string   `json:"rule"`
	Status   string   `json:"status"`
	Tick     int64    `json:"tick"`
	ActorID  string   `json:"actor_id,omitempty"`
	Evidence []uint64 `json:"evidence"`
	Detail   string   `json:"detail"`
}

type ActorState struct {
	Location  string         `json:"location"`
	Venue     string         `json:"venue,omitempty"`
	House     string         `json:"house,omitempty"`
	Health    int            `json:"health"`
	Energy    int            `json:"energy"`
	Hunger    int            `json:"hunger"`
	Thirst    int            `json:"thirst"`
	Money     int            `json:"money"`
	Items     map[string]int `json:"items"`
	Task      string         `json:"task,omitempty"`
	Remaining int            `json:"remaining,omitempty"`
}

type DecisionReview struct {
	UnavailablePreferences []sim.Action `json:"unavailable_preferences,omitempty"`
	Source                 string       `json:"decision_source,omitempty"`
	Tick                   int64        `json:"tick"`
	ActorID                string       `json:"actor_id"`
	Goal                   string       `json:"goal"`
	InputSeq               uint64       `json:"input_seq"`
	ReadySeq               uint64       `json:"ready_seq,omitempty"`
	OptionID               string       `json:"option_id,omitempty"`
	Action                 sim.Action   `json:"action"`
	Before                 ActorState   `json:"before"`
	After                  *ActorState  `json:"after"`
	Outcomes               []string     `json:"outcomes"`
}

type Report struct {
	WorldID           string           `json:"world_id"`
	RunCompleted      bool             `json:"run_completed"`
	TraceComplete     bool             `json:"trace_complete"`
	TicksSettled      int              `json:"ticks_settled"`
	Decisions         int              `json:"decisions"`
	JevResponses      int              `json:"jev_responses"`
	PersistedTicks    int              `json:"persisted_ticks"`
	RunError          string           `json:"run_error,omitempty"`
	Findings          []Finding        `json:"findings"`
	Timeline          []DecisionReview `json:"timeline"`
	Correctness       string           `json:"correctness"`
	ReviewRequirement string           `json:"review_requirement"`
}

type decisionKey struct {
	tick  int64
	actor string
}

type inputOption struct {
	ID     string     `json:"id"`
	Label  string     `json:"label"`
	Action sim.Action `json:"action"`
}
type inputData struct {
	DecisionSource string         `json:"decision_source"`
	Perception     sim.Perception `json:"perception"`
	Options        []inputOption  `json:"options"`
}

type decisionEvidence struct {
	input      inputData
	row        DecisionReview
	jev        bool
	jevRequest bool
	jevStage   string
	jevChoice  string
}

func actorState(a sim.Actor) ActorState {
	s := ActorState{Location: a.Location, Venue: a.VenueID, House: a.InsideHouse,
		Health: a.Health, Energy: a.Energy, Hunger: a.Hunger, Thirst: a.Thirst,
		Money: a.Money, Items: a.Items}
	if a.Task != nil {
		s.Task, s.Remaining = a.Task.Action.Kind, a.Task.Remaining
	}
	return s
}

func lookupActor(w *sim.World, id string) (sim.Actor, bool) {
	for _, a := range w.Actors {
		if a.ID == id {
			return a, true
		}
	}
	for _, c := range w.Cohorts {
		for _, a := range c.Members {
			if a.ID == id {
				return a, true
			}
		}
	}
	return sim.Actor{}, false
}

func decodeData(e Entry, out any) error {
	if err := json.Unmarshal(e.Data, out); err != nil {
		return fmt.Errorf("trace entry %d (%s): %w", e.Seq, e.Kind, err)
	}
	return nil
}

// Analyze reads original evidence. It does not ask the model to grade itself or
// derive expected effects by calling the settlement code being evaluated.
func Analyze(dir string) (Report, error) {
	report := Report{Findings: []Finding{}, Timeline: []DecisionReview{},
		Correctness:       "not_established",
		ReviewRequirement: "必须结合原始模型返回和人物时间线检查动机、资源来源、空间及因果；流程完成或未检出问题不代表模拟正确。"}
	initial, err := os.ReadFile(filepath.Join(dir, "initial.json"))
	if err != nil {
		return report, fmt.Errorf("read trace initial state: %w", err)
	}
	var before sim.World
	if err := json.Unmarshal(initial, &before); err != nil {
		return report, fmt.Errorf("decode trace initial state: %w", err)
	}
	report.WorldID = before.ID
	f, err := os.Open(filepath.Join(dir, "trace.ndjson"))
	if err != nil {
		return report, err
	}
	defer f.Close()
	decoder := json.NewDecoder(f)
	decisions := map[decisionKey]*decisionEvidence{}
	var seq uint64
	ended := false
	mode := ""
	add := func(e Entry, rule, status, detail string, evidence ...uint64) {
		report.Findings = append(report.Findings, Finding{Rule: rule, Status: status,
			Tick: e.Tick, ActorID: e.ActorID, Evidence: append([]uint64{e.Seq}, evidence...), Detail: detail})
	}
	for {
		var e Entry
		if err := decoder.Decode(&e); err == io.EOF {
			break
		} else if err != nil {
			return report, fmt.Errorf("decode trace after entry %d: %w", seq, err)
		}
		if e.Seq != seq+1 || ended {
			return report, fmt.Errorf("trace sequence invalid at %d after %d (ended=%v)", e.Seq, seq, ended)
		}
		seq = e.Seq
		key := decisionKey{e.Tick, e.ActorID}
		switch e.Kind {
		case "run_start":
			var data struct {
				Decider string `json:"decider"`
			}
			if err := decodeData(e, &data); err != nil {
				return report, err
			}
			mode = data.Decider
		case "run_world", "tick_persisted":
			if e.Kind == "tick_persisted" {
				report.PersistedTicks++
			}
		case "tick_begin":
			var data struct {
				World sim.World `json:"world"`
			}
			if err := decodeData(e, &data); err != nil {
				return report, err
			}
			before = data.World
			if before.Tick != e.Tick {
				add(e, "state_tick", "violation", "开始状态的时步与记录不一致")
			}
		case "decision_input":
			if _, exists := decisions[key]; exists {
				return report, fmt.Errorf("duplicate decision input at tick %d actor %s", e.Tick, e.ActorID)
			}
			d := &decisionEvidence{}
			if err := decodeData(e, &d.input); err != nil {
				return report, err
			}
			p := d.input.Perception
			d.row = DecisionReview{Source: d.input.DecisionSource, Tick: e.Tick, ActorID: e.ActorID, Goal: p.Self.Goal,
				InputSeq: e.Seq, Before: actorState(p.Self), Outcomes: []string{}}
			decisions[key] = d
			if plan := p.ActivePlan; plan != nil && strings.HasPrefix(plan.NextStep, "craft:") {
				recipe := strings.TrimPrefix(plan.NextStep, "craft:")
				offered := false
				for _, option := range d.input.Options {
					if option.Action.Kind == "craft" && option.Action.RecipeID == recipe {
						offered = true
						break
					}
				}
				if !offered {
					add(e, "plan_availability", "violation", fmt.Sprintf("计划要求现在制作 %s，当刻没有该动作", recipe))
				}
			}
			if p.Tick != e.Tick || p.Self.ID != e.ActorID {
				add(e, "perception_identity", "violation", "模型感知与执行人物或时步不一致")
			}
			if a, ok := lookupActor(&before, e.ActorID); !ok {
				add(e, "perception_identity", "violation", "决策人物不在执行前的世界中")
			} else if !reflect.DeepEqual(actorState(a), actorState(p.Self)) {
				add(e, "perception_state", "needs_review", "模型看到的物理状态与步前快照不同，需检查感知构造与阶段顺序")
			}
		case "rule_unavailable":
			d, ok := decisions[key]
			if !ok || d.input.DecisionSource != "rules" {
				return report, fmt.Errorf("rule preferences without rule input at %d", e.Seq)
			}
			var data struct{ Preferences []sim.Action }
			if err := decodeData(e, &data); err != nil {
				return report, err
			}
			d.row.UnavailablePreferences = data.Preferences
		case "decision_selected":
			d, ok := decisions[key]
			if !ok {
				return report, fmt.Errorf("selection without input at %d", e.Seq)
			}
			var data struct {
				OptionID string     `json:"option_id"`
				Action   sim.Action `json:"action"`
			}
			if err := decodeData(e, &data); err != nil {
				return report, err
			}
			d.row.OptionID, d.row.Action = data.OptionID, data.Action
			if mode == "jev" && d.input.DecisionSource != "rules" && (!d.jev || d.jevChoice != data.OptionID) {
				add(e, "model_evidence", "observed_error", "最终动作缺少本轮对应的模型确认凭证", d.row.InputSeq)
			}
			found := false
			for _, option := range d.input.Options {
				if option.ID == data.OptionID && reflect.DeepEqual(option.Action, data.Action) {
					found = true
				}
			}
			if !found {
				add(e, "available_action", "violation", "选择结果不等于当时提供的任一完整候选", d.row.InputSeq)
			}
		case "decision_ready":
			d, ok := decisions[key]
			if !ok {
				return report, fmt.Errorf("ready action without input at %d", e.Seq)
			}
			var data struct {
				Action sim.Action `json:"action"`
			}
			if err := decodeData(e, &data); err != nil {
				return report, err
			}
			if !reflect.DeepEqual(d.row.Action, data.Action) {
				add(e, "content_authority", "violation", "准备执行的动作改变了已选定的系统动作", d.row.InputSeq)
			}
			d.row.Action, d.row.ReadySeq = data.Action, e.Seq
			if mode == "jev" && d.input.DecisionSource != "rules" && !d.jev {
				add(e, "model_evidence", "observed_error", "真实模型模式的已执行决策缺少相应模型原始响应", d.row.InputSeq)
			}
			if err := sim.ValidateCitation(d.input.Perception.Self.Memories, data.Action.BasisEventID, data.Action.Statement); err != nil {
				add(e, "memory_provenance", "violation", err.Error(), d.row.InputSeq)
			}
			if data.Action.Kind == "speak" {
				local := false
				for _, n := range d.input.Perception.Nearby {
					if n.ID == data.Action.TargetID {
						local = true
					}
				}
				if !local {
					add(e, "speech_locality", "violation", "当面讲话的目标不在人物当时可接触的邻居中", d.row.InputSeq)
				}
			}
		case "jev_request":
			d, ok := decisions[key]
			if !ok {
				return report, fmt.Errorf("model request without perception at %d", e.Seq)
			}
			var data struct {
				Stage string `json:"selection_stage"`
			}
			if err := decodeData(e, &data); err != nil {
				return report, err
			}
			d.jevRequest = true
			d.jevStage = data.Stage
		case "jev_response":
			d, ok := decisions[key]
			if !ok {
				return report, fmt.Errorf("model response without perception at %d", e.Seq)
			}
			var data struct {
				Status int    `json:"status"`
				Body   string `json:"body"`
			}
			if err := decodeData(e, &data); err != nil {
				return report, err
			}
			if d.jevRequest && d.jevStage == "action" && data.Status == 200 {
				var response struct {
					Answers map[string]struct {
						Type   string `json:"type"`
						Choice string `json:"choice"`
					} `json:"answers"`
				}
				if err := json.Unmarshal([]byte(data.Body), &response); err == nil && response.Answers["decision"].Type == "choice" {
					d.jev = true
					d.jevChoice = response.Answers["decision"].Choice
				}
			}
			d.jevRequest = false
			report.JevResponses++
			if data.Status != 200 {
				add(e, "provider_response", "observed_error", fmt.Sprintf("外部模型返回HTTP %d；原文保存在该记录中", data.Status))
			}
		case "decision_error", "jev_error", "step_error":
			var data struct {
				Stage string `json:"stage"`
				Error string `json:"error"`
			}
			if err := decodeData(e, &data); err != nil {
				return report, err
			}
			add(e, "execution_error", "observed_error", data.Stage+": "+data.Error)
		case "tick_settled":
			var data struct {
				World     sim.World           `json:"world"`
				Events    []sim.Event         `json:"events"`
				Decisions sim.TickDecisionLog `json:"decisions"`
			}
			if err := decodeData(e, &data); err != nil {
				return report, err
			}
			report.TicksSettled++
			if data.World.Tick != e.Tick+1 || data.Decisions.Tick != e.Tick {
				add(e, "settlement_tick", "violation", "状态推进或决策日志的时步不对应")
			}
			ids := make([]string, 0, len(data.Decisions.Decisions))
			for id := range data.Decisions.Decisions {
				ids = append(ids, id)
			}
			sort.Strings(ids)
			for _, id := range ids {
				action := data.Decisions.Decisions[id]
				d, ok := decisions[decisionKey{e.Tick, id}]
				if !ok {
					add(e, "decision_coverage", "needs_review", "结算含无完整决策trace的人物："+id)
					continue
				}
				if !reflect.DeepEqual(d.row.Action, action) || d.row.ReadySeq == 0 {
					add(e, "settled_decision", "violation", "结算动作与记录的最终选择不一致："+id, d.row.InputSeq)
				}
				if a, ok := lookupActor(&data.World, id); ok {
					state := actorState(a)
					d.row.After = &state
				}
				for _, event := range data.Events {
					if event.ActorID == id {
						d.row.Outcomes = append(d.row.Outcomes, event.Kind)
						if event.Kind == "action_failed" {
							add(Entry{Seq: e.Seq, Tick: e.Tick, ActorID: id}, "candidate_rejected", "needs_review", fmt.Sprintf("候选被执行层拒绝；event=%d payload=%v；需区分资源竞争与候选错误", event.ID, event.Payload), d.row.InputSeq)
						}
					}
				}
			}
			care := make(map[int64]sim.GrowthCareAgreement)
			for _, agreement := range before.Production.CareAgreements {
				care[agreement.GrowthID] = agreement
			}
			for _, event := range data.Events {
				if event.Kind == "growth_care_accepted" && event.Action.Care != nil {
					terms := *event.Action.Care
					if previous, signed := care[terms.GrowthID]; signed {
						add(e, "care_replacement", "violation", fmt.Sprintf("事件 %d 接受旧邀请，替换批次 %d 的已签照料人 %s", event.ID, terms.GrowthID, previous.CaretakerID))
					}
					care[terms.GrowthID] = terms
				}
				if event.Kind == "agreement_proposed" {
					matched := false
					if data.World.Continuity != nil {
						for _, agreement := range data.World.Continuity.Agreements {
							if agreement.ID != event.Action.AgreementID {
								continue
							}
							a := event.Action
							matched = agreement.SupplierID == event.ActorID && agreement.BuyerID == a.TargetID && agreement.Item == a.Item && agreement.Quantity == a.Quantity && agreement.Price == a.Price && (a.DueTick == nil || agreement.DueTick == *a.DueTick)
							break
						}
					}
					if !matched {
						add(e, "agreement_terms", "violation", fmt.Sprintf("事件 %d 中发出的提议与合同 %d 的条款不一致", event.ID, event.Action.AgreementID))
					}
				}
				if event.Kind != "crafted" {
					continue
				}
				quantity, ok := event.Payload["quantity"].(float64)
				if !ok {
					add(Entry{Seq: e.Seq, Tick: e.Tick, ActorID: event.ActorID}, "production_evidence", "observed_error", fmt.Sprintf("完成事件%d缺少可读取的实际产出数量", event.ID))
				} else if quantity == 0 {
					before, beforeOK := event.Payload["yield_credit_before"].(float64)
					after, afterOK := event.Payload["yield_credit_after"].(float64)
					if !(beforeOK && afterOK && after > before) {
						add(Entry{Seq: e.Seq, Tick: e.Tick, ActorID: event.ActorID}, "zero_output", "needs_review", fmt.Sprintf("完成事件%d记录产出为0，且没有把已支付的分数产量记入余量；需对照开工成本、资源扣除和整数舍入，不能以完成事件证明目标有进展", event.ID))
					}
				}
			}
			checkHoldings(e, &data.World, add)
		case "run_end":
			var data struct {
				Completed bool   `json:"completed"`
				Error     string `json:"error"`
			}
			if err := decodeData(e, &data); err != nil {
				return report, err
			}
			report.RunCompleted, report.RunError, ended = data.Completed, data.Error, true
		default:
			if strings.HasPrefix(e.Kind, "ext:") {
				continue
			}
			return report, fmt.Errorf("unknown trace entry kind %q at %d", e.Kind, e.Seq)
		}
	}
	report.TraceComplete = ended
	keys := make([]decisionKey, 0, len(decisions))
	for key := range decisions {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].tick != keys[j].tick {
			return keys[i].tick < keys[j].tick
		}
		return keys[i].actor < keys[j].actor
	})
	for _, key := range keys {
		report.Timeline = append(report.Timeline, decisions[key].row)
	}
	report.Decisions = len(keys)
	if !ended {
		report.Findings = append(report.Findings, Finding{Rule: "trace_completion", Status: "observed_error", Evidence: []uint64{seq}, Detail: "缺少结束记录，不能把截断trace判为运行成功"})
	}
	return report, nil
}

func checkHoldings(e Entry, world *sim.World, add func(Entry, string, string, string, ...uint64)) {
	check := func(a sim.Actor) {
		if a.Money < 0 {
			add(e, "individual_holdings", "violation", fmt.Sprintf("人物%s现金为%d", a.ID, a.Money))
		}
		items := make([]string, 0, len(a.Items))
		for id := range a.Items {
			items = append(items, id)
		}
		sort.Strings(items)
		for _, item := range items {
			if a.Items[item] < 0 {
				add(e, "individual_holdings", "violation", fmt.Sprintf("人物%s的%s库存为%d", a.ID, item, a.Items[item]))
			}
		}
	}
	for _, a := range world.Actors {
		check(a)
	}
	for _, c := range world.Cohorts {
		for _, a := range c.Members {
			check(a)
		}
	}
	if world.PublicTreasury < 0 {
		add(e, "treasury_holdings", "violation", fmt.Sprintf("公共国库为%d", world.PublicTreasury))
	}
}
