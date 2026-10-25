package strategy

import (
	"context"
	"strings"
	"sync"

	iradix "github.com/hashicorp/go-immutable-radix/v2"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	nmstore "github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/strategy/tokenizer"
	"github.com/theapemachine/symm/system"
)

type Model struct {
	*runtime.System `json:"-"`
	desk            *broker.Desk
	store           *store.Client
	stage           *Stage
	path            *Path
	policy          *iradix.Tree[*data.Value]
	explore         *iradix.Tree[*data.Value]
	cliff           *sync.Map
}

func NewModel(
	ctx context.Context,
	desk *broker.Desk,
	client *store.Client,
	stage *Stage,
) *Model {
	if client == nil {
		client = store.NewClient(ctx, system.Cfg.Storage.S3.Bucket)
	}

	model := &Model{
		System:  runtime.NewSystem(ctx, "model"),
		desk:    desk,
		store:   client,
		stage:   stage,
		path:    NewPath(),
		policy:  iradix.New[*data.Value](),
		explore: iradix.New[*data.Value](),
	}

	if client != nil {
		model.loadPolicyFromStore()
		model.loadExploreFromStore()
	}

	return model
}

/*
Decide is the join stage: path, policy, and desk using grammar and resonance
artifacts the parallel fork already wrote on tick.
*/
func (model *Model) Decide(
	out *data.Value,
	tick *data.Value,
) *data.Value {
	if model == nil || out == nil || tick == nil {
		return out
	}

	symbol := out.Scope

	var observed []*nmstore.Region
	tick.Lookup(&observed, strings.Join([]string{"stage", symbol, "regions"}, "/"))

	out.Append(observed, "train", "regions")

	var step tokenizer.StepResult
	tick.Lookup(&step, strings.Join([]string{"stage", symbol, "step"}, "/"))

	var cut tokenizer.Token
	tick.Lookup(&cut, strings.Join([]string{"stage", symbol, "cut"}, "/"))

	frictionPass, frictionChecked := frictionAllowsEntry(observed)
	out.Append(map[string]any{
		"frictionPass":    frictionPass,
		"frictionChecked": frictionChecked,
	}, "measurement")

	out.Append(observed, "lit")
	out.Append(cut, "cut")
	appendEmitReadout(out, step, len(observed) > 0)

	var surprise []float64
	tick.Lookup(&surprise, resonanceLayerKey(symbol, "surprise"))

	if len(surprise) > 0 {
		out.Append(surprise, "resonance", "surprise")
	}

	holdingPaper := model.desk.Has(symbol, false)
	holdingVirtual := model.desk.Has(symbol, true)
	enter := !holdingPaper && !holdingVirtual

	pathKey := model.path.After(model.path.PolicyLive(symbol, enter), cut)

	if pathKey != "" {
		out.Append(pathKey, "path")
	}

	if cut.String() != "" {
		if enter {
			model.path.Append(symbol, cut)
		} else {
			model.path.AppendTrigger(symbol, cut)
		}
	}

	out.Append(model.path.Live(symbol), "livePath")
	out.Append(model.path.TriggerLive(symbol), "triggerPath")
	out.Append(pathKey, "policyPath")
	out.Append(enter, "enter")

	disagree := false
	paper := false
	values := make(map[string]*data.Value)

	if pathKey != "" {
		trees := []*iradix.Tree[*data.Value]{model.policy, model.explore}
		onPaper := []bool{true, false}

		if holdingVirtual {
			trees = []*iradix.Tree[*data.Value]{model.explore}
			onPaper = []bool{false}
		} else if holdingPaper {
			trees = []*iradix.Tree[*data.Value]{model.policy}
			onPaper = []bool{true}
		}

		for i, tree := range trees {
			matched := 0
			tree.Root().WalkPrefix([]byte(pathKey), func(k []byte, v *data.Value) bool {
				if v == nil {
					return false
				}

				var stored string
				v.Lookup(&stored, "action")

				// A leaf agrees when its action is the one this phase can take.
				// return true stops the walk; false keeps going so every path is seen.
				switch stored {
				case "enter":
					if !enter {
						disagree = true
						return true
					}

					// For explore leaves, check if cut brightness crosses threshold.
					// Lower threshold (from winning paths) is easier to cross.
					if !onPaper[i] {
						threshold := 0.5
						for idx, index := range v.Index {
							if strings.Contains(index, "threshold") {
								var thresh float64
								v.At(&thresh, idx)
								threshold += thresh
							}
						}
						if cut.Brightness > 0 && cut.Brightness < threshold {
							return false
						}
					}

					values[string(k)] = v
					matched++
				case "exit":
					if enter {
						disagree = true
						return true
					}

					values[string(k)] = v
					matched++
				}

				return false
			})

			if disagree {
				break
			}
			if matched > 0 {
				paper = onPaper[i]
				break
			}
		}
	}

	if holdingPaper || holdingVirtual {
		model.updateCliffTrack(symbol, cliffReadingFromTick(tick, symbol))
		if model.tryResonanceCliffExit(out, tick, symbol, pathKey, holdingVirtual) {
			return out
		}
	}

	if !disagree && len(values) > 0 && pathDepth(pathKey) >= minPathDepth() {
		virtual := !paper
		if enter {
			if frictionChecked && !frictionPass {
				out.Append("friction", "blockedReason")
				return out
			}
			if !model.resonanceAllowsEntry(tick, symbol, observed) {
				out.Append("resonance veto", "blockedReason")
				return out
			}
			if paper && !model.paperRoom() {
				return out
			}
			if paper {
				virtual = false
			}
			if err := model.desk.Enter(symbol, pathKey, virtual, true); err != nil {
				return out
			}
			model.path.Reset(symbol)
			model.resetCliffTrack(symbol)
			return out
		}

		position := model.desk.Position(symbol, virtual)
		if position != nil && pathKey == position.EntryPath {
			// The exit sentence has not left the entry prefix yet.
		} else {
			if position != nil {
				if paper {
					position.AppendOutcomeTrigger("policy exit")
				} else {
					position.AppendOutcomeTrigger("explore exit")
				}
			}
			if err := model.desk.Exit(symbol, pathKey, virtual); err != nil {
				return out
			} else if paper {
				model.path.Reset(symbol)
				model.stage.ResetSequencer(symbol)
				model.resetCliffTrack(symbol)
				out.Append("exit submitted", "outcome")
				return out
			} else {

				for key, value := range values {
					if position != nil && position.PnL != nil && position.PnL.Sign() > 0 {
						value.Append(-0.05, "threshold")
					} else {
						value.Append(0.05, "threshold")
					}

					model.explore, _, _ = model.explore.Insert(
						[]byte(key),
						value,
					)

					threshold := 0.5

					for idx, index := range value.Index {
						if strings.Contains(index, "threshold") {
							var thresh float64
							value.At(&thresh, idx)
							threshold += thresh
						}
					}

					if threshold > 0.8 {
						model.store.DeletePrefix("model/explore/" + key + "/")
						model.explore, _ = model.explore.DeletePrefix([]byte(key))
					}

					if threshold < 0.2 {
						suffix := "enter.bin"
						var action string
						value.Lookup(&action, "action")

						if action == "exit" {
							suffix = "exit.bin"
						}

						model.store.WriteAtPrefix(
							"model/policy/"+key+"/"+suffix,
							value.Bytes(),
						)

						model.policy, _, _ = model.policy.Insert(
							[]byte(key),
							value,
						)

						model.store.DeletePrefix("model/explore/" + key + "/")
						model.explore, _ = model.explore.DeletePrefix([]byte(key))

						// The exit sentence starts from its own root, so the
						// entry path is a different leaf. Paper needs both.
						if position != nil && position.EntryPath != "" && position.EntryPath != key {
							entry, ok := model.explore.Get([]byte(position.EntryPath))
							if !ok || entry == nil {
								entry = model.store.Download(
									model.Context(),
									"model/explore/"+position.EntryPath+"/enter.bin",
								)
							}
							if entry == nil {
								entry = data.NewValue(
									"model", "policy", position.Symbol,
								).Append(
									"enter", "action",
								)
								entry.Append(explorePnLStep(position), "threshold")
							}
							model.store.WriteAtPrefix(
								"model/policy/"+position.EntryPath+"/enter.bin",
								entry.Bytes(),
							)
							model.policy, _, _ = model.policy.Insert(
								[]byte(position.EntryPath),
								entry,
							)
							model.store.DeletePrefix("model/explore/" + position.EntryPath + "/")
							model.explore, _ = model.explore.DeletePrefix([]byte(position.EntryPath))
						}
					}
				}

				if model.writePaths(position) {
					model.stage.ResetSequencer(symbol)
					out.Append("exit submitted", "outcome")
				}
			}
		}
	}

	if position := model.desk.Position(symbol, true); position != nil {
		exitReady := pathDepth(pathKey) >= minPathDepth() && pathKey != position.EntryPath
		if trigger, ok := position.ShouldExit(); ok && exitReady {
			position.AppendOutcomeTrigger(trigger)
			if err := model.desk.Exit(symbol, pathKey, true); err != nil {
				return out
			}
			if model.writePaths(position) {
				model.stage.ResetSequencer(symbol)
				out.Append("exit submitted", "outcome")
			}
			return out
		}
	}

	if model.explorePath(symbol, pathKey, tick, observed) {
		out.Append("enter submitted", "outcome")
	}
	return out
}

