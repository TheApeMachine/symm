package strategy

import (
	"context"
	"fmt"
	"io"
	"slices"
	"strings"
	"sync"

	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	nmstore "github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/strategy/tokenizer"
	"github.com/theapemachine/symm/types"
)

type (
	Token  = tokenizer.Token
	Tokens = tokenizer.Tokens
)

/*
key is a token's region address: its first path element (R08, or a chord
production's head). String() already renders the full path.
*/
func key(token *Token) string {
	if len(token.Path) == 0 {
		return ""
	}

	return token.Path[0]
}

/*
Training paper trades the live market against the action tree the Trainer
published: every frame becomes a region token, the token advances the
symbol's grammar through Sequitur, and when an induced rule matches a stored
path with an agreed action, that action is taken through the Desk.
*/
type Training struct {
	*runtime.System
	model  *Model
	price  *broker.Price
	desk   *broker.Desk
	client *store.Client
	grid   *nmstore.Grid
	hub    io.Writer

	/*
	last holds each symbol's previous frame. Ticks are sparse: a symbol the
	grid did not observe this tick has nothing new to say, and re-sending
	the previous frame keeps its sentence and region rings alive on the
	dashboard instead of letting them vanish and reappear.
	*/
	mu   sync.Mutex
	last map[string]*data.Value
}

func NewTraining(
	ctx context.Context,
	price *broker.Price,
	desk *broker.Desk,
	client *store.Client,
	hub io.Writer,
) *Training {
	training := &Training{
		System: runtime.NewSystem(ctx, "training", price),
		model:  NewModel(ctx, desk, client, hub),
		price:  price,
		desk:   desk,
		client: client,
		grid:   nmstore.NewGrid(ctx),
		hub:    hub,
		last:   make(map[string]*data.Value),
	}

	return training
}

func (training *Training) Step(value *data.Value) *data.Value {
	if value == nil {
		return nil
	}

	grouped := make(map[string]bool)

	for _, idx := range value.Index {
		rest := idx

		if value.Prefix != "" && strings.HasPrefix(idx, value.Prefix+"/") {
			rest = strings.TrimPrefix(idx, value.Prefix+"/")
		}

		parts := strings.Split(rest, "/")

		// A signal layer key is source/symbol/label..., e.g. "cvd/BTC/USD/midpoint".
		// Only keys from known signal sources with a fully-qualified symbol
		// (base/quote) are tradable; skip spot:trade, train/*, and anything else.
		if len(parts) < 3 {
			continue
		}

		source := parts[0]
		isSignal := slices.Contains(types.SignalSourceStrings, source)

		if !isSignal {
			continue
		}

		symbol := parts[1] + "/" + parts[2]
		grouped[symbol] = true
	}

	for symbol := range grouped {
		frame := data.NewValue("training", "tick", symbol)

		observed, err := training.grid.Observe(value)

		if err != nil {
			training.Error(err)
			continue
		}

		regions := make(Tokens, 0, len(observed))

		for _, region := range observed {
			regions = append(regions, tokenizer.NewToken(
				[]string{fmt.Sprintf("R%02d", region.Key)},
				region.Brightness,
				region.Brightness,
			))
		}

		/*
			train/regions/<symbol>/<R##> carries brightness per accepted
			region; train/regions/<symbol>/tokens carries the live sentence.
		*/
		for _, region := range regions {
			frame.Append(
				[]float64{region.Brightness}, "train", "regions", symbol, key(region),
			)
		}

		sentence := training.model.Sentence(symbol, regions)

		frame.Append(
			Tokens{tokenizer.NewToken(
				strings.Split(sentence, "/"), 0, 0,
			)}, "train", "regions", symbol, "tokens",
		)

		if match := training.model.Evaluate(symbol, sentence); match != nil {
			frame.Append(match, "train", "match", symbol, sentence)
		}

		training.model.Explore(symbol, sentence)

		training.model.Train(frame)

		training.mu.Lock()
		training.last[symbol] = frame
		training.mu.Unlock()

		value.Append(frame, "training")
	}

	/*
	Symbols with no signal in this frame get their previous frame back:
	their sentence has not changed, and an absent training origin would
	clear their token strip and region ring on the dashboard until the next
	sparse tick happened to mention them.
	*/
	training.mu.Lock()
	stale := make([]string, 0)

	for symbol := range training.last {
		if !grouped[symbol] {
			stale = append(stale, symbol)
		}
	}

	previous := make([]*data.Value, 0, len(stale))

	for _, symbol := range stale {
		previous = append(previous, training.last[symbol])
	}

	training.mu.Unlock()

	for _, frame := range previous {
		value.Append(frame, "training")
	}

	return value
}
