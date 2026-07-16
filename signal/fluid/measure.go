package fluid

import (
	"fmt"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

/*
Calculate applies one central immutable market cut in deterministic event-time
order and publishes only the measurements produced by Fluid's calculators.
*/
func (signal *Signal) Calculate(
	frame *types.MarketFrame,
) ([]*types.Measurement, error) {
	events := signal.events(frame.Tickers, frame.Trades, frame.Books)
	measurements := signal.apply(events)

	return measurements, nil
}

/*
events creates the deterministic entity timeline. At equal timestamps ticker
context precedes tape, and tape precedes the publishing book observation.
*/
func (signal *Signal) events(
	tickers []kraken.TickerData,
	trades []kraken.TradeData,
	books []kraken.BookData,
) []types.Event {
	events := make([]types.Event, 0, len(tickers)+len(trades)+len(books))

	for index, row := range tickers {
		events = append(events, types.Event{
			Stream:   "ticker",
			Priority: 0,
			Sequence: uint64(index + 1),
			At:       row.Timestamp,
			Symbol:   row.Symbol,
			Row:      row,
		})
	}

	for index, row := range trades {
		events = append(events, types.Event{
			Stream:   "trade",
			Priority: 1,
			Sequence: uint64(index + 1),
			At:       row.Timestamp,
			Symbol:   row.Symbol,
			Row:      row,
		})
	}

	for index, row := range books {
		events = append(events, types.Event{
			Stream:   "book",
			Priority: 2,
			Sequence: uint64(index + 1),
			At:       row.Timestamp,
			Symbol:   row.Symbol,
			Row:      row,
		})
	}

	types.OrderEvents(events)

	return events
}

/*
apply feeds the already ordered timeline into Fluid's composed entity
calculators and retains every valid measurement emitted by book events.
*/
func (signal *Signal) apply(events []types.Event) []*types.Measurement {
	measurements := make([]*types.Measurement, 0, len(events))

	for _, event := range events {
		measured, err := signal.measure(event)

		if err != nil {
			errnie.Error(err)
			continue
		}

		measurements = append(measurements, measured...)
	}

	return measurements
}

/*
measure dispatches one ordered event to its existing specialized entity
calculator without adding another model or state layer.
*/
func (signal *Signal) measure(
	event types.Event,
) ([]*types.Measurement, error) {
	switch event.Stream {
	case "ticker":
		return signal.ticker.Measure(event.Row.(kraken.TickerData))
	case "trade":
		return signal.trade.Measure(event.Row.(kraken.TradeData))
	case "book":
		return signal.book.Measure(event.Row.(kraken.BookData))
	}

	return nil, fmt.Errorf("fluid: unsupported event stream %q", event.Stream)
}
