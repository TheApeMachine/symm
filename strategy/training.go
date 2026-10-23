package strategy

import (
	"context"
	"io"

	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	nmstore "github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/strategy/tokenizer"
)

type Training struct {
	*runtime.System
	model  *Model
	price  *broker.Price
	desk   Desk
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

/*
PublishPolicy replays the current policy trie to a dashboard that just
connected, or that asked for policy/current.
*/
func (training *Training) PublishPolicy() {
	if training == nil || training.model == nil {
		return
	}

	training.model.PublishPolicy()
}

/*
Step is the per-tick entry point. It observes the grid (which emits
regions above the brightness midpoint), groups them by symbol, creates
tokens, feeds each token to the model for evaluation, and publishes the
resulting frame.

The model evaluates one token per call: each token is one MDL cut over
the grammar stream. The model accumulates tokens into a path per symbol
and checks the path against the policy tree.
*/
func (training *Training) Step(value *data.Value) *data.Value {
	if value == nil {
		return nil
	}

	observed, err := training.grid.Observe(value)
	if err != nil {
		training.Error(err)
		return nil
	}

	if len(observed) == 0 {
		return value
	}

	// Group regions by symbol: one symbol may have multiple active regions
	// in a single tick. Each region becomes one token.
	bySymbol := make(map[string][]tokenizer.Token)

	for _, region := range observed {
		token := tokenizer.NewTokenFromRegion(region.Key, region.Brightness)
		bySymbol[region.Symbol] = append(bySymbol[region.Symbol], token)
	}

	// Feed each symbol's tokens to the model. The model's Evaluate
	// accumulates them into the symbol's path and checks the trie.
	for symbol, tokens := range bySymbol {
		for _, token := range tokens {
			training.model.Evaluate(symbol, token)
		}
	}

	// Append the observed regions so the frontend's Impulse Map and
	// position-card radial charts can render them.
	value.Append(observed, "train", "regions")

	// Safety net: settle any closed virtuals that were not handled
	// inside Evaluate (e.g. restored from history on restart).
	for _, cv := range training.desk.ClosedVirtuals() {
		training.model.Settle(cv)
	}

	return value
}
