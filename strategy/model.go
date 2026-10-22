package strategy

import (
	"context"
	"io"
	"sync"

	iradix "github.com/hashicorp/go-immutable-radix/v2"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/strategy/tokenizer"
)

const thresholdKey = "model/state/current"

const (
	thresholdStart = 0.5
	thresholdStep  = 0.05
	thresholdFloor = 0.05
	thresholdCeil  = 0.95
	virtualPromote = 3
)

/*
Model is the energy gate for a token path. A region matches a path only when
its brightness clears that path's threshold. A profitable exit lowers the
threshold and a loss raises it. Three profitable virtual closes store the
path as enter.bin and exit.bin. Threshold, Wins, and Promoted are the stored
model; open lots live on the desk cache alone, and each carries its path.
*/
type Model struct {
	*runtime.System `json:"-"`
	mu              sync.Mutex
	desk            *broker.Desk
	store           *store.Client
	tree            *iradix.Tree[*data.Value]
	explored        *iradix.Tree[*data.Value]
	sequencers      *sync.Map
}

func NewModel(
	ctx context.Context, desk *broker.Desk, client *store.Client,
	hub io.Writer,
) *Model {
	model := &Model{
		System:     runtime.NewSystem(ctx, "model"),
		desk:       desk,
		store:      client,
		tree:       iradix.New[*data.Value](),
		explored:   iradix.New[*data.Value](),
		sequencers: &sync.Map{},
	}

	for prefix := range model.store.ListKeys("model/current") {
		value := model.store.Download(ctx, prefix)

		if value == nil {
			continue
		}

		model.tree.Insert([]byte(prefix), value)
	}

	return model
}

func (model *Model) Evaluate(value *data.Value) {
	var tokens tokenizer.Tokens
	value.Lookup(&tokens, "tokens")

	for _, tree := range []*iradix.Tree[*data.Value]{model.tree, model.explored} {
		action := model.Search(tree, []byte("model/current/"+tokens.String()))

		if action != nil {
			break
		}

		if action == nil {
			return
		}

		switch action.Scope {
		case "enter":
			if !model.desk.Has(value.Scope, true) {
				model.desk.Enter(value.Scope, tokens.String(), true)
			}
		case "exit":
			if model.desk.Has(value.Scope, true) {
				model.desk.Exit(value.Scope, tokens.String(), true)
			}
		}
	}
}

func (model *Model) Train() {
}

/*
Search returns an action scoped Value, but only if all Values found for
the current path agree on the action. This makes the model less deterministic,
and more predictive.
*/
func (model *Model) Search(tree *iradix.Tree[*data.Value], path []byte) *data.Value {
	var current *data.Value

	for _, node, _ := tree.Root().PathIterator(
		path,
	).Next(); node != nil; _, node, _ = tree.Root().PathIterator(
		path,
	).Next() {
		if current == nil || node.Scope == current.Scope {
			current = node
		}

		current = nil
		break
	}

	return current
}
