package depthflow

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/algorithm/book/flow"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
)

/*
Signal is the "Weight of the Book" perspective, measuring touch-level book
imbalance with trade-pressure confirmation. Categories belong in logic; this
signal emits numerical scores only.
*/
type Signal struct {
	thesis   *types.Thesis
	ctx      context.Context
	cancel   context.CancelFunc
	sample   *flow.Sample
	bookflow *equation.Bookflow
	bookOut  atomic.Pointer[[]*types.Measurement]
	ui       chan []byte
	ticker    *types.Subscription[*kraken.Ticker]
	book      *types.Subscription[*kraken.Book]
	trade     *types.Subscription[*kraken.Trade]
	subMu     sync.Mutex
	theses    []*types.Subscription[*types.Thesis]
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
func (signal *Signal) Initialize(market types.MarketFeed, thesis *types.Thesis) {
	signal.thesis = thesis

	if market != nil {
		signal.ticker = market.Ticker()
		signal.book = market.Book()
		signal.trade = market.Trade()
	}

	go signal.run()
}

func (signal *Signal) Thesis() *types.Subscription[*types.Thesis] {
	subscription := types.NewSubscription[*types.Thesis]()
	signal.subMu.Lock()
	signal.theses = append(signal.theses, subscription)
	signal.subMu.Unlock()
	return subscription
}

func (signal *Signal) run() {
	var tickers <-chan *kraken.Ticker
	var books <-chan *kraken.Book
	var trades <-chan *kraken.Trade

	if signal.ticker != nil {
		tickers = signal.ticker.Channel
	}

	if signal.book != nil {
		books = signal.book.Channel
	}

	if signal.trade != nil {
		trades = signal.trade.Channel
	}

	for {
		select {
		case <-signal.ctx.Done():
			return
		case ticker := <-tickers:
			signal.onTicker(ticker)
		case book := <-books:
			signal.onBook(book)
		case trade := <-trades:
			signal.onTrade(trade)
		}
	}
}

func (signal *Signal) onTicker(ticker *kraken.Ticker) {
	signal.publish(signal.thesis.AppendMeasuremnts(
		types.SourceDepthFlow, signal.Calculate(ticker.Data, nil, nil),
	))
}

func (signal *Signal) onBook(book *kraken.Book) {
	signal.publish(signal.thesis.AppendMeasuremnts(
		types.SourceDepthFlow, signal.Calculate(nil, nil, book.Data),
	))
}

func (signal *Signal) onTrade(trade *kraken.Trade) {
	signal.publish(signal.thesis.AppendMeasuremnts(
		types.SourceDepthFlow, signal.Calculate(nil, trade.Data, nil),
	))
}

func (signal *Signal) publish(thesis *types.Thesis) {
	if thesis == nil {
		return
	}

	signal.subMu.Lock()
	subscribers := append([]*types.Subscription[*types.Thesis](nil), signal.theses...)
	signal.subMu.Unlock()

	for _, subscription := range subscribers {
		subscription.Send(thesis)
	}
}

func (signal *Signal) Calculate(
	tickers []kraken.TickerData,
	trades []kraken.TradeData,
	books []kraken.BookData,
) []*types.Measurement {
	events := depthEvents(books, trades)
	out, err := types.MeasureEventsParallel(events, func(event types.Event) ([]*types.Measurement, error) {
		if event.Stream == "book" {
			return signal.measureBook(event.Row.(kraken.BookData))
		}

		return signal.measureTrade(event.Row.(kraken.TradeData))
	})

	uiOut := datura.NewMap(
		"measurements", make([]*types.Measurement, 0),
	)

	for _, measurement := range out {
		if measurement.Symbol == types.Focus() {
			uiOut["measurements"] = append(
				uiOut["measurements"].([]*types.Measurement), measurement,
			)
		}
	}

	if err != nil {
		errnie.Error(err)
		return nil
	}

	if len(uiOut["measurements"].([]*types.Measurement)) > 0 {
		utils.Publish(signal.ui, uiOut)
	}

	return out
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
		Side:     flow.TradeSide(row.Side),
		At:       row.Timestamp,
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
frame converts a bookflow calculator output into one source×symbol row so both
the book-driven and trade-driven observation paths emit the same metric set.
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
	measurement := &types.Measurement{
		Source:   types.SourceDepthFlow,
		Symbol:   symbol,
		At:       at,
		Maturity: signal.thesis.Tick,
		Validity: validity,
		Metrics:  make(map[string]types.MetricSample, 6),
		Scale:    scale,
	}
	measurement.Metrics[types.MetricKey(types.MetricLoadedScore, types.SideNone)] = types.MetricSample{Raw: output.LoadedScore, Normalized: types.NormalizeFinite(output.LoadedScore), Unit: types.UnitDimensionless}
	measurement.Metrics[types.MetricKey(types.MetricSpoofScore, types.SideNone)] = types.MetricSample{Raw: output.SpoofScore, Normalized: types.NormalizeFinite(output.SpoofScore), Unit: types.UnitDimensionless}
	measurement.Metrics[types.MetricKey(types.MetricThinScore, types.SideNone)] = types.MetricSample{Raw: output.ThinScore, Normalized: types.NormalizeFinite(output.ThinScore), Unit: types.UnitDimensionless}
	measurement.Metrics[types.MetricKey(types.MetricNeutralScore, types.SideNone)] = types.MetricSample{Raw: output.NeutralScore, Normalized: types.NormalizeFinite(output.NeutralScore), Unit: types.UnitDimensionless}
	measurement.Metrics[types.MetricKey(types.MetricStrength, types.SideNone)] = types.MetricSample{Raw: output.Strength, Normalized: types.NormalizeFinite(output.Strength), Unit: types.UnitDimensionless}
	measurement.Metrics[types.MetricKey(types.MetricValue, types.SideNone)] = types.MetricSample{Raw: output.Value, Normalized: types.NormalizeFinite(output.Value), Unit: types.UnitDimensionless}

	return []*types.Measurement{measurement}
}

/*
Close releases the receiver's owned resources so shutdown does not leave
active market-data producers.
*/
func (signal *Signal) Close() error {
	signal.cancel()
	return nil
}
