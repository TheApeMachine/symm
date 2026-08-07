package depthflow

import (
	"context"
	"math"
	"runtime"
	"sort"
	"sync"
	"time"

	"github.com/alitto/pond/v2"
	"github.com/spf13/viper"
	spotbook "github.com/theapemachine/api-go/v2/pkg/book"
	"github.com/theapemachine/api-go/v2/pkg/decimal"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"

	"github.com/theapemachine/nomagique/algorithm/book/flow"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
)

/*
Signal is the "Weight of the Book" perspective, measuring touch-level book
imbalance with trade-pressure confirmation. Categories belong in logic; this
signal emits numerical scores only.
*/
type Signal struct {
	status        types.Status
	ctx           context.Context
	cancel        context.CancelFunc
	books         websocket.BookSource
	instrument    *broker.Instrument
	sample        *flow.Sample
	bookflow      *equation.Bookflow
	ui            chan []byte
	subscriptions map[string]*types.Subscription[any]
	subscribers   *sync.Map
	subscribeMu   sync.Mutex
	lastTrade     *sync.Map
	lastBookAt    *sync.Map
	lastBook      *sync.Map
	pool          pond.Pool
	group         pond.TaskGroup
}

type tradeCursor struct {
	at  time.Time
	ids map[int64]struct{}
}

type bookSnapshot struct {
	bids map[int64]flow.BookLevel
	asks map[int64]flow.BookLevel
}

/*
NewSignal creates depth-flow state shared by the causally ordered book and
trade observations in each central market cut.
*/
func NewSignal(
	ctx context.Context,
	books websocket.BookSource,
	instrument *broker.Instrument,
	ui chan []byte,
	subscriptions map[string]*types.Subscription[any],
) *Signal {
	ctx, cancel := context.WithCancel(ctx)
	sample, err := flow.NewSample(viper.GetViper().GetInt("signals.depthflow.sampleSize"))

	if err != nil {
		cancel()
		errnie.Error(errnie.Err(
			errnie.Validation,
			"depthflow: failed to create flow sample",
			err,
		))
		return nil
	}

	signal := &Signal{
		status:        types.INITIALIZING,
		ctx:           ctx,
		cancel:        cancel,
		books:         books,
		instrument:    instrument,
		sample:        sample,
		bookflow:      equation.NewBookflow(),
		ui:            ui,
		subscriptions: subscriptions,
		subscribers:   &sync.Map{},
		lastTrade:     &sync.Map{},
		lastBookAt:    &sync.Map{},
		lastBook:      &sync.Map{},
		pool:          pond.NewPool(runtime.GOMAXPROCS(0)),
	}
	signal.group = signal.pool.NewGroup()
	signal.status = types.READY
	signal.run()
	return signal
}

/*
Name returns the signal source identity.
*/
func (signal *Signal) Name() string {
	return string(types.SourceDepthFlow)
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
					}

					utils.Fanout(signal.subscribers, signal.Name(), thesis)
				}
			}
		}
	}()
}

