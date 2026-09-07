package hawkes

import (
	"fmt"
	"sync"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	nmhawkes "github.com/theapemachine/symm/nomagique/statistic/hawkes"
	"github.com/theapemachine/symm/nomagique/transport"
)

// Trade presents complete, event-timed arrivals to one keyed Primitive owner.
// No clock or symbol side-channel can change during an observation.
type Trade struct {
	mu      sync.Mutex
	process core.Primitive
}

func NewTrade() *Trade { return &Trade{process: nmhawkes.NewBivariate()} }
func (trade *Trade) Step(observation kraken.TradeData) *data.Measurement[float64] {
	if observation.Side != "buy" && observation.Side != "sell" {
		return &data.Measurement[float64]{Err: fmt.Errorf("hawkes: unsupported trade side %q", observation.Side)}
	}
	trade.mu.Lock()
	defer trade.mu.Unlock()
	measurement, err := transport.Evaluate[*data.Measurement[float64]](trade.process, core.Record(map[string]any{"key": observation.Symbol, "at": observation.Timestamp.UnixNano(), "mark": markForSide(observation.Side)}))
	if err != nil {
		return &data.Measurement[float64]{Err: err}
	}
	return measurement
}
func (trade *Trade) Close() error { return nil }
func markForSide(side string) float64 {
	if side == "buy" {
		return 1
	}
	return -1
}
