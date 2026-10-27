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
	desk      *broker.Desk
	store     *store.Client
	sequences *sync.Map
	tries     *sync.Map
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
		System:    runtime.NewSystem(ctx, "model"),
		desk:      desk,
		store:     client,
		sequences: &sync.Map{},
		tries:     &sync.Map{},
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
			val := model.store.Download(ctx, prefix)
			if val == nil {
				continue
			}

			chunks := strings.Split(prefix, "/")

			tree, _, _ = tree.Insert([]byte(
				strings.Join(chunks[2:len(chunks)-1], "/"),
			), val)
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

	sequences, _ := model.sequences.LoadOrStore(
		role+"/"+symbol, tokenizer.NewTokens(),
	)

	path := sequences.(tokenizer.Tokens)

	out.Append(observed, "lit")

	var cut tokenizer.Token

	if tick == nil {
		return out
	}

	tick.Lookup(&cut, strings.Join(
		[]string{"stage", symbol, "cut"}, "/",
	))

	if cut.String() == "" {
		return out
	}

	out.Append(cut, "cut")

	path = append(path, cut)
	model.sequences.Store(role+"/"+symbol, path)

	if len(path) > 0 {
		out.Append(path.String(), "path")
	}

	out.Append(path.String(), "policyPath")
	out.Append(enter, "enter")

	if pending, ok := model.desk.TakePendingTrain(symbol); ok {
		model.completeExit(out, symbol, pending)
		return out
	}

	if enter {
		// Gap 3: Cannot enter without a valid grammar path
		if path.Len() < system.Cfg.Learning.MinPathLength {
			return out
		}

		pathFound := false
		probe := []byte(path.String() + "/enter")

		for _, book := range []string{"explore", "policy"} {
			untypedTrie, ok := model.tries.Load(book)

			if !ok {
				continue
			}

			trie := untypedTrie.(*iradix.Tree[*data.Value])

			if _, _, ok := trie.Root().LongestPrefix(probe); ok {
				pathFound = true
				break
			}
		}

		if !coder.Bullish {
			return out
		}

		if err := model.desk.Enter(symbol, path.String(), true, pathFound); err != nil {
			errnie.Error(err)
			return out
		}

		out.Append("enter submitted", "outcome")

		if pos := model.desk.Position(symbol, true); pos != nil {
			pos.OpenSurprise = coder.Surprise
		}

		model.sequences.Store(role+"/"+symbol, tokenizer.NewTokens())
		return out
	}

	position := model.desk.Position(symbol, true)

	if position == nil {
		return out
	}

	position.ExitPath = path.String()
	shouldExit := false

	if position.OpenSurprise > 0 && coder.Surprise >= 2*position.OpenSurprise {
		shouldExit = true
	}

	if !shouldExit && path.Len() >= system.Cfg.Learning.MinPathLength {
		if untypedTrie, ok := model.tries.Load(role); ok {
			trie := untypedTrie.(*iradix.Tree[*data.Value])

			if _, _, ok := trie.Root().LongestPrefix([]byte(path.String() + "/exit")); ok {
				shouldExit = true
			}
		}
	}

	if shouldExit {
		if err := model.desk.Exit(symbol, path.String(), true); err != nil {
			errnie.Error(err)
			return out
		}

		if pending, ok := model.desk.TakePendingTrain(symbol); ok {
			model.completeExit(out, symbol, pending)
		}
	}

	return out
}

func (model *Model) completeExit(
	out *data.Value, symbol string, position *broker.Position,
) {
	for _, book := range []string{"explore", "policy"} {
		model.updatePathThreshold(book, position)
	}

	out.Append("exit submitted", "outcome")
	model.sequences.Store("explore/"+symbol, tokenizer.NewTokens())
	model.sequences.Store("policy/"+symbol, tokenizer.NewTokens())
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

	for _, pathKey := range []string{
		position.EntryPath, position.ExitPath,
	} {
		if pathKey == "" {
			continue
		}

		value, ok := trie.Get([]byte(pathKey))

		if !ok || value == nil {
			continue
		}

		var action string
		value.Lookup(&action, "action")

		threshold := model.getThreshold(value)

		if position.PnL != nil && position.PnL.Sign() > 0 {
			threshold -= 0.01
		} else {
			threshold += 0.01
		}

		if threshold > 0.8 {
			if model.store != nil {
				model.store.DeletePrefix(fmt.Sprintf("model/%s/%s/", role, pathKey))
			}

			trie, _, _ = trie.Delete([]byte(pathKey))
			continue
		}

		if threshold < 0.01 {
			threshold = 0.01
		}

		value.Append(action, "action")
		value.Append(threshold, "threshold")

		if threshold < 0.2 && role == "explore" {
			if model.store != nil {
				model.store.DeletePrefix(fmt.Sprintf("model/explore/%s/", pathKey))
			}

			trie, _, _ = trie.Delete([]byte(pathKey))

			if untypedPolicy, ok := model.tries.Load("policy"); ok {
				policy := untypedPolicy.(*iradix.Tree[*data.Value])
				policy, _, _ = policy.Insert([]byte(pathKey), value)
				model.tries.Store("policy", policy)
			}

			if model.store != nil {
				model.store.WriteAtPrefix(
					fmt.Sprintf("model/policy/%s/%s.bin", pathKey, action),
					value.Bytes(),
				)
			}

			continue
		}

		trie, _, _ = trie.Insert([]byte(pathKey), value)

		if model.store != nil {
			model.store.WriteAtPrefix(
				fmt.Sprintf("model/%s/%s/%s.bin", role, pathKey, action),
				value.Bytes(),
			)
		}
	}

	model.tries.Store(role, trie)
}

func (model *Model) getThreshold(value *data.Value) float64 {
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