func (signal *Signal) Measure(thesis *types.Thesis) []*types.Measurement {
	trades := thesis.MarketTrades()
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
	symbols := thesis.MarketSymbols()
	symbolSet := make(map[string]struct{}, len(symbols))

	for _, symbol := range symbols {
		symbolSet[symbol] = struct{}{}
	}

	sort.Strings(symbols)
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

	if signal.pool == nil {
		signal.pool = pond.NewPool(runtime.GOMAXPROCS(0))
	}

	if signal.group == nil {
		signal.group = signal.pool.NewGroup()
	}

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
		managed := signal.books.Book(symbol)
		bookAt := managedBookObservedAt(managed)

		signal.group.Submit(func() {
			symbolMeasurements := make([]*types.Measurement, 0)
			symbolOut := make([]*types.Measurement, 0)
			bookPending := managed != nil

			for _, trade := range symbolTrades {
				if bookPending && !trade.Timestamp.Before(bookAt) {
					bookMeasurements, err := signal.measureManagedBook(managed)

					if err != nil {
						errnie.Error(errnie.Err(
							errnie.UnprocessableContent,
							"depthflow: failed to measure book",
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
						"depthflow: failed to measure trade",
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
						"depthflow: failed to measure book",
						err,
					))
				}

				symbolMeasurements = append(symbolMeasurements, bookMeasurements...)
			}

			results.Store(symbol, symbolMeasurements)
			publish.Store(symbol, symbolOut)
		})
	}

	if err := signal.group.Wait(); err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"depthflow: parallel measurement failed",
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

	input, ready, maturity, err := signal.sample.MeasureBook(flow.BookInput{
		Symbol:   managed.Name,
		TickSize: instrument.TickSize.Float64(),
		Bids:     snapshotDelta(current.bids, previous.bids),
		Asks:     snapshotDelta(current.asks, previous.asks),
	})
	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"depthflow: failed to measure book",
			err,
		))
	}

	signal.lastBook.Store(managed.Name, current)
	signal.lastBookAt.Store(managed.Name, observedAt)

	if !ready {
		return nil, nil
	}

	output, err := signal.bookflow.Measure(input)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"depthflow: failed to measure bookflow",
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
measureTrade applies one trade event to the shared flow sample at its causal
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
			"depthflow: failed to measure trade",
			err,
		))
	}

	if !ready {
		return nil, nil
	}

	output, err := signal.bookflow.Measure(input)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"depthflow: failed to measure bookflow",
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
frame converts a bookflow calculator output into one source×symbol row so both
the book-driven and trade-driven observation paths emit the same metric set.
*/
func (signal *Signal) frame(
	symbol string, at time.Time,
	output equation.BookflowOutput,
	maturity float64,
) []*types.Measurement {
	loaded := normalizedBookflowScore(types.MetricLoadedScore, output.LoadedScore, output.Category)
	spoof := normalizedBookflowScore(types.MetricSpoofScore, output.SpoofScore, output.Category)
	thin := normalizedBookflowScore(types.MetricThinScore, output.ThinScore, output.Category)
	neutral := normalizedBookflowScore(types.MetricNeutralScore, output.NeutralScore, output.Category)
	strength := normalizedBookflowScore(types.MetricStrength, output.Strength, output.Category)
	value := normalizedBookflowScore(types.MetricValue, output.Value, output.Category)

	if loaded == nil || spoof == nil || thin == nil || neutral == nil ||
		strength == nil || value == nil {
	}

	measurement := &types.Measurement{
		Source:   types.SourceDepthFlow,
		Symbol:   symbol,
		At:       at,
		Maturity: maturity,
		Metrics: map[string]types.MetricSample{
			types.MetricKey(types.MetricLoadedScore, types.SideNone): {
				Raw:        output.LoadedScore,
				Normalized: loaded,
				Unit:       types.UnitDimensionless,
			},
			types.MetricKey(types.MetricSpoofScore, types.SideNone): {
				Raw:        output.SpoofScore,
				Normalized: spoof,
				Unit:       types.UnitDimensionless,
			},
			types.MetricKey(types.MetricThinScore, types.SideNone): {
				Raw:        output.ThinScore,
				Normalized: thin,
				Unit:       types.UnitDimensionless,
			},
			types.MetricKey(types.MetricNeutralScore, types.SideNone): {
				Raw:        output.NeutralScore,
				Normalized: neutral,
				Unit:       types.UnitDimensionless,
			},
			types.MetricKey(types.MetricStrength, types.SideNone): {
				Raw:        output.Strength,
				Normalized: strength,
				Unit:       types.UnitDimensionless,
			},
			types.MetricKey(types.MetricValue, types.SideNone): {
				Raw:        output.Value,
				Normalized: value,
				Unit:       types.UnitDimensionless,
			},
		},
	}

	return []*types.Measurement{measurement}
}

const (
	bookflowSpoofCategory    = 2.0
	maxBookImbalanceContrast = 2.0
)

/*
normalizedBookflowScore preserves the equation's bounded depth fractions. A
spoof score is the absolute difference of two imbalances in [-1, 1], so its
domain-derived maximum contrast is two; the winning strength/value use that
same scale only for the spoof category.
*/
func normalizedBookflowScore(
	metric types.MetricType,
	raw float64,
	category float64,
) *float64 {
	if math.IsNaN(raw) || math.IsInf(raw, 0) || raw < 0 {
		return nil
	}

	value := raw

	switch metric {
	case types.MetricSpoofScore:
		if raw > maxBookImbalanceContrast {
			return nil
		}

		value = raw / maxBookImbalanceContrast
	case types.MetricStrength, types.MetricValue:
		if category == bookflowSpoofCategory {
			if raw > maxBookImbalanceContrast {
				return nil
			}

			value = raw / maxBookImbalanceContrast
			break
		}

		if raw > 1 {
			return nil
		}
	case types.MetricLoadedScore,
		types.MetricThinScore,
		types.MetricNeutralScore:
		if raw > 1 {
			return nil
		}
	default:
		return nil
	}

	return &value
}

/*
Close releases the receiver's owned resources so shutdown does not leave
active market-data producers.
*/
func (signal *Signal) Close() error {
	if signal.cancel != nil {
		signal.cancel()
	}

	if signal.pool != nil {
		signal.pool.StopAndWait()
	}

	return nil
}
