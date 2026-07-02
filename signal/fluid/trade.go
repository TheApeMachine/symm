package fluid

import (
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/market"
)

type Trade struct {
	registry *Registry
}

func NewTrade(registry *Registry) *Trade {
	return &Trade{registry: registry}
}

func (trade *Trade) Measure(
	frame *datura.Artifact,
	crossSection *market.CrossSection,
) *datura.Artifact {
	eventAt, err := eventTime(frame, -1)

	if err != nil {
		return frame.WithError(errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			err.Error(),
			err,
		)))
	}

	symbol, _ := frame.Scope()
	state := trade.registry.loadSymbol(symbol)

	if state == nil {
		return frame.WithError(errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"fluid: symbol state required",
			nil,
		)))
	}

	update := tradeUpdate(frame, -1, symbol, eventAt)

	if update.Price <= 0 || update.Qty <= 0 {
		return frame.WithError(errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"fluid: trade price and qty required",
			nil,
		)))
	}

	if err := state.FeedTrade(eventAt, update.Price, update.Qty, update.Side); errnie.Error(err) != nil {
		return frame.WithError(errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			err.Error(),
			err,
		)))
	}

	return nil
}
