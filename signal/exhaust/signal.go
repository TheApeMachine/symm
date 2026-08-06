package exhaust

import (
	"context"
	"math"
	"sort"
	"sync"
	"time"

	spotbook "github.com/theapemachine/api-go/v2/pkg/book"
	"github.com/theapemachine/api-go/v2/pkg/decimal"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"

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
	status        types.Status
	ctx           context.Context
	cancel        context.CancelFunc
	books         websocket.BookSource
	instrument    *broker.Instrument
	sample        *algorithm.DecaySample
	decay         *equation.Decay
	ui            chan []byte
	subscriptions map[string]*types.Subscription[any]
	subscribers   *sync.Map
	subscribeMu   sync.Mutex
	lastTrade     map[string]tradeCursor
	lastBookAt    map[string]time.Time
	lastBook      map[string]bookSnapshot
}

type tradeCursor struct {
	at  time.Time
	ids map[int64]struct{}
}

type bookSnapshot struct {
	bids map[int64]flow.BookLevel
	asks map[int64]flow.BookLevel
}

type bookObservation struct {
	managed *spotbook.Book
	at      time.Time
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
	subscriptions map[string]*types.Subscription[any],
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		status:        types.INITIALIZING,
		ctx:           ctx,
		cancel:        cancel,
		books:         books,
		instrument:    instrument,
		sample:        algorithm.NewDecaySample(),
		decay:         equation.NewDecay(),
		ui:            ui,
		subscriptions: subscriptions,
		subscribers:   &sync.Map{},
		lastTrade:     make(map[string]tradeCursor),
		lastBookAt:    make(map[string]time.Time),
		lastBook:      make(map[string]bookSnapshot),
	}
	signal.status = types.READY
	signal.run()
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

func (signal *Signal) Subscribe(
	channel string,
	subscription *types.Subscription[any],
) *types.Subscription[any] {
	return utils.Subscribe(
		signal.subscribers,
		channel,
		subscription,
	)
}

func (signal *Signal) run() {
	thesisSubscription := signal.subscriptions["thesis"]

	if thesisSubscription == nil {
		return
	}

	go func() {
		for {
			select {
			case <-signal.ctx.Done():
				return
			case message := <-thesisSubscription.Channel:
				if thesis, ok := message.(*types.Thesis); ok {
					measurements := signal.Measure(thesis)

					if len(measurements) > 0 {
						thesis.AppendMeasurements(measurements, true)
						utils.Fanout(signal.subscribers, signal.Name(), thesis)
					}
				}
			}
		}
	}()
}

func (signal *Signal) Measure(thesis *types.Thesis) []*types.Measurement {
	trades := thesis.MarketTrades()

	measurements := make([]*types.Measurement, 0)
	out := make([]*types.Measurement, 0)
	bookObservations := make([]bookObservation, 0)

	if signal.books == nil {
		return measurements
	}

	for _, symbol := range thesis.MarketSymbols() {
		managed := signal.books.Book(symbol)

		if managed != nil {
			bookObservations = append(bookObservations, bookObservation{
				managed: managed,
				at:      managedBookObservedAt(managed),
			})
		}
	}

	sort.SliceStable(bookObservations, func(leftIndex, rightIndex int) bool {
		left := bookObservations[leftIndex]
		right := bookObservations[rightIndex]

		if left.at.Equal(right.at) {
			return left.managed.Name < right.managed.Name
		}

		return left.at.Before(right.at)
	})

	bookIndex := 0
	tradeIndex := 0

	for bookIndex < len(bookObservations) || tradeIndex < len(trades) {
		if bookIndex < len(bookObservations) &&
			(tradeIndex == len(trades) ||
				!trades[tradeIndex].Timestamp.Before(bookObservations[bookIndex].at)) {
			bookMeasurements, err := signal.measureManagedBook(
				bookObservations[bookIndex].managed,
			)
			bookIndex++

			if err != nil {
				errnie.Error(errnie.Err(
					errnie.UnprocessableContent,
					"exhaust: failed to measure book",
					err,
				))
				continue
			}

			measurements = append(measurements, bookMeasurements...)
			continue
		}

		trade := trades[tradeIndex]
		tradeIndex++

		if !validTrade(trade) || signal.seenTrade(trade) {
			continue
		}

		bookAt, hasBook := signal.lastBookAt[trade.Symbol]

		if !hasBook || bookAt.IsZero() || bookAt.After(trade.Timestamp) {
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

		measurements = append(measurements, tradeMeasurements...)

		for _, measurement := range tradeMeasurements {
			if measurement.Symbol == types.Focus() {
				out = append(out, measurement)
			}
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

	instrument, err := signal.instrument.Pair(managed.Name)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.NotFound,
			"exhaust: failed to resolve instrument",
			err,
		))
	}

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

	previous := signal.lastBook[managed.Name]

	if sameSnapshot(current, previous) {
		if observedAt.After(signal.lastBookAt[managed.Name]) {
			signal.lastBookAt[managed.Name] = observedAt
		}

		return nil, nil
	}

	if observedAt.Before(signal.lastBookAt[managed.Name]) {
		return nil, nil
	}

	input, ready, maturity, err := signal.sample.MeasureBook(flow.BookInput{
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

	signal.lastBook[managed.Name] = current
	signal.lastBookAt[managed.Name] = observedAt

	if !ready {
		return nil, nil
	}

	output, err := signal.decay.Measure(input)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"exhaust: failed to measure decay",
			err,
		))
	}

	return signal.frame(
		managed.Name,
		observedAt,
		output,
		maturity,
	), nil
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

	input, ready, maturity, err := signal.sample.MeasureTrade(flow.TradeInput{
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

	if !ready {
		return nil, nil
	}

	output, err := signal.decay.Measure(input)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"exhaust: failed to measure decay",
			err,
		))
	}

	return signal.frame(
		row.Symbol,
		row.Timestamp,
		output,
		maturity,
	), nil
}

func validTrade(row kraken.TradeData) bool {
	return row.Symbol != "" && !row.Timestamp.IsZero() && row.Price.Sign() > 0 &&
		row.Qty > 0 && (row.Side == "buy" || row.Side == "sell")
}

func (signal *Signal) seenTrade(row kraken.TradeData) bool {
	previous := signal.lastTrade[row.Symbol]

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
	previous := signal.lastTrade[row.Symbol]

	if row.Timestamp.After(previous.at) {
		previous = tradeCursor{at: row.Timestamp, ids: make(map[int64]struct{})}
	}

	if previous.ids == nil {
		previous.ids = make(map[int64]struct{})
	}

	previous.ids[row.TradeID] = struct{}{}
	signal.lastTrade[row.Symbol] = previous
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
	if math.IsNaN(raw) || math.IsInf(raw, 0) || raw < 0 || raw > 1 {
		return nil
	}

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
	signal.cancel()
	return nil
}