func pathDepth(pathKey string) int {
	if pathKey == "" {
		return 0
	}
	return strings.Count(pathKey, "/") + 1
}

func minPathDepth() int {
	if system.Cfg != nil && system.Cfg.Learning != nil && system.Cfg.Learning.MinPathLength > 1 {
		return system.Cfg.Learning.MinPathLength
	}
	return 2
}

func (model *Model) paperRoom() bool {
	if model == nil || model.desk == nil || system.Cfg == nil || system.Cfg.Learning == nil {
		return true
	}

	max := system.Cfg.Learning.MaxVenuePositions
	if max <= 0 {
		return true
	}

	return len(model.desk.OpenPositions(false)) < max
}

func (model *Model) explorePath(
	symbol string,
	pathKey string,
	tick *data.Value,
	regions []*nmstore.Region,
) bool {
	if pathDepth(pathKey) < minPathDepth() {
		return false
	}

	frictionPass, frictionChecked := frictionAllowsEntry(regions)
	if frictionChecked && !frictionPass {
		return false
	}
	if !model.resonanceAllowsEntry(tick, symbol, regions) {
		return false
	}

	if err := model.desk.Enter(symbol, pathKey, true, false); err != nil {
		return false
	}

	model.path.Reset(symbol)
	return true
}

func explorePnLStep(position *broker.Position) float64 {
	if position != nil && position.PnL != nil && position.PnL.Sign() > 0 {
		return -0.05
	}

	return 0.05
}

