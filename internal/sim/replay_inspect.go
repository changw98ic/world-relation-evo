package sim

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
)

// TickDecisionLog persists the exact settled Action chosen for each idle actor at a given simulation tick
// so checkpoint replay can reproduce runs deterministically without invoking live Jev/LLM models.
type TickDecisionLog struct {
	Tick      int64             `json:"tick"`
	Decisions map[string]Action `json:"decisions"`
}

func cloneAction(x Action) Action {
	out := x
	if x.Statement != nil {
		st := *x.Statement
		out.Statement = &st
	}
	return out
}

// RecordingDecider wraps any live Decider (such as agent.Pipeline with Jev + ContentModel) and records
// every settled Action per (Tick, ActorID) in a thread-safe manner.
type RecordingDecider struct {
	Inner Decider

	mu     sync.Mutex
	byTick map[int64]map[string]Action
}

// NewRecordingDecider creates a RecordingDecider wrapping inner.
func NewRecordingDecider(inner Decider) *RecordingDecider {
	return &RecordingDecider{
		Inner:  inner,
		byTick: make(map[int64]map[string]Action),
	}
}

func (r *RecordingDecider) Decide(ctx context.Context, p Perception) (Action, error) {
	if r == nil || r.Inner == nil {
		return Action{}, fmt.Errorf("recording decider has nil inner decider")
	}
	act, err := r.Inner.Decide(ctx, p)
	if err != nil {
		return Action{}, err
	}
	r.mu.Lock()
	if r.byTick == nil {
		r.byTick = make(map[int64]map[string]Action)
	}
	m := r.byTick[p.Tick]
	if m == nil {
		m = make(map[string]Action)
		r.byTick[p.Tick] = m
	}
	m[p.Self.ID] = cloneAction(act)
	r.mu.Unlock()
	return act, nil
}

