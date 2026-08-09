package exhaust

import (
	"context"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	spotbook "github.com/theapemachine/api-go/v2/pkg/book"
	"github.com/theapemachine/api-go/v2/pkg/decimal"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"golang.org/x/sync/errgroup"

	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/algorithm/book/flow"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
)

/*
Signal tracks side-specific microstructure decay that can advise whether a long
or short position should exit. It emits numerical family scores, their fused
urgency, and the winning numerical family identifier for downstream logic.
*/
type Signal struct {
	status     types.Status
	ctx        context.Context
	cancel     context.CancelFunc
	books      websocket.BookSource
	instrument *broker.Instrument
	sample     *algorithm.DecaySample
	decay      *equation.Decay
	ui         chan []byte
	thesis     *types.Thesis
	semaphore  chan struct{}
	lastTrade  *sync.Map
	lastBookAt *sync.Map
	lastBook   *sync.Map
}

type tradeCursor struct {
	at  time.Time
	ids map[int64]struct{}
}

type bookSnapshot struct {
	bids map[int64]flow.BookLevel
	asks map[int64]flow.BookLevel
}

const maximumDecayCategory = 4

/*
NewSignal constructs the market-wide exhaustion observer. Position inventory is
deliberately absent: the signal measures both hypothetical exit sides, leaving
the consumer to select the side matching its position.
*/
func NewSignal(
	ctx context.Context,
	books websocket.BookSource,
	instrument *broker.Instrument,
	ui chan []byte,
	thesis *types.Thesis,
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		status:     types.INITIALIZING,
		ctx:        ctx,
		cancel:     cancel,
		books:      books,
		instrument: instrument,
		sample:     algorithm.NewDecaySample(),
		decay:      equation.NewDecay(),
		ui:         ui,
		thesis:     thesis,
		semaphore:  make(chan struct{}, 1),
		lastTrade:  &sync.Map{},
		lastBookAt: &sync.Map{},
		lastBook:   &sync.Map{},
	}

	signal.thesis.Subscribe(types.SourceExhaustion, signal.semaphore)
	signal.run()
	signal.status = types.READY

	return signal
}

/*
Name returns the signal source identity.
*/
func (signal *Signal) Name() string {
	return string(types.SourceExhaustion)
}

func (signal *Signal) Status() types.Status {
	return signal.status
}

func (signal *Signal) run() {
	go func() {
		for {
			select {
			case <-signal.ctx.Done():
				return
			case <-signal.semaphore:
				errnie.Error(signal.thesis.AppendMeasurements(
					types.SourceExhaustion,
					signal.Measure(signal.thesis), true,
				))
			}
		}
	}()
}