func (model *Model) persistExploreLeaf(
	symbol, pathKey, action string,
	position *broker.Position,
) {
	if model == nil || model.store == nil || pathKey == "" || action == "" {
		return
	}

	if model.onPolicy(pathKey) {
		return
	}

	storeKey := "model/explore/" + pathKey + "/" + action + ".bin"

	leaf := model.store.Download(model.Context(), storeKey)
	if leaf == nil {
		leaf = data.NewValue(
			"model", "explore", symbol,
		).Append(
			action, "action",
		)
	}

	leaf.Append(explorePnLStep(position), "threshold")

	model.explore, _, _ = model.explore.Insert([]byte(pathKey), leaf)
	model.store.WriteAtPrefix(storeKey, leaf.Bytes())
}

func (model *Model) writePaths(position *broker.Position) bool {
	if position == nil || model == nil || model.store == nil {
		return false
	}

	if position.EntryPath == "" && position.ExitPath == "" {
		errnie.Error(errnie.Err(
			errnie.NotFound, "[model.writePaths] missing entry and exit path", nil,
		))

		return false
	}

	if position.EntryPath != "" {
		model.persistExploreLeaf(position.Symbol, position.EntryPath, "enter", position)
	}

	if position.ExitPath != "" {
		model.persistExploreLeaf(position.Symbol, position.ExitPath, "exit", position)
	}

	model.path.Reset(position.Symbol)
	return true
}

