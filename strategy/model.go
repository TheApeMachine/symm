package strategy

import (
	"context"
	"strings"

	iradix "github.com/hashicorp/go-immutable-radix/v2"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	nmstore "github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/strategy/tokenizer"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/ui"
)

const exploreRadixSep = "\x00"

func exploreRadixKey(pathKey, action string) []byte {
	if action == "" {
		return []byte(pathKey)
	}
	return []byte(pathKey + exploreRadixSep + action)
}

type Model struct {
	*runtime.System `json:"-"`
	desk            *broker.Desk
	store           *store.Client
	stage           *Stage
	path            *Path
	momentum        *Momentum
	resonance       *Resonance
	policy          *iradix.Tree[*data.Value]
	explore         *iradix.Tree[*data.Value]
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
		System:    runtime.NewSystem(ctx, "model"),
		desk:      desk,
		store:     client,
		stage:     stage,
		path:      NewPath(),
		momentum:  NewMomentum(),
		resonance: NewResonance(),
		policy:    iradix.New[*data.Value](),
		explore:   iradix.New[*data.Value](),
	}

	if client != nil {
		model.loadPolicyFromStore()
		model.loadExploreFromStore()
	}

	return model
}

/*
Decide joins grammar path, policy/explore trie, resonance gates, and the desk.
Virtual flow: prefer stored paths, else explore; on exit learn both paths.
*/
func (model *Model) Decide(out *data.Value, tick *data.Value) *data.Value {
	if out == nil || tick == nil {
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

	frictionPass, frictionChecked := model.resonance.FrictionAllows(observed)
	out.Append(map[string]any{
		"frictionPass":    frictionPass,
		"frictionChecked": frictionChecked,
	}, "measurement")

	out.Append(observed, "lit")
	out.Append(cut, "cut")
	appendEmitReadout(out, step, len(observed) > 0)

	var surprise []float64
	tick.Lookup(&surprise, model.resonance.LayerKey(symbol, "surprise"))
	if len(surprise) > 0 {
		out.Append(surprise, "resonance", "surprise")
	}

	holdingPaper := model.desk.Has(symbol, false)
	holdingVirtual := model.desk.Has(symbol, true)
	enter := !holdingPaper && !holdingVirtual

	var pathKey string

	if len(step.Spell) > 0 {
		pathKey = model.path.Follow(symbol, enter, step.Spell)
	} else {
		pathKey = model.path.After(model.path.PolicyLive(symbol, enter), cut)
		if cut.String() != "" {
			if enter {
				model.path.Append(symbol, cut)
			} else {
				model.path.AppendTrigger(symbol, cut)
			}
		}
	}

	if pathKey != "" {
		out.Append(pathKey, "path")
	}

	out.Append(model.path.Live(symbol), "livePath")
	out.Append(model.path.TriggerLive(symbol), "triggerPath")
	out.Append(pathKey, "policyPath")
	out.Append(enter, "enter")

	lit := cut.String() != ""
	// Flat only: a virtual hold is still walking the path; silence is not disuse.
	if pathKey != "" && !lit && enter && !holdingPaper && !holdingVirtual {
		model.penalizeExploreInactivity(out, symbol, pathKey)
	}

	var paper bool
	var values map[string]*data.Value
	var disagree bool

	if !enter {
		paper, values, disagree = model.matchStoredPaths(
			pathKey, enter, holdingPaper, holdingVirtual,
		)
	}

	if holdingPaper || holdingVirtual {
		if model.resonance != nil {
			model.resonance.NoteHold(symbol, tick)
		}

		if holdingVirtual && model.tryMomentumExit(out, tick, symbol, pathKey, true) {
			return out
		}

		if holdingPaper && model.tryMomentumExit(out, tick, symbol, pathKey, false) {
			return out
		}

		if model.resonance != nil && model.resonance.TryCliffExit(
			model, out, tick, symbol, pathKey, holdingVirtual,
		) {
			return out
		}
	}

	if enter && pathKey != "" {
		if model.tryExploreStoredEnter(out, symbol, pathKey) {
			return out
		}
		if model.tryPolicyStoredEnter(
			out, tick, symbol, pathKey, observed, frictionPass, frictionChecked,
		) {
			return out
		}
		if model.exploreNewPath(symbol, pathKey) {
			out.Append("enter submitted", "outcome")
		}
		return out
	}

	if !disagree && len(values) > 0 && pathKey != "" {
		virtual := !paper

		position := model.desk.Position(symbol, virtual)
		if position != nil && pathKey == position.EntryPath {
			// Exit sentence still on the entry prefix.
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
			}
			if paper {
				model.path.Reset(symbol)
				model.stage.ResetSequencer(symbol)
				if model.resonance != nil {
					model.resonance.ForgetHold(symbol)
				}
				out.Append("exit submitted", "outcome")
				return out
			}

			if model.finishVirtualExplore(position, out) {
				model.stage.ResetSequencer(symbol)
				out.Append("exit submitted", "outcome")
			}
		}
	}

	if position := model.desk.Position(symbol, true); position != nil {
		exitReady := pathKey != "" && pathKey != position.EntryPath
		if trigger, ok := position.ShouldExit(); ok && exitReady {
			position.AppendOutcomeTrigger(trigger)
			if err := model.desk.Exit(symbol, pathKey, true); err != nil {
				return out
			}
			if model.finishVirtualExplore(position, out) {
				model.stage.ResetSequencer(symbol)
				out.Append("exit submitted", "outcome")
			}
			return out
		}
	}

	return out
}

