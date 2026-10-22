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
	}

	return training
}

func (training *Training) Step(value *data.Value) *data.Value {
	if value == nil {
		return nil
	}

	observed, err := training.grid.Observe(value)

	if err != nil {
		training.Error(err)
		return nil
	}

	for _, region := range observed {
		training.model.Evaluate(data.NewValue(
			"training", "update", region.Symbol,
		).Append(
			tokenizer.NewToken(region), "token",
		))
	}

	// Append the observed regions so the frontend's Impulse Map and
	// position-card radial charts can render them.
	if len(observed) > 0 {
		value.Append(observed, "train", "regions")
	}

	// Safety net: settle any closed virtuals that were not handled
	// inside Evaluate (e.g. restored from history on restart).
	for _, cv := range training.desk.ClosedVirtuals() {
		training.model.Settle(cv)
	}

	return value
}
