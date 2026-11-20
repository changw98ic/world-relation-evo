package optiongen

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/changw98ic/world-relation-evo/internal/sim"
)

// ExpandConfig controls offline per-character option expansion.
type ExpandConfig struct {
	PackPath    string
	OutDir      string
	HorizonDays int
	Concurrency int
	ActorFilter string
	Resume      bool
	Progress    io.Writer
}

// ExpandWorld builds the complete dossier for each character in w, makes
// exactly one DS request per character, and persists per-actor JSON plus
// summary.json and overview.md into cfg.OutDir.
func ExpandWorld(ctx context.Context, w *sim.World, client Client, cfg ExpandConfig) (*WorldExpansionSummary, error) {
	if w == nil {
		return nil, fmt.Errorf("world is nil")
	}
	if cfg.HorizonDays <= 0 {
		cfg.HorizonDays = 30
	}
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 3
	}
	if strings.TrimSpace(cfg.OutDir) == "" {
		return nil, fmt.Errorf("output directory is required")
	}

	actorsDir := filepath.Join(cfg.OutDir, "actors")
	if err := os.MkdirAll(actorsDir, 0o755); err != nil {
		return nil, fmt.Errorf("create output dir: %w", err)
	}

	var targets []sim.Actor
	for _, a := range w.Actors {
		if a.Dead {
			continue
		}
		if cfg.ActorFilter != "" && a.ID != cfg.ActorFilter {
			continue
		}
		targets = append(targets, a)
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("no matching living actors found (filter=%q)", cfg.ActorFilter)
	}

	started := time.Now().UTC()
	records := make([]ActorExpansionRecord, len(targets))
	errs := make([]error, len(targets))

	sem := make(chan struct{}, cfg.Concurrency)
	var wg sync.WaitGroup
	var logMu sync.Mutex

	for i, actor := range targets {
		wg.Add(1)
		go func(idx int, a sim.Actor) {
			defer wg.Done()

			outPath := filepath.Join(actorsDir, a.ID+".json")
			if cfg.Resume {
				if existing, ok := loadExistingRecord(outPath, a.ID); ok {
					records[idx] = existing
					if cfg.Progress != nil {
						logMu.Lock()
						fmt.Fprintf(cfg.Progress, "[%d/%d] resumed %s (%s): %d options from %s\n",
							idx+1, len(targets), a.ID, a.Name, len(existing.Catalog.Options), outPath)
						logMu.Unlock()
					}
					return
				}
			}

			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				errs[idx] = ctx.Err()
				return
			}

			dossier, err := BuildDossier(w, a.ID, cfg.HorizonDays)
			if err != nil {
				errs[idx] = err
				return
			}

			if cfg.Progress != nil {
				logMu.Lock()
				fmt.Fprintf(cfg.Progress, "[%d/%d] requesting ds for %s (%s, %s)...\n", idx+1, len(targets), a.ID, a.Name, a.Role)
				logMu.Unlock()
			}

			rec, err := client.ExpandActor(ctx, dossier)
			if err != nil {
				errs[idx] = err
				if cfg.Progress != nil {
					logMu.Lock()
					fmt.Fprintf(cfg.Progress, "[%d/%d] ERROR %s (%s): %v\n", idx+1, len(targets), a.ID, a.Name, err)
					logMu.Unlock()
				}
				return
			}

			if err := writeJSONFile(outPath, rec); err != nil {
				errs[idx] = err
				return
			}
			records[idx] = rec

			if cfg.Progress != nil {
				logMu.Lock()
				fmt.Fprintf(cfg.Progress, "[%d/%d] done %s (%s): %d options, %d tokens, %dms -> %s\n",
					idx+1, len(targets), a.ID, a.Name, len(rec.Catalog.Options), rec.TotalTokens, rec.DurationMS, outPath)
				logMu.Unlock()
			}
		}(i, actor)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			return nil, fmt.Errorf("actor %s failed: %w", targets[i].ID, err)
		}
	}

	endpointURL, _ := client.chatEndpoint()
	summary := &WorldExpansionSummary{
		WorldID:       w.ID,
		PackPath:      cfg.PackPath,
		HorizonDays:   cfg.HorizonDays,
		Model:         client.Model,
		Endpoint:      endpointURL,
		StartedAt:     started,
		CompletedAt:   time.Now().UTC(),
		TotalActors:   len(targets),
		TotalRequests: len(targets),
		Actors:        make([]ActorExpansionBrief, 0, len(targets)),
	}

	for i, rec := range records {
		byCat := map[string]int{}
		newPrimSet := map[string]struct{}{}
		for _, opt := range rec.Catalog.Options {
			cat := opt.Category
			if cat == "" {
				cat = "uncategorized"
			}
			byCat[cat]++
			if strings.HasPrefix(opt.Action.EngineKind, "new_primitive:") {
				newPrimSet[opt.Action.EngineKind] = struct{}{}
			}
		}
		newPrims := make([]string, 0, len(newPrimSet))
		for k := range newPrimSet {
			newPrims = append(newPrims, k)
		}
		sort.Strings(newPrims)

		summary.TotalOptions += len(rec.Catalog.Options)
		summary.TotalTokens += rec.TotalTokens
		summary.Actors = append(summary.Actors, ActorExpansionBrief{
			ActorID:       rec.ActorID,
			ActorName:     rec.ActorName,
			Role:          targets[i].Role,
			Location:      targets[i].Location,
			OptionCount:   len(rec.Catalog.Options),
			ByCategory:    byCat,
			DurationMS:    rec.DurationMS,
			TotalTokens:   rec.TotalTokens,
			OutputFile:    filepath.Join("actors", rec.ActorID+".json"),
			NewPrimitives: newPrims,
		})
	}

	if err := writeJSONFile(filepath.Join(cfg.OutDir, "summary.json"), summary); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(cfg.OutDir, "overview.md"), []byte(renderOverviewMarkdown(summary, records)), 0o644); err != nil {
		return nil, err
	}
	return summary, nil
}

