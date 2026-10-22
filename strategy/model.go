package strategy

import (
	"context"
	"io"
	"sync"

	iradix "github.com/hashicorp/go-immutable-radix/v2"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/strategy/tokenizer"
)

/*
Model is a predictive radix trie, or prefix-based policy. A prefix is made up
of region tokens. Virtual positions explore the market: every profitable close
stores its path as an explored value with the close's action, and strengthens
it (threshold down, win up); a loss weakens it (threshold up). Three wins
promote the path — its enter and exit halves upload under model/current — and
paper trades the promoted action from then on. Open lots live on the desk
cache alone, and each carries its path.
*/
type Model struct {
	*runtime.System `json:"-"`
	mu              sync.Mutex
	desk            *broker.Desk
	store           *store.Client
	hub             io.Writer
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
		hub:        hub,
		tree:       iradix.New[*data.Value](),
		explored:   iradix.New[*data.Value](),
		sequencers: &sync.Map{},
	}

	for prefix := range model.store.ListKeys("model/current") {
		value := model.store.Download(ctx, prefix)

		if value == nil {
			continue
		}

		errnie.Debug(
			"[model.NewModel]", "prefix", prefix, "value", value,
		)

		model.tree, _, _ = model.tree.Insert([]byte(prefix), value)
	}

	return model
}

/*
Evaluate a tokenized path against the model's current storage prefix.
To make the prefixes predictive we do not just check for a full match,
but we look at all the branches that a given prefix lights up. If all
those branches agree on the same action, we take that action.
*/
func (model *Model) Evaluate(value *data.Value) *data.Value {
	if value == nil {
		return nil
	}

	var tokens tokenizer.Tokens
	value.Lookup(&tokens, "tokens")

	untyped, _ := model.sequencers.LoadOrStore(value.Scope, tokenizer.NewSequencer())
	sequencer := untyped.(*tokenizer.Sequencer)

	sequencer.Analyze(tokens)

	matches := model.tree.Root().PathIterator([]byte(value.Prefix))
	_, value, ok := matches.Next()

	if !ok {
		return nil
	}

	return value
}

/*
Train writes a new path to the model's current storage prefix, which will
make that path available for paper/real trading.

The Value's prefix must be set to:

Origin : training
Role   : model
Scope  : enter/exit/noop

The Layers must contain the thresholds.
*/
func (model *Model) Train(value *data.Value) {
	if value == nil {
		return
	}

	model.tree, _, _ = model.tree.Insert([]byte(value.Prefix), value)
}