func (model *Model) penalizeExploreInactivity(
	out *data.Value, symbol, pathKey string,
) {
	if model == nil || pathKey == "" || model.onPolicy(pathKey) {
		return
	}

	penalty := exploreTemporalPenalty()
	if penalty <= 0 {
		return
	}

	for _, action := range []string{"enter", "exit"} {
		if _, ok := model.exploreLeafAt(pathKey, action); !ok {
			continue
		}
		model.bumpExploreLeaf(symbol, pathKey, action, penalty)
		updated := model.exploreLeaf(pathKey, action, symbol)
		model.noteExploreThreshold(out, pathKey, action, leafThreshold(updated))
	}
}

func (model *Model) exploreLeafAt(pathKey, action string) (*data.Value, bool) {
	if model == nil || model.explore == nil || pathKey == "" || action == "" {
		return nil, false
	}
	leaf, ok := model.explore.Get(exploreRadixKey(pathKey, action))
	if !ok || leaf == nil {
		return nil, false
	}
	return leaf, true
}

func (model *Model) noteExploreThreshold(
	out *data.Value, pathKey, action string, threshold float64,
) {
	if out == nil || pathKey == "" {
		return
	}
	suffix := "enter.bin"
	if action == "exit" {
		suffix = "exit.bin"
	}
	key := "model/explore/" + pathKey + "/" + suffix

	var prior map[string]float64
	out.Lookup(&prior, "exploreThresholds")
	next := map[string]float64{}
	for k, v := range prior {
		next[k] = v
	}
	next[key] = threshold
	out.Append(next, "exploreThresholds")
}

func (model *Model) exploreEnterAt(pathKey string) (*data.Value, bool) {
	if model == nil || model.explore == nil || pathKey == "" {
		return nil, false
	}
	leaf, ok := model.explore.Get(exploreRadixKey(pathKey, "enter"))
	if !ok || leaf == nil {
		return nil, false
	}
	return leaf, true
}

func (model *Model) policyEnterAt(pathKey string) (*data.Value, bool) {
	if model == nil || model.policy == nil || pathKey == "" {
		return nil, false
	}
	leaf, ok := model.policy.Get([]byte(pathKey))
	if !ok || leaf == nil {
		return nil, false
	}
	var action string
	leaf.Lookup(&action, "action")
	if action != "enter" {
		return nil, false
	}
	return leaf, true
}

