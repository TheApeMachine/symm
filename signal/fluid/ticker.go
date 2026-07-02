package fluid

import (
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/market"
)

type Ticker struct {
	registry *Registry
}

func NewTicker(registry *Registry) *Ticker {
	return &Ticker{registry: registry}
}

func (ticker *Ticker) Measure(
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
	state := ticker.registry.loadSymbol(symbol)

	if state == nil {
		return frame.WithError(errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"fluid: symbol state required",
			nil,
		)))
	}

	if err := state.FeedTicker(tickerUpdate(frame, -1, symbol, eventAt), eventAt); errnie.Error(err) != nil {
		return frame.WithError(errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			err.Error(),
			err,
		)))
	}

	return nil
}