// LogForTick returns the recorded TickDecisionLog for tick, if any decisions were recorded.
func (r *RecordingDecider) LogForTick(tick int64) (TickDecisionLog, bool) {
	if r == nil {
		return TickDecisionLog{}, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	src, ok := r.byTick[tick]
	if !ok {
		return TickDecisionLog{}, false
	}
	cp := make(map[string]Action, len(src))
	for k, v := range src {
		cp[k] = cloneAction(v)
	}
	return TickDecisionLog{Tick: tick, Decisions: cp}, true
}

// Logs returns all recorded TickDecisionLogs sorted in ascending tick order.
func (r *RecordingDecider) Logs() []TickDecisionLog {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	ticks := make([]int64, 0, len(r.byTick))
	for t := range r.byTick {
		ticks = append(ticks, t)
	}
	sort.Slice(ticks, func(i, j int) bool { return ticks[i] < ticks[j] })
	out := make([]TickDecisionLog, 0, len(ticks))
	for _, t := range ticks {
		src := r.byTick[t]
		cp := make(map[string]Action, len(src))
		for k, v := range src {
			cp[k] = cloneAction(v)
		}
		out = append(out, TickDecisionLog{Tick: t, Decisions: cp})
	}
	return out
}

// StepRecorded advances the world by one tick using d while recording every idle actor's
// resolved Action into the returned TickDecisionLog.
func (w *World) StepRecorded(ctx context.Context, d Decider) ([]Event, TickDecisionLog, error) {
	if w == nil {
		return nil, TickDecisionLog{}, fmt.Errorf("world is nil")
	}
	startTick := w.Tick
	rec := NewRecordingDecider(d)
	events, err := w.Step(ctx, rec)
	if err != nil {
		return nil, TickDecisionLog{}, err
	}
	log, ok := rec.LogForTick(startTick)
	if !ok {
		log = TickDecisionLog{Tick: startTick, Decisions: map[string]Action{}}
	}
	return events, log, nil
}

// ReplayDecider feeds persisted TickDecisionLog actions back into World.Step without ever
// calling a live selector or language model.
type ReplayDecider struct {
	byTick map[int64]map[string]Action
}

// NewReplayDecider indexes logs by tick and actor ID for deterministic replay.
func NewReplayDecider(logs []TickDecisionLog) *ReplayDecider {
	byTick := make(map[int64]map[string]Action, len(logs))
	for _, l := range logs {
		cp := make(map[string]Action, len(l.Decisions))
		for actorID, act := range l.Decisions {
			cp[actorID] = cloneAction(act)
		}
		byTick[l.Tick] = cp
	}
	return &ReplayDecider{byTick: byTick}
}

func (r *ReplayDecider) Decide(_ context.Context, p Perception) (Action, error) {
	if r == nil {
		return Action{}, fmt.Errorf("replay decider is nil")
	}
	tickMap, ok := r.byTick[p.Tick]
	if !ok {
		return Action{}, fmt.Errorf("replay missing decision log for tick %d (actor %s)", p.Tick, p.Self.ID)
	}
	act, ok := tickMap[p.Self.ID]
	if !ok {
		return Action{}, fmt.Errorf("replay missing recorded decision for actor %s at tick %d", p.Self.ID, p.Tick)
	}
	return cloneAction(act), nil
}

// ReplayFromCheckpoint restores a World from checkpointJSON and steps deterministically forward
// to targetTick using only the recorded decisions in logs (zero live model calls).
func ReplayFromCheckpoint(checkpointJSON []byte, logs []TickDecisionLog, targetTick int64) (*World, []Event, error) {
	if len(checkpointJSON) == 0 {
		return nil, nil, fmt.Errorf("checkpoint JSON is empty")
	}
	var w World
	if err := json.Unmarshal(checkpointJSON, &w); err != nil {
		return nil, nil, fmt.Errorf("decode checkpoint: %w", err)
	}
	if targetTick < w.Tick {
		return nil, nil, fmt.Errorf("target tick %d precedes checkpoint tick %d", targetTick, w.Tick)
	}
	rd := NewReplayDecider(logs)
	var allEvents []Event
	ctx := context.Background()
	for w.Tick < targetTick {
		evs, err := w.Step(ctx, rd)
		if err != nil {
			return nil, nil, err
		}
		allEvents = append(allEvents, evs...)
	}
	return &w, allEvents, nil
}

// MemoryInspection exposes an actor memory with explicit epistemic separation between
// first-hand Observation and second-hand/channel Statement.
type MemoryInspection struct {
	EventID        int64        `json:"event_id"`
	Tick           int64        `json:"tick"`
	Kind           string       `json:"kind"`
	Source         string       `json:"source"`
	SourceRole     string       `json:"source_role,omitempty"`
	TargetID       string       `json:"target_id,omitempty"`
	BasisEventID   int64        `json:"basis_event_id,omitempty"`
	RootEventID    int64        `json:"root_event_id"`
	ChannelID      string       `json:"channel_id,omitempty"`
	HasObservation bool         `json:"has_observation"`
	Observation    *Observation `json:"observation,omitempty"`
	HasStatement   bool         `json:"has_statement"`
	Statement      *Statement   `json:"statement,omitempty"`
	Text           string       `json:"text"`
}

// ActorInspection is a structured backend report of an actor's full profile, placement, inventory, and epistemic memories.
type ActorInspection struct {
	ID               string             `json:"id"`
	Name             string             `json:"name"`
	Role             string             `json:"role"`
	Tags             []string           `json:"tags"`
	Level            int                `json:"level"`
	Attributes       Attributes         `json:"attributes"`
	Professions      map[string]int     `json:"professions"`
	ProfessionRanks  map[string]string  `json:"profession_ranks,omitempty"`
	Guilds           []string           `json:"guilds,omitempty"`
	Health           int                `json:"health"`
	Energy           int                `json:"energy"`
	Hunger           int                `json:"hunger"`
	Thirst           int                `json:"thirst,omitempty"`
	Money            int                `json:"money"`
	Currencies       map[string]int64   `json:"currencies,omitempty"`
	FormID           string             `json:"form_id,omitempty"`
	StateSlots       map[string]int     `json:"state_slots,omitempty"`
	KnownTechs       []string           `json:"known_techs,omitempty"`
	Dead             bool               `json:"dead,omitempty"`
	Jailed           bool               `json:"jailed,omitempty"`
	Location         string             `json:"location"`
	DistrictName     string             `json:"district_name,omitempty"`
	VenueID          string             `json:"venue_id,omitempty"`
	VenueName        string             `json:"venue_name,omitempty"`
	InsideHouse      string             `json:"inside_house,omitempty"`
	HomeID           string             `json:"home_id,omitempty"`
	GeoRegionID      string             `json:"geo_region_id,omitempty"`
	GeoRegionName    string             `json:"geo_region_name,omitempty"`
	PolityID         string             `json:"polity_id,omitempty"`
	PolityName       string             `json:"polity_name,omitempty"`
	EffectiveRulerID string             `json:"effective_ruler_id,omitempty"`
	Inventory        map[string]int     `json:"inventory"`
	ActiveTask       *Task              `json:"active_task,omitempty"`
	Memories         []MemoryInspection `json:"memories,omitempty"`
}

// InspectActor returns a structured backend inspection report for actorID in w.
func InspectActor(w *World, actorID string) (*ActorInspection, error) {
	if w == nil {
		return nil, fmt.Errorf("world is nil")
	}
	a := w.actor(actorID)
	if a == nil {
		return nil, fmt.Errorf("unknown actor: %s", actorID)
	}
	distName := ""
	if d, ok := w.Map.DistrictByID(a.Location); ok {
		distName = d.Name
	}
	venueName := ""
	if a.VenueID != "" {
		if v, ok := w.Map.VenueByID(a.VenueID); ok {
			venueName = v.Name
		}
	}
	regID := w.Map.GeoRegionIDForLocation(a.Location, a.VenueID)
	regName := ""
	if regID != "" {
		if reg, ok := w.Map.GeoRegionByID(regID); ok {
			regName = reg.Name
		}
	}
	polityID := w.Map.PolityIDForLocation(a.Location, a.VenueID)
	polityName := ""
	rulerID := ""
	if w.Polities != nil && polityID != "" {
		if node := w.Polities.NodeByID(polityID); node != nil {
			polityName = node.Name
		}
		if chain := w.Polities.EffectivePolityChain(polityID); len(chain) > 0 {
			rulerID = chain[len(chain)-1].ID
		}
	}
	mems := make([]MemoryInspection, 0, len(a.Memories))
	for _, m := range a.Memories {
		mi := MemoryInspection{
			EventID:        m.EventID,
			Tick:           m.Tick,
			Kind:           m.Kind,
			Source:         m.Source,
			SourceRole:     m.SourceRole,
			TargetID:       m.TargetID,
			BasisEventID:   m.BasisEventID,
			RootEventID:    m.RootEventID,
			ChannelID:      m.Channel,
			HasObservation: m.Observation != nil,
			HasStatement:   m.Statement != nil,
			Text:           m.Text,
		}
		if m.Observation != nil {
			oc := *m.Observation
			mi.Observation = &oc
		}
		if m.Statement != nil {
			sc := *m.Statement
			mi.Statement = &sc
		}
		mems = append(mems, mi)
	}
	var taskCopy *Task
	if a.Task != nil {
		tc := *a.Task
		tc.Action = cloneAction(tc.Action)
		taskCopy = &tc
	}
	return &ActorInspection{
		ID:               a.ID,
		Name:             a.Name,
		Role:             a.Role,
		Tags:             append([]string(nil), a.Tags...),
		Level:            a.Level,
		Attributes:       a.Attributes,
		Professions:      cloneIntMap(a.Professions),
		ProfessionRanks:  cloneStringMap(a.ProfessionRanks),
		Guilds:           append([]string(nil), a.Guilds...),
		Health:           a.Health,
		Energy:           a.Energy,
		Hunger:           a.Hunger,
		Thirst:           a.Thirst,
		Money:            a.Money,
		Currencies:       cloneInt64Map(a.Currencies),
		FormID:           a.FormID,
		StateSlots:       cloneIntMap(a.StateSlots),
		KnownTechs:       w.ActorKnownTechs(a.ID),
		Dead:             a.Dead,
		Jailed:           w.Tick < a.JailedUntil,
		Location:         a.Location,
		DistrictName:     distName,
		VenueID:          a.VenueID,
		VenueName:        venueName,
		InsideHouse:      a.InsideHouse,
		HomeID:           a.HomeID,
		GeoRegionID:      regID,
		GeoRegionName:    regName,
		PolityID:         polityID,
		PolityName:       polityName,
		EffectiveRulerID: rulerID,
		Inventory:        cloneIntMap(a.Items),
		ActiveTask:       taskCopy,
		Memories:         mems,
	}, nil
}

// LineageNode summarizes one event node in an event's causal ancestry or propagation tree.
type LineageNode struct {
	EventID      int64      `json:"event_id"`
	Tick         int64      `json:"tick"`
	ActorID      string     `json:"actor_id"`
	TargetID     string     `json:"target_id,omitempty"`
	Kind         string     `json:"kind"`
	ActionKind   string     `json:"action_kind"`
	ChannelID    string     `json:"channel_id,omitempty"`
	BasisEventID int64      `json:"basis_event_id,omitempty"`
	RootEventID  int64      `json:"root_event_id"`
	HasStatement bool       `json:"has_statement,omitempty"`
	Statement    *Statement `json:"statement,omitempty"`
	Text         string     `json:"text,omitempty"`
}

// LineageTreeNode represents a recursive downstream causal propagation branch.
type LineageTreeNode struct {
	Node     LineageNode        `json:"node"`
	Children []*LineageTreeNode `json:"children,omitempty"`
}

// LineageInspection traces the full upstream chain (RootEventID -> ... -> BasisEventID -> eventID)
// and the downstream propagation tree across events.
type LineageInspection struct {
	TargetEventID  int64            `json:"target_event_id"`
	RootEventID    int64            `json:"root_event_id"`
	UpstreamChain  []LineageNode    `json:"upstream_chain"`
	DownstreamTree *LineageTreeNode `json:"downstream_tree"`
	RootTree       *LineageTreeNode `json:"root_tree,omitempty"`
	DescendantIDs  []int64          `json:"descendant_ids,omitempty"`
}

func eventLineageNode(e Event) LineageNode {
	basis := e.Action.BasisEventID
	if basis == 0 {
		basis = payloadInt64(e.Payload, "basis_event_id")
	}
	root := payloadInt64(e.Payload, "root_event_id")
	if root == 0 && basis == 0 {
		root = e.ID
	}
	ch := e.Action.Channel
	if ch == "" && e.Payload != nil {
		if s, ok := e.Payload["channel_id"].(string); ok {
			ch = s
		}
	}
	text := ""
	if e.Payload != nil {
		if s, ok := e.Payload["text"].(string); ok {
			text = s
		}
	}
	if text == "" {
		text = e.Action.Text
	}
	var stmt *Statement
	if e.Action.Statement != nil {
		sc := *e.Action.Statement
		stmt = &sc
	}
	return LineageNode{
		EventID:      e.ID,
		Tick:         e.Tick,
		ActorID:      e.ActorID,
		TargetID:     e.TargetID,
		Kind:         e.Kind,
		ActionKind:   e.Action.Kind,
		ChannelID:    ch,
		BasisEventID: basis,
		RootEventID:  root,
		HasStatement: stmt != nil,
		Statement:    stmt,
		Text:         text,
	}
}

// InspectLineage traces the full causal ancestry (RootEventID -> ... -> BasisEventID -> eventID)
// and downstream propagation tree for eventID across events.
func InspectLineage(events []Event, eventID int64) (*LineageInspection, error) {
	byID := make(map[int64]LineageNode, len(events))
	orderedIDs := make([]int64, 0, len(events))
	for _, e := range events {
		if _, exists := byID[e.ID]; !exists {
			orderedIDs = append(orderedIDs, e.ID)
		}
		byID[e.ID] = eventLineageNode(e)
	}
	target, ok := byID[eventID]
	if !ok {
		return nil, fmt.Errorf("event %d not found", eventID)
	}
	sort.Slice(orderedIDs, func(i, j int) bool { return orderedIDs[i] < orderedIDs[j] })

	// Walk upstream along BasisEventID links.
	var reversed []LineageNode
	seen := make(map[int64]bool)
	curID := eventID
	for curID != 0 && !seen[curID] {
		seen[curID] = true
		node, exists := byID[curID]
		if !exists {
			break
		}
		reversed = append(reversed, node)
		curID = node.BasisEventID
	}
	rootID := target.RootEventID
	if rootID == 0 {
		rootID = reversed[len(reversed)-1].RootEventID
		if rootID == 0 {
			rootID = reversed[len(reversed)-1].EventID
		}
	}
	// If the root event is present in events and wasn't reached due to compacted intermediate hops, prepend it.
	if rootID != 0 && !seen[rootID] {
		if rootNode, exists := byID[rootID]; exists {
			reversed = append(reversed, rootNode)
		}
	}

	chain := make([]LineageNode, len(reversed))
	for i := range reversed {
		chain[i] = reversed[len(reversed)-1-i]
	}

	// Build parent -> sorted child edges for downstream propagation trees.
	childrenByParent := make(map[int64][]int64)
	for _, id := range orderedIDs {
		n := byID[id]
		parentID := n.BasisEventID
		if parentID != 0 {
			if _, hasParent := byID[parentID]; hasParent {
				childrenByParent[parentID] = append(childrenByParent[parentID], id)
				continue
			}
		}
		// Fallback when intermediate basis event was compacted out but root exists.
		if n.RootEventID != 0 && n.RootEventID != id {
			if _, hasRoot := byID[n.RootEventID]; hasRoot {
				childrenByParent[n.RootEventID] = append(childrenByParent[n.RootEventID], id)
			}
		}
	}

	var buildTree func(id int64, visited map[int64]bool) *LineageTreeNode
	buildTree = func(id int64, visited map[int64]bool) *LineageTreeNode {
		node, exists := byID[id]
		if !exists || visited[id] {
			return nil
		}
		visited[id] = true
		tree := &LineageTreeNode{Node: node}
		for _, cid := range childrenByParent[id] {
			if child := buildTree(cid, visited); child != nil {
				tree.Children = append(tree.Children, child)
			}
		}
		return tree
	}

	downstream := buildTree(eventID, map[int64]bool{})
	var descIDs []int64
	var collectDesc func(*LineageTreeNode)
	collectDesc = func(t *LineageTreeNode) {
		if t == nil {
			return
		}
		for _, c := range t.Children {
			descIDs = append(descIDs, c.Node.EventID)
			collectDesc(c)
		}
	}
	collectDesc(downstream)

	var rootTree *LineageTreeNode
	if rootID != 0 && rootID != eventID {
		if _, hasRoot := byID[rootID]; hasRoot {
			rootTree = buildTree(rootID, map[int64]bool{})
		}
	}

	return &LineageInspection{
		TargetEventID:  eventID,
		RootEventID:    rootID,
		UpstreamChain:  chain,
		DownstreamTree: downstream,
		RootTree:       rootTree,
		DescendantIDs:  descIDs,
	}, nil
}

// PolitySummary summarizes a subordinate or linked PolityNode.
type PolitySummary struct {
	ID                  string `json:"id"`
	Name                string `json:"name"`
	Level               string `json:"level"`
	DeJureParentID      string `json:"de_jure_parent_id,omitempty"`
	DeFactoControllerID string `json:"de_facto_controller_id,omitempty"`
	LeaderActorID       string `json:"leader_actor_id,omitempty"`
	Treasury            int64  `json:"treasury"`
}

// PolityInspection reports statutory vs de-facto authority, leader, treasury, decrees, subordinates, and venues.
type PolityInspection struct {
	ID                  string          `json:"id"`
	Name                string          `json:"name"`
	Level               string          `json:"level"`
	DeJureParentID      string          `json:"de_jure_parent_id,omitempty"`
	DeFactoControllerID string          `json:"de_facto_controller_id,omitempty"`
	EffectiveChain      []string        `json:"effective_chain,omitempty"`
	EffectiveRulerID    string          `json:"effective_ruler_id,omitempty"`
	LeaderActorID       string          `json:"leader_actor_id,omitempty"`
	LeaderName          string          `json:"leader_name,omitempty"`
	SeatDistrictID      string          `json:"seat_district_id,omitempty"`
	SeatVenueID         string          `json:"seat_venue_id,omitempty"`
	Treasury            int64           `json:"treasury"`
	Ledger              TreasuryLedger  `json:"ledger"`
	MarketTaxPermille   int             `json:"market_tax_permille"`
	PollTaxPermille     int             `json:"poll_tax_permille"`
	Curfew              bool            `json:"curfew"`
	EmbargoItems        []string        `json:"embargo_items,omitempty"`
	ActiveDecrees       []PolityDecree  `json:"active_decrees,omitempty"`
	DeJureChildren      []string        `json:"de_jure_children,omitempty"`
	DeFactoSubjects     []string        `json:"de_facto_subjects,omitempty"`
	SubordinateNodes    []PolitySummary `json:"subordinate_nodes,omitempty"`
	Districts           []string        `json:"districts,omitempty"`
	Venues              []Venue         `json:"venues,omitempty"`
}

// InspectPolity returns a structured inspection report for polityID in w.
func InspectPolity(w *World, polityID string) (*PolityInspection, error) {
	if w == nil {
		return nil, fmt.Errorf("world is nil")
	}
	if w.Polities == nil {
		return nil, fmt.Errorf("world %q has no polity network", w.ID)
	}
	node := w.Polities.NodeByID(polityID)
	if node == nil {
		return nil, fmt.Errorf("unknown polity: %s", polityID)
	}
	chainNodes := w.Polities.EffectivePolityChain(polityID)
	chainIDs := make([]string, len(chainNodes))
	for i, n := range chainNodes {
		chainIDs[i] = n.ID
	}
	effRuler := ""
	if len(chainIDs) > 0 {
		effRuler = chainIDs[len(chainIDs)-1]
	}
	leaderName := ""
	if node.LeaderActorID != "" {
		if la := w.actor(node.LeaderActorID); la != nil {
			leaderName = la.Name
		}
	}

	var deJureChildren, deFactoSubjects []string
	var subordinates []PolitySummary
	seenSub := make(map[string]bool)
	for _, n := range w.Polities.Nodes {
		if n.ID == polityID {
			continue
		}
		isDeJureChild := n.DeJureParentID == polityID
		isDeFactoSubj := w.Polities.PolityGovernsNode(polityID, n.ID)
		if isDeJureChild {
			deJureChildren = append(deJureChildren, n.ID)
		}
		if isDeFactoSubj {
			deFactoSubjects = append(deFactoSubjects, n.ID)
		}
		if (isDeJureChild || isDeFactoSubj) && !seenSub[n.ID] {
			seenSub[n.ID] = true
			subordinates = append(subordinates, PolitySummary{
				ID:                  n.ID,
				Name:                n.Name,
				Level:               n.Level,
				DeJureParentID:      n.DeJureParentID,
				DeFactoControllerID: n.DeFactoControllerID,
				LeaderActorID:       n.LeaderActorID,
				Treasury:            n.Treasury,
			})
		}
	}

	districtPolity := make(map[string]string, len(w.Map.Districts))
	var districts []string
	for _, d := range w.Map.Districts {
		districtPolity[d.ID] = d.PolityID
		if d.PolityID == polityID {
			districts = append(districts, d.ID)
		}
	}
	var venues []Venue
	for _, v := range w.Map.Venues {
		vp := v.PolityID
		if vp == "" {
			vp = districtPolity[v.DistrictID]
		}
		if vp == polityID {
			vc := v
			if v.Features != nil {
				vc.Features = append([]string(nil), v.Features...)
			}
			venues = append(venues, vc)
		}
	}

	return &PolityInspection{
		ID:                  node.ID,
		Name:                node.Name,
		Level:               node.Level,
		DeJureParentID:      node.DeJureParentID,
		DeFactoControllerID: node.DeFactoControllerID,
		EffectiveChain:      chainIDs,
		EffectiveRulerID:    effRuler,
		LeaderActorID:       node.LeaderActorID,
		LeaderName:          leaderName,
		SeatDistrictID:      node.SeatDistrictID,
		SeatVenueID:         node.SeatVenueID,
		Treasury:            node.Treasury,
		Ledger:              node.Ledger,
		MarketTaxPermille:   node.MarketTaxPermille,
		PollTaxPermille:     node.PollTaxPermille,
		Curfew:              node.Curfew,
		EmbargoItems:        append([]string(nil), node.EmbargoItems...),
		ActiveDecrees:       append([]PolityDecree(nil), node.Decrees...),
		DeJureChildren:      deJureChildren,
		DeFactoSubjects:     deFactoSubjects,
		SubordinateNodes:    subordinates,
		Districts:           districts,
		Venues:              venues,
	}, nil
}

// CohortSummary summarizes one macro PopulationCohort for conservation inspection.
type CohortSummary struct {
	ID                  string `json:"id"`
	Name                string `json:"name"`
	District            string `json:"district"`
	FactionID           string `json:"faction_id,omitempty"`
	Profession          string `json:"profession"`
	Population          int    `json:"population"`
	EffectivePopulation int    `json:"effective_population"`
	Wounded             int    `json:"wounded,omitempty"`
	Sick                int    `json:"sick,omitempty"`
	Jailed              int    `json:"jailed,omitempty"`
	MoneyPool           int64  `json:"money_pool"`
}

// ConservationInspection reports global demographic and M0 monetary invariants of a World.
type ConservationInspection struct {
	WorldID           string          `json:"world_id"`
	Tick              int64           `json:"tick"`
	TotalPopulation   int             `json:"total_population"`
	LivingActors      int             `json:"living_actors"`
	DeadActors        int             `json:"dead_actors"`
	CohortPopulation  int             `json:"cohort_population"`
	TotalCoins        int64            `json:"total_coins"`
	ActorCoins        int64            `json:"actor_coins"`
	CohortCoins       int64            `json:"cohort_coins"`
	PublicTreasury    int64            `json:"public_treasury"`
	PolityTreasurySum int64            `json:"polity_treasury_sum"`
	TreasuryLedger    TreasuryLedger   `json:"treasury_ledger"`
	Currencies        map[string]int64 `json:"currencies,omitempty"`
	LedgerConserved   bool             `json:"ledger_conserved"`
	LedgerError       string           `json:"ledger_error,omitempty"`
	ActiveOffers      int              `json:"active_offers"`
	HouseCount        int             `json:"house_count"`
	CohortCount       int             `json:"cohort_count"`
	Cohorts           []CohortSummary `json:"cohorts,omitempty"`
}

// InspectWorldConservation computes population, M0 coin conservation, treasury, active offers,
// house count, and cohort summary for w.
func InspectWorldConservation(w *World) *ConservationInspection {
	if w == nil {
		return &ConservationInspection{}
	}
	var living, dead, cohortPop int
	var actorCoins, cohortCoins, polityTreasurySum int64
	for i := range w.Actors {
		if w.Actors[i].Dead {
			dead++
		} else {
			living++
			actorCoins += int64(w.Actors[i].Money)
		}
	}
	cohorts := make([]CohortSummary, 0, len(w.Cohorts))
	for i := range w.Cohorts {
		c := w.Cohorts[i]
		cohortPop += c.Population
		cohortCoins += c.MoneyPool
		cohorts = append(cohorts, CohortSummary{
			ID:                  c.ID,
			Name:                c.Name,
			District:            c.District,
			FactionID:           c.FactionID,
			Profession:          c.Profession,
			Population:          c.Population,
			EffectivePopulation: c.EffectivePopulation(),
			Wounded:             c.Wounded,
			Sick:                c.Sick,
			Jailed:              c.Jailed,
			MoneyPool:           c.MoneyPool,
		})
	}
	if w.Polities != nil {
		for i := range w.Polities.Nodes {
			polityTreasurySum += w.Polities.Nodes[i].Treasury
		}
	}
	curKeys := w.allHeldCurrencies()
	var currencies map[string]int64
	if len(curKeys) > 0 {
		currencies = make(map[string]int64, len(curKeys))
		for _, cur := range curKeys {
			currencies[cur] = w.totalCurrencyHeld(cur)
		}
	}
	ledgerConserved := false
	ledgerErr := ""
	if err := w.VerifyLedgerConservation(); err != nil {
		ledgerErr = err.Error()
	} else {
		ledgerConserved = true
	}
	return &ConservationInspection{
		WorldID:           w.ID,
		Tick:              w.Tick,
		TotalPopulation:   w.TotalPopulation(),
		LivingActors:      living,
		DeadActors:        dead,
		CohortPopulation:  cohortPop,
		TotalCoins:        w.TotalCoins(),
		ActorCoins:        actorCoins,
		CohortCoins:       cohortCoins,
		PublicTreasury:    w.PublicTreasury,
		PolityTreasurySum: polityTreasurySum,
		TreasuryLedger:    w.TreasuryLedger,
		Currencies:        currencies,
		LedgerConserved:   ledgerConserved,
		LedgerError:       ledgerErr,
		ActiveOffers:      len(w.Offers),
		HouseCount:        len(w.Houses),
		CohortCount:       len(w.Cohorts),
		Cohorts:           cohorts,
	}
}
