package depthflow

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/algorithm/book/flow"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

/*
Signal is the "Weight of the Book" perspective, measuring touch-level book
imbalance with trade-pressure confirmation. Categories belong in logic; this
signal emits numerical scores only.
*/
type Signal struct {
	*types.Actor
	thesis   *types.Thesis
	ctx      context.Context
	cancel   context.CancelFunc
	sample   *flow.Sample
	bookflow *equation.Bookflow
	ui       chan []byte
}

/*
NewSignal creates depth-flow state shared by the causally ordered book and
trade observations in each central market cut.
*/
func NewSignal(
	ctx context.Context,
	ui chan []byte,
	historyCapacity int,
) (*Signal, error) {
	ctx, cancel := context.WithCancel(ctx)
	sample, err := flow.NewSample(historyCapacity)

	if err != nil {
		cancel()
		return nil, err
	}

	signal := &Signal{
		ctx:      ctx,
		cancel:   cancel,
		sample:   sample,
		bookflow: equation.NewBookflow(),
		ui:       ui,
	}

	signal.Actor = types.NewActor(ctx, map[string]types.Handler{
		"ticker": {
			Topic: "thesis",
			Fn:    signal.onTicker,
		},
		"book": {
			Topic: "thesis",
			Fn:    signal.onBook,
		},
		"trade": {
			Topic: "thesis",
			Fn:    signal.onTrade,
		},
	})

	return signal, nil
}

/*
Name returns the signal source identity.
*/
func (signal *Signal) Name() string {
	return string(types.SourceDepthFlow)
}

/*
Initialize wires ticker, book, and trade ingress from Live.
*/
func (signal *Signal) Initialize(live *types.Actor, thesis *types.Thesis) {
	signal.thesis = thesis
	signal.Actor.Initialize(
		types.Topic{Name: "ticker", Actor: live},
		types.Topic{Name: "book", Actor: live},
		types.Topic{Name: "trade", Actor: live},
	)
}

func (signal *Signal) onTicker(message any) any {
	rows := message.(*kraken.Ticker).Data
	measurements, err := signal.Calculate(rows, nil, nil)

	if err != nil {
		errnie.Error(err)
		return nil
	}

	if len(measurements) == 0 {
		return nil
	}

	signal.thesis.Measurements = append(signal.thesis.Measurements, measurements...)

	return signal.thesis
}

func (signal *Signal) onBook(message any) any {
	rows := message.(*kraken.Book).Data
	measurements, err := signal.Calculate(nil, nil, rows)

	if err != nil {
		errnie.Error(err)
		return nil
	}

	if len(measurements) == 0 {
		return nil
	}

	signal.thesis.Measurements = append(signal.thesis.Measurements, measurements...)

	return signal.thesis
}

func (signal *Signal) onTrade(message any) any {
	rows := message.(*kraken.Trade).Data
	measurements, err := signal.Calculate(nil, rows, nil)

	if err != nil {
		errnie.Error(err)
		return nil
	}

	if len(measurements) == 0 {
		return nil
	}

	signal.thesis.Measurements = append(signal.thesis.Measurements, measurements...)

	return signal.thesis
}

func (signal *Signal) Calculate(
	tickers []kraken.TickerData,
	trades []kraken.TradeData,
	books []kraken.BookData,
) ([]*types.Measurement, error) {
	events := depthEvents(books, trades)
	out := make([]*types.Measurement, 0, len(events))

	for _, event := range events {
		if event.Stream == "book" {
			measurements, err := signal.measureBook(event.Row.(kraken.BookData))

			// One malformed or crossed venue book must not abort the cut for
			// every other symbol; skip the unmeasurable event.
			if err != nil {
				continue
			}

			out = append(out, measurements...)
			continue
		}

		measurements, err := signal.measureTrade(event.Row.(kraken.TradeData))

		if err != nil {
			continue
		}

		out = append(out, measurements...)
	}

	types.WireMeasurements(out, signal.ui)

	return out, nil
}

