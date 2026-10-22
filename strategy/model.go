package strategy

import (
	"context"
	"io"
	"strings"
	"sync"

	iradix "github.com/hashicorp/go-immutable-radix/v2"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/strategy/tokenizer"
)

const (
	policyPrefix = "model/current/"
	enterSuffix  = "/enter.bin"
	exitSuffix   = "/exit.bin"
)

const (
	thresholdStart = 0.5
	thresholdStep  = 0.05
	thresholdFloor = 0.05
	thresholdCeil  = 0.95
	virtualPromote = 3
)

/*
Model is the energy gate for a token path. Virtual positions explore the
market: every profitable close stores its path as an explored value with
the close's action, and strengthens it (threshold down, win up); a loss
weakens it (threshold up). Three wins promote the path — its enter and exit
halves upload under model/current — and paper trades the promoted action
from then on. Open lots live on the desk cache alone, and each carries its
path.
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
	settled         map[string]bool
	pending         []*data.Value
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
		settled:    make(map[string]bool),
		pending:    make([]*data.Value, 0),
	}

	for prefix := range model.store.ListKeys("model/current") {
		value := model.store.Download(ctx, prefix)

		if value == nil {
			continue
		}

		model.tree, _, _ = model.tree.Insert([]byte(prefix), value)
	}

	return model
}

/*
SequencerFor returns the symbol's grammar, created on first use. Each tick's
region tokens become one chord symbol and the grammar grows from it.
*/
func (model *Model) SequencerFor(symbol string) *tokenizer.Sequencer {
	sequencer, _ := model.sequencers.LoadOrStore(symbol, tokenizer.NewSequencer())
	return sequencer.(*tokenizer.Sequencer)
}

/*
Sentence is the live path for the symbol: the production induced on the
latest tick flattened to region keys, else the tick's own chord of regions.
*/
func (model *Model) Sentence(symbol string, tokens Tokens) string {
	if len(tokens) == 0 {
		return ""
	}

	sequencer := model.SequencerFor(symbol)

	rule, _, _ := sequencer.AnalyzeTokens(tokens)

	if names := sequencer.Grammar().TerminalNames(rule); len(names) > 0 {
		return names[0]
	}

	return tokenizer.Chord(tokens)
}

/*
Evaluate drives paper trades from the promoted tree: a stored path acts only
when the current sentence agrees with it — enter opens paper, exit closes.
Virtual exploration is not decided here; Explore runs it every tick.
*/
func (model *Model) Evaluate(symbol string, sentence string) *data.Value {
	action, found := model.tree.Root().Get(pathOf(symbol, sentence))

	if !found {
		return nil
	}

	match := data.NewValue("training", action.Scope, symbol).Append(
		tokenizer.Tokens{tokenizer.NewToken(
			strings.Split(sentence, "/"), 0, 0,
		)}, "tokens", symbol, sentence,
	)

	switch action.Scope {
	case "enter":
		if !model.desk.Has(symbol, false) {
			if err := model.desk.Enter(symbol, sentence, false); err != nil {
				model.Error(err)
			}
		}
	case "exit":
		if model.desk.Has(symbol, false) {
			if err := model.desk.Exit(symbol, sentence, false); err != nil {
				model.Error(err)
			}
		}
	}

	return match
}

/*
Explore is one pass of the virtual path explorer per symbol. A sentence that
is not yet explored enters a virtual immediately, and an open virtual whose
band walk is over exits; the desk settles the close.
*/
func (model *Model) Explore(symbol string, sentence string) {
	if sentence == "" {
		return
	}

	position := model.desk.Virtual(symbol)

	if position != nil {
		if position.ShouldExit() {
			if err := model.desk.Exit(symbol, sentence, true); err != nil {
				model.Error(err)
			}
		}

		return
	}

	if _, found := model.explored.Root().Get(pathOf(symbol, sentence)); found {
		return
	}

	if err := model.desk.Enter(symbol, sentence, true); err != nil {
		model.Error(err)
	}
}

/*
Train settles every virtual the desk closed since the last call into
frame, publishes each newly promoted path (origin training so the learning
route carries it), and uploads each promoted path once.
*/
func (model *Model) Train(frame *data.Value) {
	for _, position := range model.desk.ClosedVirtuals() {
		model.settle(position, frame)
		model.desk.Drop(position.Symbol)
	}

	model.mu.Lock()
	pending := model.pending
	model.pending = make([]*data.Value, 0)
	model.mu.Unlock()

	for _, value := range pending {
		if _, err := model.hub.Write(value.Bytes()); err != nil {
			model.Error(err)
		}
	}
}

/*
settle applies one closed virtual to its path in the explored tree. A new
path is stored with the close's action; an existing path is strengthened on
profit or weakened on loss. Three wins promote the path to the policy tree,
upload its enter/exit halves, and queue it for the hub.
*/
func (model *Model) settle(position *broker.Position, frame *data.Value) {
	if position.NetReturn == nil || position.EntryPath == "" {
		return
	}

	model.mu.Lock()
	defer model.mu.Unlock()

	if model.settled[position.ID] {
		return
	}

	model.settled[position.ID] = true

	key := pathOf(position.Symbol, position.EntryPath)

	stored, found := model.explored.Root().Get(key)

	threshold, wins := thresholdStart, 0

	if found {
		var tokens Tokens
		stored.Lookup(&tokens, "tokens")

		if len(tokens) > 0 {
			threshold = tokens[len(tokens)-1].Threshold
			wins = int(tokens[len(tokens)-1].Brightness)
		}
	}

	scope := "exit"

	if *position.NetReturn > 0 {
		scope = "enter"
		wins++
		threshold -= thresholdStep
	} else {
		threshold += thresholdStep
	}

	if threshold < thresholdFloor {
		threshold = thresholdFloor
	}

	if threshold > thresholdCeil {
		threshold = thresholdCeil
	}

	token := tokenizer.NewToken(
		strings.Split(position.EntryPath, "/"), threshold, float64(wins),
	)

	value := data.NewValue("training", scope, position.Symbol).
		Append(Tokens{token}, "tokens", position.Symbol, position.EntryPath).
		Append(position.NetReturn, "outcome", position.Symbol, position.EntryPath)

	model.explored, _, _ = model.explored.Insert(key, value)

	frame.Append(value, "train", "explored", position.Symbol, position.EntryPath)

	if wins >= virtualPromote {
		model.tree, _, _ = model.tree.Insert(key, value)
		value.Append([]string{scope}, "promoted", position.Symbol, position.EntryPath)
		model.persist(key, value)
		model.pending = append(model.pending, value)
	}
}

/*
persist uploads the path's canonical enter/exit objects so the model
survives a restart.
*/
func (model *Model) persist(key []byte, value *data.Value) {
	for _, suffix := range []string{enterSuffix, exitSuffix} {
		if _, err := model.store.WriteAtPrefix(
			string(key)+suffix, value.Bytes(),
		); err != nil {
			model.Error(err)
		}
	}
}

func pathOf(symbol string, sentence string) []byte {
	return []byte(policyPrefix + symbol + "/" + sentence)
}