/*
RecordExploreClose persists enter/exit explore leaves after the desk has
settled a lot (including dashboard manual exit).
*/
func (model *Model) RecordExploreClose(position *broker.Position) bool {
	return model.writePaths(position)
}

/*
DeskExitPath chooses the exit grammar path for a hub-initiated close.
*/
func (model *Model) DeskExitPath(symbol string, position *broker.Position) string {
	if position == nil {
		return ""
	}

	return model.emergencyExitPath(symbol, "", position)
}

func (model *Model) onPolicy(pathKey string) bool {
	if model == nil || model.policy == nil || pathKey == "" {
		return false
	}

	_, ok := model.policy.Get([]byte(pathKey))
	return ok
}

func (model *Model) exploreInsertAction(pathKey, action string) {
	if model == nil || pathKey == "" || action == "" {
		return
	}

	model.explore, _, _ = model.explore.Insert(
		[]byte(pathKey),
		data.NewValue("model", "explore", "").Append(action, "action"),
	)
}

func (model *Model) loadPolicyFromStore() {
	if model == nil || model.store == nil || system.Cfg.Learning == nil {
		return
	}

	cfg := system.Cfg.Learning
	prefixes := []string{"model/policy/"}
	if cfg.PolicyPrefix != "" && cfg.PolicyPrefix != "model/policy/" {
		prefixes = append(prefixes, cfg.PolicyPrefix)
	}

	for _, prefix := range prefixes {
		model.loadPolicyPrefix(prefix, cfg.EnterSuffix, cfg.ExitSuffix)
	}
}

func (model *Model) loadPolicyPrefix(prefix, enterSuffix, exitSuffix string) {
	for objectKey := range model.store.ListKeys(prefix) {
		pathKey, action, ok := trieObjectKey(objectKey, prefix, enterSuffix, exitSuffix)
		if !ok {
			continue
		}

		value := model.store.Download(model.Context(), objectKey)
		if value == nil {
			continue
		}

		var stored string
		value.Lookup(&stored, "action")
		if stored == "" {
			value.Append(action, "action")
		}

		model.policy, _, _ = model.policy.Insert([]byte(pathKey), value)
	}
}

func (model *Model) loadExploreFromStore() {
	if model == nil || model.store == nil || system.Cfg.Learning == nil {
		return
	}

	cfg := system.Cfg.Learning

	for objectKey := range model.store.ListKeys("model/explore/") {
		pathKey, action, ok := trieObjectKey(objectKey, "model/explore/", cfg.EnterSuffix, cfg.ExitSuffix)
		if !ok {
			continue
		}

		model.exploreInsertAction(pathKey, action)
	}
}

func trieObjectKey(objectKey, prefix, enterSuffix, exitSuffix string) (pathKey, action string, ok bool) {
	if !strings.HasPrefix(objectKey, prefix) {
		return "", "", false
	}

	rest := strings.TrimPrefix(objectKey, prefix)

	switch {
	case strings.HasSuffix(rest, enterSuffix):
		return strings.TrimSuffix(rest, enterSuffix), "enter", true
	case strings.HasSuffix(rest, exitSuffix):
		return strings.TrimSuffix(rest, exitSuffix), "exit", true
	default:
		return "", "", false
	}
}