/*
measureBook applies one book event to the shared flow sample and emits the
resulting measurements only after both sample and equation report readiness.
*/
func (signal *Signal) measureBook(
	row kraken.BookData,
) ([]*types.Measurement, error) {
	if row.Symbol == "" {
		return nil, fmt.Errorf("depthflow: book symbol required")
	}

	if row.Timestamp.IsZero() {
		return nil, fmt.Errorf("depthflow: book timestamp required for %s", row.Symbol)
	}

	if row.PriceIncrement == nil || row.PriceIncrement.Sign() <= 0 {
		return nil, fmt.Errorf("depthflow: positive price increment required for %s", row.Symbol)
	}

	bids, asks, err := kraken.BookLevels(row)

	if err != nil {
		return nil, fmt.Errorf("depthflow: project %s book levels: %w", row.Symbol, err)
	}

	// Kraken book rows are atomic. Applying removals first prevents replacement
	// array order from changing which prior level counts as the cancelled touch.
	for _, levels := range [][]flow.BookLevel{bids, asks} {
		sort.SliceStable(levels, func(left, right int) bool {
			return levels[left].Quantity == 0 && levels[right].Quantity > 0
		})
	}

	input, ready, maturity, err := signal.sample.MeasureBook(flow.BookInput{
		Symbol:   row.Symbol,
		TickSize: row.PriceIncrement.Float64(),
		Bids:     bids,
		Asks:     asks,
	})

	if err != nil {
		return nil, fmt.Errorf("depthflow: measure %s book: %w", row.Symbol, err)
	}

	if !ready {
		return nil, nil
	}

	output, err := signal.bookflow.Measure(input)

	if err != nil {
		return nil, fmt.Errorf("depthflow: classify %s book: %w", row.Symbol, err)
	}

	if !output.Ready {
		return nil, nil
	}

	return signal.frame(row.Symbol, row.Timestamp, output, maturity), nil
}

/*
measureTrade applies one trade event to the shared flow sample at its causal
position in the merged entity timeline.
*/
func (signal *Signal) measureTrade(
	row kraken.TradeData,
) ([]*types.Measurement, error) {
	if row.Symbol == "" || row.Price.Sign() <= 0 || row.Qty <= 0 || row.Timestamp.IsZero() {
		return nil, fmt.Errorf("depthflow: complete positive trade required")
	}

	input, ready, maturity, err := signal.sample.MeasureTrade(flow.TradeInput{
		Symbol:   row.Symbol,
		Price:    row.Price.Float64(),
		Quantity: row.Qty,
		Side:     row.Side,
	})

	if err != nil {
		return nil, fmt.Errorf("depthflow: measure %s trade: %w", row.Symbol, err)
	}

	if !ready {
		return nil, nil
	}

	output, err := signal.bookflow.Measure(input)

	if err != nil {
		return nil, fmt.Errorf("depthflow: classify %s trade: %w", row.Symbol, err)
	}

	if !output.Ready {
		return nil, nil
	}

	return signal.frame(row.Symbol, row.Timestamp, output, maturity), nil
}

/*
depthEvents merges book and trade batches by event time. Trades precede books
at equal timestamps so a publishing book observation includes simultaneous tape.
*/
func depthEvents(
	books []kraken.BookData,
	trades []kraken.TradeData,
) []types.Event {
	events := make([]types.Event, 0, len(books)+len(trades))

	for index, row := range trades {
		events = append(events, types.Event{
			Stream:   "trade",
			Priority: 0,
			Sequence: uint64(index + 1),
			At:       row.Timestamp,
			Symbol:   row.Symbol,
			Row:      row,
		})
	}

	for index, row := range books {
		events = append(events, types.Event{
			Stream:   "book",
			Priority: 1,
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
frame converts a bookflow calculator output into the shared Measurement shape,
so both the book-driven and trade-driven observation paths emit the same metric
set for a symbol.
*/
func (signal *Signal) frame(
	symbol string, at time.Time, output equation.BookflowOutput, maturity float64,
) []*types.Measurement {
	validity := types.MeasurementValidity{
		State:     types.ValidityValid,
		Readiness: types.ReadinessObservation,
	}
	scale := types.ScaleReference{
		Kind:    types.ScaleObservationWindow,
		From:    at,
		Through: at,
	}
	specs := []struct {
		metric types.MetricType
		raw    float64
	}{
		{types.MetricLoadedScore, output.LoadedScore},
		{types.MetricSpoofScore, output.SpoofScore},
		{types.MetricThinScore, output.ThinScore},
		{types.MetricNeutralScore, output.NeutralScore},
		{types.MetricStrength, output.Strength},
		{types.MetricValue, output.Value},
	}
	measurements := make([]*types.Measurement, 0, len(specs))

	for _, spec := range specs {
		measurements = append(measurements, &types.Measurement{
			Source:     types.SourceDepthFlow,
			Stream:     types.DepthFlow,
			Metric:     spec.metric,
			Subject:    types.SubjectBookImbalance,
			Symbol:     symbol,
			At:         at,
			Unit:       types.UnitDimensionless,
			Raw:        spec.raw,
			Normalized: types.NormalizeFinite(spec.raw),
			Maturity:   maturity,
			Validity:   validity,
			Scale:      scale,
		})
	}

	return measurements
}

/*
Close releases the receiver's owned resources so shutdown does not leave
active market-data producers.
*/
func (signal *Signal) Close() error {
	signal.cancel()
	return nil
}
