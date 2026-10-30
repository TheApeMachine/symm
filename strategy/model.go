package strategy

import (
	"context"
	"strings"
	"sync/atomic"

	"github.com/theapemachine/nomagique/data"
	"github.com/theapemachine/nomagique/runtime"
	nmstore "github.com/theapemachine/nomagique/store"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/strategy/tokenizer"
	"github.com/theapemachine/symm/system"
)

type Model struct {
	*runtime.System
	desk  *broker.Desk
	price *broker.Price
	store *store.Client
	tree  *atomic.Pointer[Tree]
}

func NewModel(
	ctx context.Context,
	desk *broker.Desk,
	price *broker.Price,
) *Model {
	client := store.NewClient(ctx, system.Cfg.Storage.S3.Bucket)

	model := &Model{
		System: runtime.NewSystem(ctx, "model"),
		desk:   desk,
		price:  price,
		store:  client,
		tree:   &atomic.Pointer[Tree]{},
	}

	tree := NewTree(ctx, "model/paths/")
	model.tree.Store(tree)

	return model
}

func (model *Model) Evaluate(
	out *data.Value, observed []*nmstore.Region, tick *data.Value,
) *data.Value {
	if out == nil {
		return nil
	}

	symbol := out.Scope

	if tick == nil {
		return out
	}

	coder := ReadCoder(tick, symbol)

	if !coder.Ready {
		return out
	}

	out.Append(observed, "lit")

	// First we handle the positions that were exited by the stoploss.
	if pending, ok := model.desk.TakePending(symbol); ok {
		model.Exit(pending)
	}

	position := model.desk.Position(symbol)

	tokens := data.Get[tokenizer.Tokens](
		tick, strings.Join([]string{"stage", symbol, "cut"}, "/"),
	)

	out.Append(tokens, "cut")

	action, strength, disagreed := model.tree.Load().Predict(tokens)

	if disagreed {
		return out
	}

	var phase string
	tick.Lookup(&phase, strings.Join([]string{"stage", symbol, "phase"}, "/"))

	switch action {
	case ActionEnter:
		if !coder.Bullish || position != nil {
			break
		}

		paper := strength > 0.2
		model.Enter(symbol, tokens.String(), !paper, paper)
	case ActionHold:
		model.Hold(position, tokens.String())
	case ActionExit:
		if coder.Bullish {
			break
		}

		model.Exit(position)
	case ActionNone:
		if !coder.Bullish || position != nil || phase != "enter" || tokens.Len() == 0 {
			break
		}

		model.Enter(symbol, tokens.String(), true, false)
	}

	return out
}

/*
Enter enters a new position.
*/
func (model *Model) Enter(
	symbol string,
	path string,
	virtual bool,
	pathFound bool,
) {
	model.desk.Enter(symbol, path, virtual, pathFound)
}

/*
Hold holds a position.
*/
func (model *Model) Hold(position *broker.Position, path string) {
	if position == nil {
		return
	}

	position.HoldPaths = append(position.HoldPaths, path)
}

/*
Exit writes the paths to the tree, and the S3 store.
Only once a position has exited should we write the paths, so we are capturing
life-cycles, and can accurately update the resistance of the paths.
*/
func (model *Model) Exit(
	position *broker.Position,
) {
	if position == nil {
		return
	}

	tree := model.tree.Load()

	if tree == nil {
		return
	}

	if position.Virtual {
		tree.Insert(
			position.EntryPath,
			data.NewValue("model", "action", "enter"),
		)

		for _, path := range position.HoldPaths {
			tree.Insert(path, data.NewValue("model", "action", "hold"))
		}

		tree.Insert(
			position.ExitPath,
			data.NewValue("model", "action", "exit"),
		)

		return
	}

	resistance := 0.05

	if position.PnL.Sign() > 0 {
		resistance = -0.05
	}

	tree.Update(position.EntryPath, resistance)

	for _, path := range position.HoldPaths {
		tree.Update(path, resistance)
	}

	tree.Update(position.ExitPath, resistance)
}
