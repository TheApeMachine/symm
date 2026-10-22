package strategy

import (
	"context"
	"fmt"
	"io"
	"slices"
	"strings"

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
		_ = symbol
		regions := make([]*Token, 0)
		observed, err := training.grid.Observe(value)

		if err != nil {
			training.Error(err)
			continue
		}

		for _, region := range observed {
			regions = append(regions, tokenizer.NewToken(
				[]string{fmt.Sprintf("R%02d", region.Key)},
				region.Brightness,
				region.Brightness,
			))
		}

		training.model.Evaluate(value)
	}

	return value
}
