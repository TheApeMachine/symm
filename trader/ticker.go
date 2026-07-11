package trader

import (
	"maps"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/structure"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

type Ticker struct {
	status       types.Status
	signals      []types.Signal[any]
	crossSection *types.CrossSection
	sequence     uint64
	ring         *structure.SPSCRing[[]byte]
	uiHub        chan []byte
	rows         map[string]kraken.TickerData
}

func NewTicker(signal *Signal, uiHub chan []byte) *Ticker {
	return &Ticker{
		status:       types.INITIALIZING,
		signals:      signal.Ticker,
		crossSection: signal.CrossSection,
		ring: structure.NewSPSCRing[[]byte](
			viper.GetInt("signals.feed_ring_capacity"),
			true,
		),
		uiHub: uiHub,
	}
}

func (ticker *Ticker) Status() types.Status {
	return ticker.status
}

/*
Drain decodes every queued ticker frame, folds each row into the shared
CrossSection (in this stream's own arrival order, which is already its
correct event-time order), and returns the ready rows as ordered events.
It performs no signal measurement, so a Chunker can merge these events
with every other stream's before any signal sees them.
*/
func (ticker *Ticker) Drain() ([]types.Event, error) {
	events := make([]types.Event, 0)

	batchSize := ticker.ring.Len()

	for range batchSize {
		frame := ticker.ring.Pop()

		if len(frame) == 0 {
			break
		}

		message := kraken.NewTicker(frame).Data

		if ticker.status != types.READY && len(message) > 0 {
			ticker.status = types.READY
		}

		if ticker.crossSection != nil {
			if err := ticker.crossSection.Observe(message); err != nil {
				return nil, errnie.Error(errnie.Err(
					errnie.UnprocessableContent,
					err.Error(),
					err,
				))
			}
		}

		for _, msg := range message {
			row, ready := ticker.apply(msg)

			if !ready {
				continue
			}

			ticker.sequence++
			events = append(events, types.Event{
				Stream:   "ticker",
				Sequence: ticker.sequence,
				At:       row.Timestamp,
				Symbol:   row.Symbol,
				Price:    tickerMidPrice(row),
				Row:      row,
			})
		}
	}

	return events, nil
}

/*
MeasureEvent runs one already-ordered ticker event through this feed's
signals against snapshot, the frozen cross-section a Chunker took for
the whole drain cycle this event belongs to.
*/
func (ticker *Ticker) MeasureEvent(
	event types.Event, snapshot *types.CrossSection,
) ([]*types.Measurement, error) {
	row, ok := event.Row.(kraken.TickerData)

	if !ok {
		return nil, nil
	}

	measurements := make([]*types.Measurement, 0)

	results := measureSignals(ticker.signals, func(signal types.Signal[any]) ([]*types.Measurement, error) {
		return signal.Measure(row, snapshot)
	})

	for _, result := range results {
		if result.err != nil {
			return nil, errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				result.err.Error(),
				result.err,
			))
		}

		for _, item := range result.measurements {
			if item.Metrics == nil {
				item.Metrics = map[string]float64{}
			}

			if event.Price > 0 {
				item.Metrics["price"] = event.Price
			}
		}

		if len(result.measurements) > 0 {
			measurements = append(measurements, result.measurements...)
		}
	}

	if ticker.status != types.READY && len(measurements) > 0 {
		ticker.status = types.READY
		errnie.Info("ticker ready")
	}

	return measurements, nil
}

/*
Measure drains and measures this feed on its own, using its own live
cross-section rather than a frozen cycle-wide snapshot. Crypto's runtime
loop uses Chunker instead; this remains for direct single-feed use.
*/
func (ticker *Ticker) Measure() ([]*types.Measurement, error) {
	events, err := ticker.Drain()

	if err != nil {
		return nil, err
	}

	measurements := make([]*types.Measurement, 0)

	for _, event := range events {
		result, err := ticker.MeasureEvent(event, ticker.crossSection)

		if err != nil {
			return nil, err
		}

		measurements = append(measurements, result...)
	}

	select {
	case ticker.uiHub <- datura.Map[any]{
		"measurements": measurements,
	}.Marshal():
	default:
	}

	return measurements, nil
}

/*
Snapshot returns a copy of the latest merged ticker row observed for every
symbol seen so far. Universe uses this to rank the full observation tier
without holding a reference into Ticker's internal state.
*/
func (ticker *Ticker) Snapshot() map[string]kraken.TickerData {
	snapshot := make(map[string]kraken.TickerData, len(ticker.rows))

	maps.Copy(snapshot, ticker.rows)

	return snapshot
}

func (ticker *Ticker) apply(row kraken.TickerData) (kraken.TickerData, bool) {
	if row.Symbol == "" {
		return kraken.TickerData{}, false
	}

	if ticker.rows == nil {
		ticker.rows = map[string]kraken.TickerData{}
	}

	merged := ticker.rows[row.Symbol]

	if row.Bid.Float64() > 0 {
		merged.Bid = row.Bid
	}

	if row.BidQty > 0 {
		merged.BidQty = row.BidQty
	}

	if row.Ask.Float64() > 0 {
		merged.Ask = row.Ask
	}

	if row.AskQty > 0 {
		merged.AskQty = row.AskQty
	}

	if row.Last.Float64() > 0 {
		merged.Last = row.Last
	}

	if row.Volume > 0 {
		merged.Volume = row.Volume
	}

	if row.Vwap > 0 {
		merged.Vwap = row.Vwap
	}

	if row.Low.Float64() > 0 {
		merged.Low = row.Low
	}

	if row.High.Float64() > 0 {
		merged.High = row.High
	}

	if row.Change.Float64() != 0 {
		merged.Change = row.Change
	}

	if row.ChangePct != 0 {
		merged.ChangePct = row.ChangePct
	}

	if !row.Timestamp.IsZero() {
		merged.Timestamp = row.Timestamp
	}

	merged.Symbol = row.Symbol
	ticker.rows[row.Symbol] = merged

	if merged.Last.Float64() <= 0 || merged.Bid.Float64() <= 0 || merged.Ask.Float64() <= 0 || merged.Volume <= 0 {
		return merged, false
	}

	return merged, true
}

func tickerMidPrice(row kraken.TickerData) float64 {
	price := row.Bid.Add(row.Ask).Div(decimal.NewFromInt64(2))

	if price.Sign() <= 0 {
		return 0
	}

	return price.Float64()
}

func (ticker *Ticker) On(data []byte) {
	frame := make([]byte, len(data))
	copy(frame, data)

	if !ticker.ring.Push(frame) {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"trader: ticker ring full",
			nil,
		))
	}
}