func (model *Model) tryExploreStoredEnter(
	out *data.Value, symbol, pathKey string,
) bool {
	if _, ok := model.exploreEnterAt(pathKey); !ok {
		return false
	}
	if err := model.desk.Enter(symbol, pathKey, true, true); err != nil {
		return false
	}
	model.path.Reset(symbol)
	if model.resonance != nil {
		model.resonance.ForgetHold(symbol)
	}
	if out != nil {
		out.Append("enter submitted", "outcome")
	}
	return true
}

func (model *Model) tryPolicyStoredEnter(
	out *data.Value,
	tick *data.Value,
	symbol, pathKey string,
	observed []*nmstore.Region,
	frictionPass, frictionChecked bool,
) bool {
	if _, ok := model.policyEnterAt(pathKey); !ok {
		return false
	}
	if frictionChecked && !frictionPass {
		if out != nil {
			out.Append("friction", "blockedReason")
		}
		return false
	}
	if model.resonance != nil && !model.resonance.AllowsEntry(tick, symbol, observed) {
		if out != nil {
			out.Append("resonance veto", "blockedReason")
		}
		return false
	}
	if !model.paperRoom() {
		return false
	}
	if err := model.desk.Enter(symbol, pathKey, false, true); err != nil {
		return false
	}
	model.path.Reset(symbol)
	if model.resonance != nil {
		model.resonance.ForgetHold(symbol)
	}
	return true
}

// matchStoredPaths prefers policy leaves, then explore (stored exit while holding).
func (model *Model) matchStoredPaths(
	pathKey string,
	enter, holdingPaper, holdingVirtual bool,
) (paper bool, values map[string]*data.Value, disagree bool) {
	values = make(map[string]*data.Value)

	if pathKey == "" || model == nil {
		return false, values, false
	}

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

			switch stored {
			case "enter":
				if !enter {
					disagree = true
					return true
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
			return onPaper[i], values, false
		}
	}

	return false, values, disagree
}

// exploreNewPath opens a virtual lot when no explore enter leaf exists at pathKey.
func (model *Model) exploreNewPath(symbol, pathKey string) bool {
	if pathKey == "" {
		return false
	}
	if _, ok := model.exploreEnterAt(pathKey); ok {
		return false
	}
	if err := model.desk.Enter(symbol, pathKey, true, false); err != nil {
		return false
	}
	model.path.Reset(symbol)
	return true
}

// finishVirtualExplore adjusts thresholds, prunes/promotes, persists both paths (3–5).
func (model *Model) finishVirtualExplore(
	position *broker.Position, out *data.Value,
) bool {
	if model == nil || position == nil {
		return false
	}

	if !position.TrySettleExplore() {
		return true
	}

	model.adjustVirtualPair(position)
	model.reconcileVirtualPair(position)
	written := model.writePaths(position)
	model.publishExplorePairThresholds(out, position)
	model.refreshExploreUI()
	if model.stage != nil {
		model.stage.PersistSequencer(model.Context(), position.Symbol)
	}
	return written
}

func (model *Model) adjustVirtualPair(position *broker.Position) {
	step := explorePnLStep(position)
	if position.EntryPath != "" {
		model.bumpExploreLeaf(position.Symbol, position.EntryPath, "enter", step)
	}
	if position.ExitPath != "" {
		model.bumpExploreLeaf(position.Symbol, position.ExitPath, "exit", step)
	}
}

func (model *Model) reconcileVirtualPair(position *broker.Position) {
	entryKey := position.EntryPath
	exitKey := position.ExitPath
	if entryKey == "" && exitKey == "" {
		return
	}

	entryT := leafThreshold(model.exploreLeaf(entryKey, "enter", position.Symbol))
	exitT := leafThreshold(model.exploreLeaf(exitKey, "exit", position.Symbol))

	if entryT > 0.8 || exitT > 0.8 {
		model.pruneExplorePair(entryKey, exitKey)
		return
	}

	if (entryKey != "" && entryT < 0.2) || (exitKey != "" && exitT < 0.2) {
		model.promoteExplorePair(entryKey, exitKey, position.Symbol)
	}
}

