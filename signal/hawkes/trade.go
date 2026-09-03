package hawkes

import (
	"fmt"
	"sync"
	"time"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/data"
	nmhawkes "github.com/theapemachine/symm/nomagique/statistic/hawkes"
)

/*
Trade is the arrival-dynamics market entity. It maintains an online bivariate
Hawkes process model per symbol and projects data.Measurement outputs.
*/
type Trade struct {
	mu      sync.Mutex
	engines map[string]*nmhawkes.Engine
}

/*
NewTrade constructs the Trade entity.
*/
func NewTrade() *Trade {
	return &Trade{
		engines: make(map[string]*nmhawkes.Engine),
	}
}

/*
Step receives one trade, advances the bivariate Hawkes arrival process engine,
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

	engine, found := trade.engines[observation.Symbol]
	if !found {
		engine = nmhawkes.NewEngine()
		trade.engines[observation.Symbol] = engine
	}

	id := observation.Symbol + ":hawkes:" + observation.Timestamp.Format(time.RFC3339Nano)
	measurement := data.NewMeasurement[float64](
		id,
		observation.Symbol,
		"hawkes",
		observation.Timestamp,
		observation.Timestamp,
	)

	mark := markForSide(observation.Side)
	from, err := engine.Step(mark, observation.Timestamp, measurement)
	if err != nil {
		measurement.Err = err
		return measurement
	}
	measurement.From = from

	return measurement
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