func (signal *Signal) Measure(thesis *types.Thesis) []*types.Measurement {
	trades := thesis.MarketTrades(types.SourceExhaustion)
	measurements := make([]*types.Measurement, 0)
	out := make([]*types.Measurement, 0)

	if signal.books == nil {
		return measurements
	}

	if signal.lastTrade == nil {
		signal.lastTrade = &sync.Map{}
	}

	if signal.lastBookAt == nil {
		signal.lastBookAt = &sync.Map{}
	}

	if signal.lastBook == nil {
		signal.lastBook = &sync.Map{}
	}

	tradeBatches := make(map[string][]kraken.TradeData)
	symbols := make([]string, 0)
	symbolSet := make(map[string]struct{})

	thesis.Measurements.Range(func(_, value any) bool {
		rows, ok := value.([]*types.Measurement)

		if !ok {
			return true
		}

		for _, measurement := range rows {
			if measurement == nil || measurement.Symbol == "" {
				continue
			}

			if _, exists := symbolSet[measurement.Symbol]; exists {
				continue
			}

			symbolSet[measurement.Symbol] = struct{}{}
			symbols = append(symbols, measurement.Symbol)
		}

		return true
	})

	results := &sync.Map{}
	publish := &sync.Map{}

	for _, trade := range trades {
		if validTrade(trade) {
			tradeBatches[trade.Symbol] = append(tradeBatches[trade.Symbol], trade)

			if _, exists := symbolSet[trade.Symbol]; !exists {
				symbolSet[trade.Symbol] = struct{}{}
				symbols = append(symbols, trade.Symbol)
			}
		}
	}
	sort.Strings(symbols)

	group, _ := errgroup.WithContext(signal.ctx)

	for _, symbol := range symbols {
		symbolTrades := tradeBatches[symbol]
		sort.SliceStable(symbolTrades, func(leftIndex, rightIndex int) bool {
			left := symbolTrades[leftIndex]
			right := symbolTrades[rightIndex]

			if left.Timestamp.Equal(right.Timestamp) {
				return left.TradeID < right.TradeID
			}

			return left.Timestamp.Before(right.Timestamp)
		})
		group.Go(func() error {
			signal.books.Book(symbol, func(managed *spotbook.Book) {
				bookAt := managedBookObservedAt(managed)
				symbolMeasurements := make([]*types.Measurement, 0)
				symbolOut := make([]*types.Measurement, 0)
				bookPending := managed != nil

				for _, trade := range symbolTrades {
					if bookPending && !trade.Timestamp.Before(bookAt) {
						bookMeasurements, err := signal.measureManagedBook(managed)

						if err != nil {
							errnie.Error(errnie.Err(
								errnie.UnprocessableContent,
								"exhaust: failed to measure book",
								err,
							))
						}

						symbolMeasurements = append(symbolMeasurements, bookMeasurements...)
						bookPending = false
					}

					if signal.seenTrade(trade) {
						continue
					}

					lastBookAt, hasBook := signal.bookAt(trade.Symbol)

					if !hasBook || lastBookAt.IsZero() || lastBookAt.After(trade.Timestamp) {
						continue
					}

					tradeMeasurements, err := signal.measureTrade(trade)

					if err != nil {
						errnie.Error(errnie.Err(
							errnie.UnprocessableContent,
							"exhaust: failed to measure trade",
							err,
						))
						continue
					}

					signal.commitTrade(trade)
					symbolMeasurements = append(symbolMeasurements, tradeMeasurements...)

					if symbol == types.Focus() {
						symbolOut = append(symbolOut, tradeMeasurements...)
					}
				}

				if bookPending {
					bookMeasurements, err := signal.measureManagedBook(managed)

					if err != nil {
						errnie.Error(errnie.Err(
							errnie.UnprocessableContent,
							"exhaust: failed to measure book",
							err,
						))
					}

					symbolMeasurements = append(symbolMeasurements, bookMeasurements...)
				}

				results.Store(symbol, symbolMeasurements)
				publish.Store(symbol, symbolOut)
			})

			return nil
		})
	}

	if err := group.Wait(); err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"exhaust: parallel measurement failed",
			err,
		))
		return measurements
	}

	for _, symbol := range symbols {
		raw, exists := results.Load(symbol)

		if !exists {
			continue
		}

		symbolMeasurements := raw.([]*types.Measurement)
		measurements = append(measurements, symbolMeasurements...)

		focused, hasFocused := publish.Load(symbol)

		if hasFocused {
			out = append(out, focused.([]*types.Measurement)...)
		}
	}

	if len(out) > 0 {
		utils.Publish(signal.ui, datura.NewMap("measurements", out))
	}

	return measurements
}

func managedBookObservedAt(managed *spotbook.Book) time.Time {
	observedAt := time.Time{}

	if managed == nil {
		return observedAt
	}

	for bid := managed.Bids.High; bid != nil; bid = bid.Lower {
		if bid.Timestamp.After(observedAt) {
			observedAt = bid.Timestamp
		}
	}

	for ask := managed.Asks.Low; ask != nil; ask = ask.Higher {
		if ask.Timestamp.After(observedAt) {
			observedAt = ask.Timestamp
		}
	}

	return observedAt
}

