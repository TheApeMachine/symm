package strategy

import (
	"context"
	"io"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	nmstore "github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/store"
)

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

	errnie.Debug(
		"[training.Step]",
		"observed", observed,
	)

	return value
}
