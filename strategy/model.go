package strategy

import (
	"context"
	"fmt"
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
	*runtime.System
	mu             sync.RWMutex
	desk           *broker.Desk
	store          *store.Client
	sequencers     *sync.Map
	sequences      *sync.Map
	tries          *sync.Map
	exploreSettled sync.Map
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
		sequences:  &sync.Map{},
		tries:      &sync.Map{},
	}

	model.tries.Store("explore", iradix.New[*data.Value]())
	model.tries.Store("policy", iradix.New[*data.Value]())

	// Restore explore and policy tries from S3 storage
	for _, key := range []string{"explore", "policy"} {
		untypedTree, ok := model.tries.Load(key)

		if !ok {
			continue
		}

		tree := untypedTree.(*iradix.Tree[*data.Value])

		for prefix := range model.store.ListKeys(fmt.Sprintf("model/%s/", key)) {
			chunks := strings.Split(prefix, "/")

			tree, _, _ = tree.Insert([]byte(
				strings.Join(chunks[2:len(chunks)-1], "/"),
			), model.store.Download(ctx, prefix))
		}

		model.tries.Store(key, tree)
	}

	return model
}

func (model *Model) Evaluate(
	out *data.Value, observed []*nmstore.Region, tick *data.Value,
) *data.Value {
	symbol := out.Scope
	role := "explore"

	if model.desk.Has(symbol, true) {
		role = "policy"
	}

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

	sequences, _ := model.sequences.LoadOrStore(role+"/"+symbol, tokenizer.NewTokens())
	path := sequences.(tokenizer.Tokens)

	out.Append(observed, "lit")
	step := sequencer.Step(observed, enter)
	cut := step.Token
	out.Append(cut, "cut")

	path = append(path, cut)

	if len(path) > 0 {
		out.Append(path.String(), "path")
	}

	out.Append(path.String(), "policyPath")
	out.Append(enter, "enter")

	// =========================================================================
	// 1. ENTRY EVALUATION (Flat)
	// =========================================================================
	if enter {
		// Gap 3: Cannot enter without a valid grammar path
		if path.Len() < system.Cfg.Learning.MinPathLength {
			return out
		}

		// Check if this path exists in policy or explore tries
		untypedTrie, ok := model.tries.Load(role)

		if !ok {
			return out
		}

		trie := untypedTrie.(*iradix.Tree[*data.Value])
		_, _, pathFound := trie.Root().LongestPrefix([]byte(path.String() + "/enter"))

		// Gate 1: Predictive coder forward curve must be bullish
		if !coder.Bullish {
			return out
		}

		// Gate 2: Open virtual lot
		if err := model.desk.Enter(symbol, path.String(), true, pathFound); err != nil {
			return out
		}

		out.Append("enter submitted", "outcome")

		// Record entry surprise on position for cliff detection
		if pos := model.desk.Position(symbol, true); pos != nil {
			pos.OpenSurprise = coder.Surprise
		}

		model.sequences.Store(role+"/"+symbol, tokenizer.NewTokens())
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
	if !shouldExit && path.Len() >= system.Cfg.Learning.MinPathLength {
		untypedTrie, ok := model.tries.Load(role)

		if !ok {
			return out
		}

		trie := untypedTrie.(*iradix.Tree[*data.Value])
		_, _, pathFound := trie.Root().LongestPrefix([]byte(path.String() + "/exit"))

		if pathFound {
			shouldExit = true
		}
	}

	// If no exit triggered, keep holding
	if !shouldExit {
		return out
	}

	if err := model.desk.Exit(symbol, path.String(), true); err != nil {
		errnie.Error(err)
		return out
	}

	model.updatePathThreshold(role, position)

	out.Append("exit submitted", "outcome")
	model.sequences.Store(role+"/"+symbol, tokenizer.NewTokens())
	return out
}

func (model *Model) updatePathThreshold(
	role string, position *broker.Position,
) {
	if position == nil {
		return
	}

	untypedTrie, ok := model.tries.Load(role)

	if !ok {
		return
	}

	trie := untypedTrie.(*iradix.Tree[*data.Value])

	for _, path := range []string{
		position.EntryPath, position.ExitPath,
	} {
		value, pathFound := trie.Get([]byte(path))

		if !pathFound || value == nil {
			return
		}

		var action string
		value.Lookup(&action, "action")

		threshold := getThreshold(value)

		// Profitable lowers threshold (closer to promotion); loss raises threshold
		if position.PnL != nil && position.PnL.Sign() > 0 {
			threshold -= 0.05
		} else {
			threshold += 0.05
		}

		// Prune if threshold exceeds 0.8
		if threshold > 0.8 {
			trie, _, _ = trie.Delete([]byte(path))
		}

		// Promote from explore to policy if threshold < 0.2
		if threshold < 0.2 {
			trie, _, _ = trie.Insert(
				[]byte(path),
				data.NewValue("model", role, path),
			)
		}

		// Update existing path in place
		value.Append(action, "action")
		value.Append(threshold, "threshold")

		trie, _, _ = trie.Insert([]byte(path), value)
	}

	model.tries.Store(role, trie)
}

func getThreshold(value *data.Value) float64 {
	if value == nil {
		return 0.5
	}

	var threshold float64
	value.Lookup(&threshold, "threshold")

	if threshold == 0 {
		return 0.5
	}

	return threshold
}