func (signal *Signal) measureManagedBook(
	managed *spotbook.Book,
) ([]*types.Measurement, error) {
	bestBid, bestAsk := managed.BestBid(), managed.BestAsk()

	if bestBid == nil || bestAsk == nil {
		return nil, nil
	}

	current := bookSnapshot{
		bids: make(map[int64]flow.BookLevel),
		asks: make(map[int64]flow.BookLevel),
	}

	observedAt := time.Time{}
	instrument := signal.instrument.Pair(managed.Name)

	for bid := managed.Bids.High; bid != nil; bid = bid.Lower {
		level := flow.BookLevel{
			Price:    bid.Price.Float64(),
			Quantity: bid.Quantity.Float64(),
			Ticks: decimal.ExactDiv(
				bid.Price,
				&instrument.PriceIncrement,
			).Int64(),
		}
		current.bids[level.Ticks] = level

		if bid.Timestamp.After(observedAt) {
			observedAt = bid.Timestamp
		}
	}

	for ask := managed.Asks.Low; ask != nil; ask = ask.Higher {
		level := flow.BookLevel{
			Price:    ask.Price.Float64(),
			Quantity: ask.Quantity.Float64(),
			Ticks: decimal.ExactDiv(
				ask.Price,
				&instrument.PriceIncrement,
			).Int64(),
		}
		current.asks[level.Ticks] = level

		if ask.Timestamp.After(observedAt) {
			observedAt = ask.Timestamp
		}
	}

	if observedAt.IsZero() {
		return nil, nil
	}

	previous := signal.bookSnapshot(managed.Name)

	if sameSnapshot(current, previous) {
		lastBookAt, _ := signal.bookAt(managed.Name)

		if observedAt.After(lastBookAt) {
			signal.lastBookAt.Store(managed.Name, observedAt)
		}

		return nil, nil
	}

	lastBookAt, _ := signal.bookAt(managed.Name)

	if observedAt.Before(lastBookAt) {
		return nil, nil
	}

	input, _, maturity, err := signal.sample.MeasureBook(flow.BookInput{
		Symbol:   managed.Name,
		TickSize: instrument.TickSize.Float64(),
		Bids:     snapshotDelta(current.bids, previous.bids),
		Asks:     snapshotDelta(current.asks, previous.asks),
	})
	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"exhaust: failed to measure book",
			err,
		))
	}

	signal.lastBook.Store(managed.Name, current)
	signal.lastBookAt.Store(managed.Name, observedAt)

	output, err := signal.decay.Measure(input)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"exhaust: failed to measure decay",
			err,
		))
	}

	measurements := signal.frame(
		managed.Name,
		observedAt,
		output,
		maturity,
	)
	measurement := measurements[0]
	measurement.PutMetric(types.MetricBestPrice, types.SideBuy, types.MetricSample{
		Raw:  bestBid.Price.Float64(),
		Unit: types.UnitQuoteCurrency,
	})
	measurement.PutMetric(types.MetricBestPrice, types.SideSell, types.MetricSample{
		Raw:  bestAsk.Price.Float64(),
		Unit: types.UnitQuoteCurrency,
	})
	measurement.PutMetric(types.MetricTouchQuantity, types.SideBuy, types.MetricSample{
		Raw:  bestBid.Quantity.Float64(),
		Unit: types.UnitBaseCurrency,
	})
	measurement.PutMetric(types.MetricTouchQuantity, types.SideSell, types.MetricSample{
		Raw:  bestAsk.Quantity.Float64(),
		Unit: types.UnitBaseCurrency,
	})
	measurement.PutMetric(types.MetricMidpoint, types.SideNone, types.MetricSample{
		Raw:  (bestBid.Price.Float64() + bestAsk.Price.Float64()) / 2,
		Unit: types.UnitQuoteCurrency,
	})

	return measurements, nil
}

/*
measureTrade applies one trade event to the shared decay sample at its causal
position in the merged entity timeline.
*/
func (signal *Signal) measureTrade(
	row kraken.TradeData,
) ([]*types.Measurement, error) {
	if row.Symbol == "" || row.Timestamp.IsZero() || row.Price.Sign() <= 0 ||
		row.Qty <= 0 || row.Side != "buy" && row.Side != "sell" {
		return nil, nil
	}

	input, _, maturity, err := signal.sample.MeasureTrade(flow.TradeInput{
		Symbol:   row.Symbol,
		Price:    row.Price.Float64(),
		Quantity: row.Qty,
		Side:     flow.TradeSide(row.Side),
		At:       row.Timestamp,
	})

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"exhaust: failed to measure trade",
			err,
		))
	}

	output, err := signal.decay.Measure(input)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"exhaust: failed to measure decay",
			err,
		))
	}

	measurements := signal.frame(
		row.Symbol,
		row.Timestamp,
		output,
		maturity,
	)
	measurement := measurements[0]
	measurement.PutMetric(types.MetricTradePrice, types.SideNone, types.MetricSample{
		Raw:  row.Price.Float64(),
		Unit: types.UnitQuoteCurrency,
	})
	measurement.PutMetric(types.MetricTradeQuantity, types.SideNone, types.MetricSample{
		Raw:  row.Qty,
		Unit: types.UnitBaseCurrency,
	})

	return measurements, nil
}

func validTrade(row kraken.TradeData) bool {
	return row.Symbol != "" && !row.Timestamp.IsZero() && row.Price.Sign() > 0 &&
		row.Qty > 0 && (row.Side == "buy" || row.Side == "sell")
}

func (signal *Signal) seenTrade(row kraken.TradeData) bool {
	raw, exists := signal.lastTrade.Load(row.Symbol)

	if !exists {
		return false
	}

	previous := raw.(tradeCursor)

	if row.Timestamp.Before(previous.at) {
		return true
	}

	if row.Timestamp.After(previous.at) {
		return false
	}

	_, seen := previous.ids[row.TradeID]

	return seen
}

func (signal *Signal) commitTrade(row kraken.TradeData) {
	previous := tradeCursor{}
	raw, exists := signal.lastTrade.Load(row.Symbol)

	if exists {
		previous = raw.(tradeCursor)
	}

	if row.Timestamp.After(previous.at) {
		previous = tradeCursor{at: row.Timestamp, ids: make(map[int64]struct{})}
	}

	if previous.ids == nil {
		previous.ids = make(map[int64]struct{})
	}

	previous.ids[row.TradeID] = struct{}{}
	signal.lastTrade.Store(row.Symbol, previous)
}