func (model *Model) pruneExplorePair(entryKey, exitKey string) {
	for _, spec := range []struct{ key, action string }{
		{entryKey, "enter"},
		{exitKey, "exit"},
	} {
		if spec.key == "" {
			continue
		}
		model.store.DeletePrefix("model/explore/" + spec.key + "/")
		model.explore, _ = model.explore.DeletePrefix([]byte(spec.key))
	}
}

func (model *Model) promoteExplorePair(entryKey, exitKey, symbol string) {
	for _, spec := range []struct{ key, action string }{
		{entryKey, "enter"},
		{exitKey, "exit"},
	} {
		if spec.key == "" {
			continue
		}
		leaf := model.exploreLeaf(spec.key, spec.action, symbol)
		if leaf == nil {
			continue
		}

		suffix := "enter.bin"
		if spec.action == "exit" {
			suffix = "exit.bin"
		}

		model.store.WriteAtPrefix("model/policy/"+spec.key+"/"+suffix, leaf.Bytes())
		model.policy, _, _ = model.policy.Insert([]byte(spec.key), leaf)
		model.store.DeletePrefix("model/explore/" + spec.key + "/")
		model.explore, _ = model.explore.DeletePrefix([]byte(spec.key))
	}
}

func (model *Model) bumpExploreLeaf(symbol, pathKey, action string, step float64) {
	if model.onPolicy(pathKey) {
		return
	}
	leaf := model.exploreLeaf(pathKey, action, symbol)
	leaf.Append(step, "threshold")
	model.explore, _, _ = model.explore.Insert(exploreRadixKey(pathKey, action), leaf)
	model.store.WriteAtPrefix("model/explore/"+pathKey+"/"+action+".bin", leaf.Bytes())
}

func (model *Model) publishExplorePairThresholds(
	out *data.Value, position *broker.Position,
) {
	if position == nil {
		return
	}
	if position.EntryPath != "" {
		leaf := model.exploreLeaf(position.EntryPath, "enter", position.Symbol)
		model.noteExploreThreshold(out, position.EntryPath, "enter", leafThreshold(leaf))
	}
	if position.ExitPath != "" {
		leaf := model.exploreLeaf(position.ExitPath, "exit", position.Symbol)
		model.noteExploreThreshold(out, position.ExitPath, "exit", leafThreshold(leaf))
	}
}

func (model *Model) refreshExploreUI() {
	if ui.UI != nil && ui.UI.Active() {
		ui.UI.ListPrefix("model/explore")
	}
}

func (model *Model) exploreLeaf(pathKey, action, symbol string) *data.Value {
	if model == nil || pathKey == "" || action == "" {
		return nil
	}

	if leaf, ok := model.explore.Get(exploreRadixKey(pathKey, action)); ok && leaf != nil {
		return leaf
	}

	if leaf, ok := model.explore.Get([]byte(pathKey)); ok && leaf != nil {
		var stored string
		leaf.Lookup(&stored, "action")
		if stored == action || stored == "" {
			return leaf
		}
	}

	storeKey := "model/explore/" + pathKey + "/" + action + ".bin"
	if model.store != nil && model.System != nil {
		if leaf := model.store.Download(model.Context(), storeKey); leaf != nil {
			return leaf
		}
	}

	return data.NewValue("model", "explore", symbol).Append(action, "action")
}

func exploreTemporalPenalty() float64 {
	if system.Cfg != nil && system.Cfg.Learning != nil {
		return system.Cfg.Learning.ExploreTemporalPenalty
	}
	return 0
}

func leafThreshold(leaf *data.Value) float64 {
	if leaf == nil {
		return system.Cfg.Learning.ThresholdStart
	}

	threshold := system.Cfg.Learning.ThresholdStart

	for idx, index := range leaf.Index {
		if strings.Contains(index, "threshold") {
			var v float64
			leaf.At(&v, idx)
			threshold += v
		}
	}

	return threshold
}

