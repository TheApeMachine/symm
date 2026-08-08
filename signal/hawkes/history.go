package hawkes

import (
	"fmt"
	"sort"
	"time"

	"github.com/theapemachine/nomagique/algorithm/excitation"
	nmhawkes "github.com/theapemachine/nomagique/hawkes"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

/*
arrivalHistory reconstructs one symbol's bounded Hawkes input from its prior
Measurement and the trades that have not yet produced a newer Measurement.
*/
type arrivalHistory struct {
	symbol string
	origin time.Time
	buys   []time.Time
	sells  []time.Time
}

func newArrivalHistory(previous *types.Measurement) *arrivalHistory {
	history := &arrivalHistory{}

	if previous == nil {
		return history
	}

	history.symbol = previous.Symbol
	history.origin = previous.ObservedFrom

	for _, arrival := range previous.Arrivals {
		if arrival.Side == types.SideBuy {
			history.buys = append(history.buys, arrival.At)
		}

		if arrival.Side == types.SideSell {
			history.sells = append(history.sells, arrival.At)
		}
	}

	return history
}

func (history *arrivalHistory) Append(trade kraken.TradeData) error {
	if trade.Symbol == "" || trade.Timestamp.IsZero() {
		return fmt.Errorf("hawkes arrival requires symbol and timestamp")
	}

	if history.symbol != "" && history.symbol != trade.Symbol {
		return fmt.Errorf(
			"hawkes arrival symbol %s does not match history %s",
			trade.Symbol,
			history.symbol,
		)
	}

	history.symbol = trade.Symbol

	if trade.Side == "buy" {
		history.buys = append(history.buys, trade.Timestamp)
		return nil
	}

	if trade.Side == "sell" {
		history.sells = append(history.sells, trade.Timestamp)
		return nil
	}

	return fmt.Errorf("hawkes arrival has unsupported side %q", trade.Side)
}

func (history *arrivalHistory) Input() (excitation.Input, bool) {
	stream := nmhawkes.NewArrivalStream(history.buys, history.sells)

	if !history.origin.IsZero() {
		stream = nmhawkes.NewArrivalStreamFrom(
			history.origin,
			history.buys,
			history.sells,
		)
	}

	_, horizon, found := stream.Bounds()

	if !found {
		return excitation.Input{}, false
	}

	return excitation.Input{
		Symbol:       history.symbol,
		ObservedFrom: stream.ObservationOrigin(),
		Horizon:      horizon,
		Stream:       stream,
	}, true
}

func (history *arrivalHistory) Arrivals(from time.Time) []types.MeasurementArrival {
	arrivals := make([]types.MeasurementArrival, 0, len(history.buys)+len(history.sells))

	for _, at := range history.buys {
		if !at.Before(from) {
			arrivals = append(arrivals, types.MeasurementArrival{
				At: at, Side: types.SideBuy,
			})
		}
	}

	for _, at := range history.sells {
		if !at.Before(from) {
			arrivals = append(arrivals, types.MeasurementArrival{
				At: at, Side: types.SideSell,
			})
		}
	}

	sort.SliceStable(arrivals, func(left, right int) bool {
		if arrivals[left].At.Equal(arrivals[right].At) {
			return arrivals[left].Side < arrivals[right].Side
		}

		return arrivals[left].At.Before(arrivals[right].At)
	})

	return arrivals
}