func (signal *Signal) bookAt(symbol string) (time.Time, bool) {
	raw, exists := signal.lastBookAt.Load(symbol)

	if !exists {
		return time.Time{}, false
	}

	return raw.(time.Time), true
}

func (signal *Signal) bookSnapshot(symbol string) bookSnapshot {
	raw, exists := signal.lastBook.Load(symbol)

	if !exists {
		return bookSnapshot{}
	}

	return raw.(bookSnapshot)
}

func sameSnapshot(current, previous bookSnapshot) bool {
	return sameLevels(current.bids, previous.bids) && sameLevels(current.asks, previous.asks)
}

func sameLevels(current, previous map[int64]flow.BookLevel) bool {
	if len(current) != len(previous) {
		return false
	}

	for ticks, level := range current {
		prior, ok := previous[ticks]

		if !ok || prior.Price != level.Price || prior.Quantity != level.Quantity {
			return false
		}
	}

	return true
}

func snapshotDelta(
	current, previous map[int64]flow.BookLevel,
) []flow.BookLevel {
	levels := make([]flow.BookLevel, 0, len(current)+len(previous))

	for _, level := range current {
		levels = append(levels, level)
	}

	for ticks, level := range previous {
		if _, exists := current[ticks]; exists {
			continue
		}

		level.Quantity = 0
		levels = append(levels, level)
	}

	return levels
}

/*
frame converts a decay calculator output into the shared Measurement shape, so
both the book-driven and trade-driven observation paths emit the same metric
set for a symbol.
*/
func (signal *Signal) frame(
	symbol string,
	at time.Time,
	output equation.DecayOutput,
	maturity float64,
) []*types.Measurement {
	metrics, _ := normalizedDecayMetrics(output)

	measurement := &types.Measurement{
		ID:       uuid.NewString(),
		Source:   types.SourceExhaustion,
		Symbol:   symbol,
		At:       at,
		Maturity: maturity,
		Metrics:  metrics,
	}

	return []*types.Measurement{measurement}
}

/*
normalizedDecayMetrics accepts the probability margins emitted by Decay in
[0, 1]. Category is a nominal family identifier, so it deliberately has no
scalar normalization; treating its integer code as magnitude would invent an
ordering between mechanical, fragile, thermal, and reversal exhaustion.
*/
func normalizedDecayMetrics(
	output equation.DecayOutput,
) (map[string]types.MetricSample, bool) {
	type reading struct {
		metric types.MetricType
		side   types.MeasurementSide
		raw    float64
	}

	readings := []reading{
		{types.MetricMechanical, types.SideBuy, output.Long.Mechanical},
		{types.MetricThermal, types.SideBuy, output.Long.Thermal},
		{types.MetricFragile, types.SideBuy, output.Long.Fragile},
		{types.MetricReversal, types.SideBuy, output.Long.Reversal},
		{types.MetricUrgency, types.SideBuy, output.Long.Urgency},
		{types.MetricStrength, types.SideBuy, output.Long.Strength},
		{types.MetricValue, types.SideBuy, output.Long.Value},
		{types.MetricCategory, types.SideBuy, output.Long.Category},
		{types.MetricMechanical, types.SideSell, output.Short.Mechanical},
		{types.MetricThermal, types.SideSell, output.Short.Thermal},
		{types.MetricFragile, types.SideSell, output.Short.Fragile},
		{types.MetricReversal, types.SideSell, output.Short.Reversal},
		{types.MetricUrgency, types.SideSell, output.Short.Urgency},
		{types.MetricStrength, types.SideSell, output.Short.Strength},
		{types.MetricValue, types.SideSell, output.Short.Value},
		{types.MetricCategory, types.SideSell, output.Short.Category},
	}
	metrics := make(map[string]types.MetricSample, len(readings))
	valid := true

	for _, item := range readings {
		sample := types.MetricSample{Raw: item.raw, Unit: types.UnitDimensionless}

		if item.metric == types.MetricCategory {
			if !validDecayCategory(item.raw) {
				valid = false
			}

			metrics[types.MetricKey(item.metric, item.side)] = sample
			continue
		}

		sample.Normalized = normalizedDecayScore(item.raw)

		if sample.Normalized == nil {
			valid = false
		}

		metrics[types.MetricKey(item.metric, item.side)] = sample
	}

	return metrics, valid
}

func normalizedDecayScore(raw float64) *float64 {
	value := raw

	return &value
}

func validDecayCategory(raw float64) bool {
	return finiteDecay(raw) && raw >= 0 && raw <= maximumDecayCategory &&
		raw == math.Trunc(raw)
}

func finiteDecay(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

/*
Close releases the receiver's owned resources so shutdown does not leave
active market-data producers.
*/
func (signal *Signal) Close() error {
	if signal.cancel != nil {
		signal.cancel()
	}

	return nil
}
