package strategy

import (
	"context"
	"fmt"
	"strings"
	"sync"

	iradix "github.com/hashicorp/go-immutable-radix/v2"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	nmstore "github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/strategy/tokenizer"
	"github.com/theapemachine/symm/system"
)

type Model struct {
	*runtime.System
	mu         sync.RWMutex
	desk       *broker.Desk
	store      *store.Client
	sequencers *sync.Map
	path       *Path
	policy     *iradix.Tree[*data.Value]
	explore    *iradix.Tree[*data.Value]
}

func NewModel(
	ctx context.Context,
	desk *broker.Desk,
	client *store.Client,
) *Model {
	if client == nil {
		client = store.NewClient(ctx, system.Cfg.Storage.S3.Bucket)
	}

	model := &Model{
		System:     runtime.NewSystem(ctx, "model"),
		desk:       desk,
		store:      client,
		sequencers: &sync.Map{},
		path:       NewPath(),
		policy:     iradix.New[*data.Value](),
		explore:    iradix.New[*data.Value](),
	}

	if client != nil {
		// Restore explore and policy tries from S3 storage
		for _, role := range []string{"policy", "explore"} {
			prefix := fmt.Sprintf("model/%s/", role)
			tree := model.explore
			if role == "policy" {
				tree = model.policy
			}

			// Gap 6: ListKeys yields the actual S3 object keys
			for s3Key := range model.store.ListKeys(prefix) {
				val := model.store.Download(ctx, s3Key)
				if val == nil {
					continue
				}

				// Extract pathKey cleanly from S3 key if val.Scope is empty
				pathKey := val.Scope
				if pathKey == "" {
					rel := strings.TrimPrefix(s3Key, prefix)
					rel = strings.TrimSuffix(rel, "/enter.bin")
					rel = strings.TrimSuffix(rel, "/exit.bin")
					pathKey = rel
				}

				if pathKey != "" {
					tree, _, _ = tree.Insert([]byte(pathKey), val)
				}
			}

			if role == "policy" {
				model.policy = tree
			} else {
				model.explore = tree
			}
		}
	}

	return model
}

func (model *Model) Evaluate(
	out *data.Value,
	observed []*nmstore.Region,
	tick *data.Value, // Gap 1: Workspace tick containing solver/resonance layers
) *data.Value {
	symbol := out.Scope
	enter := !model.desk.Has(symbol, true)

	// Gap 1: Read coder layers from workspace tick if provided, fallback to out
	source := out

	if tick != nil {
		source = tick
	}
	
	coder := ReadCoder(source, symbol)

	untypedSequencer, ok := model.sequencers.Load(symbol)
	if !ok {
		return out
	}
	sequencer := untypedSequencer.(*tokenizer.Sequencer)

	out.Append(observed, "lit")
	step := sequencer.Step(observed, enter)
	cut := step.Token
	out.Append(cut, "cut")

	pathKey := model.path.After(model.path.PolicyLive(symbol, enter), cut)
	if pathKey != "" {
		out.Append(pathKey, "path")
	}

	// Gap 2: Fixed inverted append (Append while flat, AppendTrigger while holding)
	if cut.String() != "" {
		if enter {
			model.path.Append(symbol, cut)
		} else {
			model.path.AppendTrigger(symbol, cut)
		}
	}

	// =========================================================================
	// 1. ENTRY EVALUATION (Flat)
	// =========================================================================
	if enter {
		// Gap 3: Cannot enter without a valid grammar path
		if pathKey == "" {
			return out
		}

		// Check if this path exists in policy or explore tries
		_, inPolicy, inExplore := model.lookupPath(pathKey)
		pathFound := inPolicy || inExplore

		// Gate 1: Predictive coder forward curve must be bullish
		if !coder.Bullish {
			return out
		}

		// Gate 2: Open virtual lot
		if err := model.desk.Enter(symbol, pathKey, true, pathFound); err != nil {
			return out
		}

		// Record entry surprise on position for cliff detection
		if pos := model.desk.Position(symbol, true); pos != nil {
			pos.OpenSurprise = coder.Surprise
		}

		model.path.Reset(symbol)
		return out
	}

	// =========================================================================
	// 2. EXIT EVALUATION (Holding)
	// =========================================================================
	position := model.desk.Position(symbol, true)
	if position == nil {
		return out
	}

	shouldExit := false

	// Trigger A: Informational Cliff (Model surprise doubled)
	if position.OpenSurprise > 0 && coder.Surprise >= 2*position.OpenSurprise {
		shouldExit = true
	}

	// Trigger B: Order-book momentum stop or reversal
	if !shouldExit {
		if _, ok := position.ShouldExit(); ok {
			shouldExit = true
		}
	}

	// Trigger C: Exit Path Matched in Radix Trie
	if !shouldExit && pathKey != "" {
		if val, inPolicy, inExplore := model.lookupPath(pathKey); inPolicy || inExplore {
			var action string
			val.Lookup(&action, "action")
			if action == "exit" {
				shouldExit = true
			}
		}
	}

	// If no exit triggered, keep holding
	if !shouldExit {
		return out
	}

	// Gap 5: Resolve exit path (pathKey is now the valid trigger path)
	exitPath := pathKey
	if exitPath == "" {
		exitPath = model.path.TriggerLive(symbol)
	}
	if exitPath == "" {
		exitPath = position.EntryPath
	}

	// Execute Exit
	if err := model.desk.Exit(symbol, exitPath, true); err != nil {
		return out
	}

	// Gap 4 & Gap 7: Unified reinforcement learning
	model.RecordExploreClose(position)

	model.path.Reset(symbol)
	return out
}