func explorePnLStep(position *broker.Position) float64 {
	win := position != nil && position.VirtualExploreWin()

	if system.Cfg != nil && system.Cfg.Learning != nil && system.Cfg.Learning.ThresholdStep > 0 {
		step := system.Cfg.Learning.ThresholdStep
		if win {
			return -step
		}
		return step
	}

	if win {
		return -0.05
	}
	return 0.05
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
		leaf := model.exploreLeaf(position.EntryPath, "enter", position.Symbol)
		model.saveExploreLeaf(position.EntryPath, "enter", leaf)
	}
	if position.ExitPath != "" {
		leaf := model.exploreLeaf(position.ExitPath, "exit", position.Symbol)
		model.saveExploreLeaf(position.ExitPath, "exit", leaf)
	}

	model.path.Reset(position.Symbol)
	return true
}

func (model *Model) saveExploreLeaf(pathKey, action string, leaf *data.Value) {
	if leaf == nil || model.onPolicy(pathKey) {
		return
	}
	model.explore, _, _ = model.explore.Insert(exploreRadixKey(pathKey, action), leaf)
	model.store.WriteAtPrefix("model/explore/"+pathKey+"/"+action+".bin", leaf.Bytes())
}

func (model *Model) tryMomentumExit(
	out *data.Value,
	tick *data.Value,
	symbol string,
	pathKey string,
	virtual bool,
) bool {
	position := model.desk.Position(symbol, virtual)
	if position == nil || tick == nil {
		return false
	}

	trigger, shouldExit := model.momentum.trigger(model.resonance, tick, symbol, position)
	if !shouldExit {
		model.momentum.advance(model.resonance, tick, symbol, position)
		return false
	}

	exitPath := model.emergencyExitPath(symbol, pathKey, position)
	position.AppendOutcomeTrigger(trigger)

	if err := model.desk.Exit(symbol, exitPath, virtual); err != nil {
		return false
	}

	model.momentum.forget(position)
	model.resonance.ForgetHold(symbol)

	if model.finishVirtualExplore(position, out) {
		model.stage.ResetSequencer(symbol)
		if out != nil {
			out.Append(trigger, "outcome")
			out.Append("exit submitted", "outcome")
		}
	}

	return true
}

func (model *Model) sweepMomentum(tick *data.Value) {
	if tick == nil {
		return
	}

	for _, position := range model.desk.OpenPositions(true) {
		if position == nil || position.Symbol == "" {
			continue
		}

		pathKey := position.EntryPath
		if pathKey == "" {
			pathKey = model.path.PolicyLive(position.Symbol, false)
		}

		model.tryMomentumExit(nil, tick, position.Symbol, pathKey, true)
	}
}

func (model *Model) RecordExploreClose(position *broker.Position) bool {
	if position == nil {
		return false
	}
	if position.Virtual {
		return model.finishVirtualExplore(position, nil)
	}
	return model.writePaths(position)
}

func (model *Model) DeskExitPath(symbol string, position *broker.Position) string {
	if position == nil {
		return ""
	}
	return model.emergencyExitPath(symbol, "", position)
}

func (model *Model) emergencyExitPath(
	symbol string,
	pathKey string,
	position *broker.Position,
) string {
	if position == nil {
		return pathKey
	}

	if pathKey != "" && pathKey != position.EntryPath {
		return pathKey
	}

	if trigger := model.path.TriggerLive(symbol); trigger != "" {
		return trigger
	}

	if live := model.path.Live(symbol); live != "" && live != position.EntryPath {
		return live
	}

	if position.EntryPath != "" {
		return position.EntryPath + "/R03"
	}

	return pathKey
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

func (model *Model) onPolicy(pathKey string) bool {
	if model == nil || model.policy == nil || pathKey == "" {
		return false
	}
	_, ok := model.policy.Get([]byte(pathKey))
	return ok
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

		value := model.store.Download(model.Context(), objectKey)
		if value == nil {
			continue
		}

		var stored string
		value.Lookup(&stored, "action")
		if stored == "" {
			value.Append(action, "action")
		}

		model.explore, _, _ = model.explore.Insert(exploreRadixKey(pathKey, action), value)
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
