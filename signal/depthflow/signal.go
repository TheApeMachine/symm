package depthflow

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/spf13/viper"
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
	ctx        context.Context
	cancel     context.CancelFunc
	books      websocket.BookSource
	instrument *broker.Instrument
	sample     *flow.Sample
	bookflow   *equation.Bookflow
	ui         chan []byte
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

/*
NewSignal creates depth-flow state shared by the causally ordered book and
trade observations in each central market cut.
*/
func NewSignal(
	ctx context.Context,
	books websocket.BookSource,
	instrument *broker.Instrument,
	ui chan []byte,
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
		ctx:        ctx,
		cancel:     cancel,
		books:      books,
		instrument: instrument,
		sample:     sample,
		bookflow:   equation.NewBookflow(),
		ui:         ui,
		lastTrade:  &sync.Map{},
		lastBookAt: &sync.Map{},
		lastBook:   &sync.Map{},
	}

	return signal
}

/*
Name returns the signal source identity.
*/
func (signal *Signal) Name() string {
	return string(types.SourceDepthFlow)
}

func (signal *Signal) Type() types.SourceType {
	return types.SourceDepthFlow
}

func (signal *Signal) Measure(symbol *types.Symbol) []*types.Measurement {
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

	signal.books.Book(symbol.Symbol, func(managed *spotbook.Book) {
		bookAt := managedBookObservedAt(managed)
		bookPending := managed != nil

		for trade := range symbol.MarketTrades(types.SourceDepthFlow) {
			if !validTrade(trade) {
				continue
			}

			if bookPending && !trade.Timestamp.Before(bookAt) {
				bookMeasurements, err := signal.measureManagedBook(managed)

				if err != nil {
					errnie.Error(errnie.Err(
						errnie.UnprocessableContent,
						"depthflow: failed to measure book",
						err,
					))
				}

				measurements = append(measurements, bookMeasurements...)
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
			measurements = append(measurements, tradeMeasurements...)

			if symbol.Symbol == types.Focus() {
				out = append(out, tradeMeasurements...)
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

			measurements = append(measurements, bookMeasurements...)
		}
	})

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
			Ticks: decimal.NewFromInt64(0).Add(bid.Price).Div(
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
			Ticks: decimal.NewFromInt64(0).Add(ask.Price).Div(
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
			"depthflow: failed to measure book",
			err,
		))
	}

	signal.lastBook.Store(managed.Name, current)
	signal.lastBookAt.Store(managed.Name, observedAt)

	output, err := signal.bookflow.Measure(input)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"depthflow: failed to measure bookflow",
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
			"depthflow: failed to measure trade",
			err,
		))
	}

	output, err := signal.bookflow.Measure(input)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"depthflow: failed to measure bookflow",
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
frame converts a bookflow calculator output into one source×symbol row so both
the book-driven and trade-driven observation paths emit the same metric set.
*/
func (signal *Signal) frame(
	symbol string, at time.Time,
	output equation.BookflowOutput,
	maturity float64,
) []*types.Measurement {
	loaded := normalizedBookflowScore(types.MetricLoadedScore, output.LoadedScore)
	spoof := normalizedBookflowScore(types.MetricSpoofScore, output.SpoofScore)
	thin := normalizedBookflowScore(types.MetricThinScore, output.ThinScore)
	neutral := normalizedBookflowScore(types.MetricNeutralScore, output.NeutralScore)

	if loaded == nil || spoof == nil || thin == nil || neutral == nil {
		panic("depthflow: bookflow output outside its defined metric domain")
	}

	if !output.Ready {
		loaded = nil
		spoof = nil
		thin = nil
		neutral = nil
	}

	measurement := &types.Measurement{
		ID:       uuid.NewString(),
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
		},
	}
	snr, snrReady := types.MeasurementSignalNoiseRatio(
		types.SourceDepthFlow,
		measurement.Metrics,
	)
	snrSample := types.MetricSample{
		Raw:  snr,
		Unit: types.UnitDimensionless,
	}

	if snrReady {
		snrSample.Normalized = &snr
	}

	measurement.PutMetric(types.MetricSNR, types.SideNone, snrSample)

	return []*types.Measurement{measurement}
}

const maxBookImbalanceContrast = 2.0

/*
normalizedBookflowScore preserves the equation's bounded depth fractions. A
spoof score is the absolute difference of two imbalances in [-1, 1], so its
domain-derived maximum contrast is two.
*/
func normalizedBookflowScore(
	metric types.MetricType,
	raw float64,
) *float64 {
	maximum := 1.0

	if metric == types.MetricSpoofScore {
		maximum = maxBookImbalanceContrast
	}

	if raw < 0 || raw > maximum {
		return nil
	}

	value := raw

	if metric == types.MetricSpoofScore {
		value = raw / maxBookImbalanceContrast
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

	return nil
}