// -----------------------------------------------------------------------------
// UI & Manual Close Hooks (Booter Contract)
// -----------------------------------------------------------------------------

// Gap 7: DeskExitPath resolves the path for manual dashboard exits
func (model *Model) DeskExitPath(symbol string, position *broker.Position) string {
	if position != nil && position.ExitPath != "" {
		return position.ExitPath
	}
	live := model.path.TriggerLive(symbol)
	if live != "" {
		return live
	}
	if position != nil && position.EntryPath != "" {
		return position.EntryPath
	}
	return symbol
}

// Gap 7: RecordExploreClose updates thresholds or mints exploration paths
func (model *Model) RecordExploreClose(position *broker.Position) {
	if model == nil || position == nil {
		return
	}

	isProfitable := position.PnL != nil && position.PnL.Sign() > 0
	exitPath := position.ExitPath
	if exitPath == "" {
		exitPath = model.path.TriggerLive(position.Symbol)
	}

	if position.PathFound {
		// A. Existing path: update thresholds based on win/loss
		model.updatePathThreshold(position.EntryPath, isProfitable)
		if exitPath != "" && exitPath != position.EntryPath {
			model.updatePathThreshold(exitPath, isProfitable)
		}
	} else if isProfitable {
		// Gap 4: Only mint new exploration paths if the first run won!
		if position.EntryPath != "" {
			model.savePath("explore", position.EntryPath, "enter", 0.5)
		}
		if exitPath != "" {
			model.savePath("explore", exitPath, "exit", 0.5)
		}
	}
}

// -----------------------------------------------------------------------------
// Trie & Storage Helpers
// -----------------------------------------------------------------------------

func (model *Model) lookupPath(pathKey string) (val *data.Value, inPolicy bool, inExplore bool) {
	if pathKey == "" {
		return nil, false, false
	}

	model.mu.RLock()
	defer model.mu.RUnlock()

	key := []byte(pathKey)
	if v, ok := model.policy.Get(key); ok {
		return v, true, false
	}
	if v, ok := model.explore.Get(key); ok {
		return v, false, true
	}

	return nil, false, false
}

func (model *Model) updatePathThreshold(pathKey string, profitable bool) {
	if pathKey == "" {
		return
	}

	val, inPolicy, inExplore := model.lookupPath(pathKey)
	if !inPolicy && !inExplore {
		return
	}

	currentRole := "explore"
	if inPolicy {
		currentRole = "policy"
	}

	action := "enter"
	if val != nil {
		val.Lookup(&action, "action")
	}

	threshold := getThreshold(val)

	// Profitable lowers threshold (closer to promotion); loss raises threshold
	if profitable {
		threshold -= 0.05
	} else {
		threshold += 0.05
	}

	// Prune if threshold exceeds 0.8
	if threshold > 0.8 {
		model.deletePath(currentRole, pathKey)
		return
	}

	// Promote from explore to policy if threshold < 0.2
	if threshold < 0.2 {
		if currentRole == "explore" {
			model.deletePath("explore", pathKey)
		}
		if threshold < 0.05 {
			threshold = 0.05 // floor
		}
		model.savePath("policy", pathKey, action, threshold)
		return
	}

	// Update existing path in place
	model.savePath(currentRole, pathKey, action, threshold)
}

func (model *Model) savePath(role, pathKey, action string, threshold float64) {
	value := data.NewValue("model", role, pathKey)
	value.Append(action, "action")
	value.Append(threshold, "threshold")

	// Persist to S3
	s3Key := fmt.Sprintf("model/%s/%s/%s.bin", role, pathKey, action)
	if model.store != nil {
		model.store.WriteAtPrefix(s3Key, value.Bytes())
	}

	// Update in-memory immutable radix tree
	model.mu.Lock()
	defer model.mu.Unlock()

	key := []byte(pathKey)
	if role == "policy" {
		model.policy, _, _ = model.policy.Insert(key, value)
	} else {
		model.explore, _, _ = model.explore.Insert(key, value)
	}
}

func (model *Model) deletePath(role, pathKey string) {
	s3Prefix := fmt.Sprintf("model/%s/%s/", role, pathKey)
	if model.store != nil {
		model.store.DeletePrefix(s3Prefix)
	}

	model.mu.Lock()
	defer model.mu.Unlock()

	key := []byte(pathKey)
	if role == "policy" {
		model.policy, _, _ = model.policy.Delete(key)
	} else {
		model.explore, _, _ = model.explore.Delete(key)
	}
}

func getThreshold(val *data.Value) float64 {
	if val == nil {
		return 0.5
	}
	var threshold float64
	val.Lookup(&threshold, "threshold")
	if threshold == 0 {
		return 0.5
	}
	return threshold
}
