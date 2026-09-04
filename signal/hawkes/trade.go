package hawkes

import (
	"fmt"
	"sync"
	"time"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/temporal"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

type input struct {
	clock temporal.Clock
}

/*
Trade is the arrival-dynamics market entity. It maintains an online bivariate
Hawkes process model via a single nomagique.Number composition and projects
data.Measurement outputs.
*/
type Trade struct {
	mu     sync.Mutex
	number *nomagique.Pipeline
	hawkes *algo.Hawkes

	in     input
	symbol string
	at     time.Time
}

/*
NewTrade constructs the Trade entity with a single inlined Number composition.
*/
func NewTrade() *Trade {
	trade := &Trade{}

	keyStore := store.NewKeyStore(func() string { return trade.symbol })

	trade.hawkes = algo.NewHawkes(algo.HawkesConfig{
		Clock: &trade.in.clock,
		Store: keyStore,
		Key:   func() string { return trade.symbol },
	})

	trade.number = nomagique.Number(trade.hawkes)

	return trade
}

/*
Step receives one trade, advances the bivariate Hawkes arrival pipeline,
and projects exactly one Measurement.
*/
func (trade *Trade) Step(observation kraken.TradeData) *data.Measurement[float64] {
	if observation.Side != "buy" && observation.Side != "sell" {
		return &data.Measurement[float64]{Err: fmt.Errorf(
			"hawkes: unsupported trade side %q", observation.Side,
		)}
	}

	trade.mu.Lock()
	defer trade.mu.Unlock()

	trade.symbol = observation.Symbol
	trade.at = observation.Timestamp
	trade.in.clock.Observe(observation.Timestamp)

	mark := markForSide(observation.Side)
	trade.number.Step(nmtypes.Scalar(mark))

	return trade.number.Measurement()
}

/*
Close releases resources held by the Trade entity.
*/
func (trade *Trade) Close() error {
	return nil
}

/*
markForSide encodes one trade's aggressor side into the process mark: buys
are the positive mark (+1), sells are the negative mark (-1).
*/
func markForSide(side string) float64 {
	if side == "buy" {
		return 1
	}

	return -1
}