func loadExistingRecord(path, expectedActorID string) (ActorExpansionRecord, bool) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ActorExpansionRecord{}, false
	}
	var rec ActorExpansionRecord
	if err := json.Unmarshal(raw, &rec); err != nil {
		return ActorExpansionRecord{}, false
	}
	if rec.ActorID != expectedActorID || len(rec.Catalog.Options) == 0 {
		return ActorExpansionRecord{}, false
	}
	return rec, true
}

func writeJSONFile(path string, v any) error {
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return os.WriteFile(path, raw, 0o644)
}

func renderOverviewMarkdown(s *WorldExpansionSummary, records []ActorExpansionRecord) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s — %d 天角色选项扩充总览\n\n", s.WorldID, s.HorizonDays)
	fmt.Fprintf(&b, "- 模型：`%s`\n", s.Model)
	fmt.Fprintf(&b, "- 角色数 / DS 请求数：`%d` / `%d`（每角色 1 次请求）\n", s.TotalActors, s.TotalRequests)
	fmt.Fprintf(&b, "- 生成总选项数：`%d`\n", s.TotalOptions)
	fmt.Fprintf(&b, "- 总 Token 消耗：`%d`\n\n", s.TotalTokens)

	b.WriteString("| 角色 ID | 角色名 | 身份 | 初始地点 | 选项数 | 耗时 (ms) | 建议新原语数 |\n")
	b.WriteString("| --- | --- | --- | --- | ---: | ---: | ---: |\n")
	for _, a := range s.Actors {
		fmt.Fprintf(&b, "| `%s` | %s | `%s` | `%s` | %d | %d | %d |\n",
			a.ActorID, a.ActorName, a.Role, a.Location, a.OptionCount, a.DurationMS, len(a.NewPrimitives))
	}

	for _, rec := range records {
		fmt.Fprintf(&b, "\n## `%s` — %s\n\n", rec.ActorID, rec.ActorName)
		if rec.Catalog.CharacterSummary != "" {
			fmt.Fprintf(&b, "> %s\n\n", rec.Catalog.CharacterSummary)
		}
		if len(rec.Catalog.CoreDrives) > 0 {
			fmt.Fprintf(&b, "- **核心驱动**：%s\n", strings.Join(rec.Catalog.CoreDrives, "；"))
		}
		if len(rec.Catalog.KeyTensions) > 0 {
			fmt.Fprintf(&b, "- **核心张力**：%s\n", strings.Join(rec.Catalog.KeyTensions, "；"))
		}
		b.WriteString("\n| ID | 类别 | 天数 | 优先级 | 动作原语 | 标题 | 触发条件（if） | 潜在碰撞 |\n")
		b.WriteString("| --- | --- | --- | --- | --- | --- | --- | --- |\n")
		for _, opt := range rec.Catalog.Options {
			cond := strings.Join(opt.WhenIf.Conditions, "; ")
			if opt.WhenIf.TriggerEvent != "" {
				if cond != "" {
					cond += " | 触发: " + opt.WhenIf.TriggerEvent
				} else {
					cond = "触发: " + opt.WhenIf.TriggerEvent
				}
			}
			coll := strings.Join(opt.PotentialCollisions, "; ")
			fmt.Fprintf(&b, "| `%s` | `%s` | D%d-D%d | `%s` | `%s` | %s | %s | %s |\n",
				opt.ID, opt.Category, opt.DayWindow[0], opt.DayWindow[1], opt.Priority,
				opt.Action.EngineKind, escapePipe(opt.Title), escapePipe(cond), escapePipe(coll))
		}
	}
	return b.String()
}

func escapePipe(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.ReplaceAll(s, "|", "\\|")
}
