package toxicity

import (
	"context"
	"math"
	"sort"
	"sync"
	"time"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
)

/*
Signal tracks whether near-touch liquidity is sincere, retreating, or bluffing
from Level3 order events corroborated by the public trade tape.
*/
type Signal struct {
	status        types.Status
	ctx           context.Context
	cancel        context.CancelFunc
	api           *websocket.API
	previousTouch map[string]touchSnapshot
	pendingTrades map[string]map[tradeIdentity]kraken.TradeData
	planner       *strategy.Planner
	ui            chan []byte
	subscriptions map[string]*types.Subscription[any]
	subscribers   *sync.Map
	subscribeMu   sync.Mutex
	lastTrade     map[string]tradeCursor
}

type tradeCursor struct {
	at  time.Time
	ids map[int64]struct{}
}

type touchObservation struct {
	price    float64
	quantity float64
}

type touchSnapshot struct {
	asOf time.Time
	bid  touchObservation
	ask  touchObservation
}

type tradeIdentity struct {
	at time.Time
	id int64
}

/*
NewSignal creates the Level3 honesty calculator against the production Kraken
API so tests can replace only its connections, never its market mechanics.
*/
func NewSignal(
	ctx context.Context,
	api *websocket.API,
	planner *strategy.Planner,
	ui chan []byte,
	subscriptions map[string]*types.Subscription[any],
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		status:        types.INITIALIZING,
		ctx:           ctx,
		cancel:        cancel,
		api:           api,
		previousTouch: make(map[string]touchSnapshot),
		pendingTrades: make(map[string]map[tradeIdentity]kraken.TradeData),
		planner:       planner,
		ui:            ui,
		subscriptions: subscriptions,
		subscribers:   &sync.Map{},
		lastTrade:     make(map[string]tradeCursor),
	}

	signal.status = types.READY
	signal.run()
	return signal
}

/*
Name returns the signal source identity.
*/
func (signal *Signal) Name() string {
	return string(types.SourceToxicity)
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

/*
onTrade attributes public executions to the resting side they consumed while
retaining the same Level3 touch evidence used by book-only observations.
*/
func (signal *Signal) run() {
	go func() {
		for {
			select {
			case <-signal.ctx.Done():
				return
			case message := <-signal.subscriptions["thesis"].Channel:
				if thesis, ok := message.(*types.Thesis); ok {
					measurements := signal.Measure(thesis)

					if len(measurements) > 0 {
						thesis.Measurements.Store(
							types.SourceToxicity,
							measurements,
						)

						thesis.Readiness.Toxicity = true
						utils.Fanout(signal.subscribers, signal.Name(), thesis)
					}
				}
			}
		}
	}()
}

func (signal *Signal) Measure(thesis *types.Thesis) []*types.Measurement {
	trades := thesis.MarketTrades()
	books := thesis.Books
	measurements := make([]*types.Measurement, 0)
	out := make([]*types.Measurement, 0)
	currentTouches := make(map[string]touchSnapshot)
	hadPrevious := make(map[string]bool)

	books.Range(func(key, value any) bool {
		symbol, ok := key.(string)

		if !ok {
			return true
		}

		managed, ok := value.(*spotbook.Book)

		if !ok {
			return true
		}

		current, ok := observedTouch(managed)

		if !ok {
			return true
		}

		currentTouches[symbol] = current
		_, hadPrevious[symbol] = signal.previousTouch[symbol]

		if !hadPrevious[symbol] {
			signal.previousTouch[symbol] = current
		}

		return true
	})

	for _, trade := range trades {
		if !validTrade(trade) || signal.seenTrade(trade) {
			continue
		}

		previous, exists := signal.previousTouch[trade.Symbol]

		if !exists || !trade.Timestamp.After(previous.asOf) {
			continue
		}

		signal.queueTrade(trade)
	}

	for symbol, current := range currentTouches {
		if !hadPrevious[symbol] {
			continue
		}

		previous := signal.previousTouch[symbol]

		if !current.asOf.After(previous.asOf) {
			continue
		}

		bracketed := signal.bracketedTrades(symbol, previous.asOf, current.asOf)
		signal.previousTouch[symbol] = current

		if len(bracketed) == 0 {
			continue
		}

		measurement := toxicityMeasurement(symbol, previous, current, bracketed)
		measurements = append(measurements, measurement)

		for _, trade := range bracketed {
			signal.commitTrade(trade)
			signal.removePending(trade)
		}

		if measurement.Symbol == types.Focus() {
			out = append(out, measurement)
		}
	}

	if len(out) > 0 {
		utils.Publish(signal.ui, datura.NewMap(
			"measurements", out,
		))
	}

	return measurements
}

func observedTouch(managed *spotbook.Book) (touchSnapshot, bool) {
	if managed == nil {
		return touchSnapshot{}, false
	}

	bid := managed.BestBid()
	ask := managed.BestAsk()

	if bid == nil || ask == nil || bid.Price == nil || ask.Price == nil ||
		bid.Quantity == nil || ask.Quantity == nil {
		return touchSnapshot{}, false
	}

	bidPrice := bid.Price.Float64()
	askPrice := ask.Price.Float64()
	bidQuantity := bid.Quantity.Float64()
	askQuantity := ask.Quantity.Float64()
	asOf := bid.Timestamp

	if ask.Timestamp.After(asOf) {
		asOf = ask.Timestamp
	}

	if asOf.IsZero() || bidPrice <= 0 || askPrice <= bidPrice || bidQuantity < 0 ||
		askQuantity < 0 || !finite(bidPrice) || !finite(askPrice) ||
		!finite(bidQuantity) || !finite(askQuantity) {
		return touchSnapshot{}, false
	}

	return touchSnapshot{
		asOf: asOf,
		bid:  touchObservation{price: bidPrice, quantity: bidQuantity},
		ask:  touchObservation{price: askPrice, quantity: askQuantity},
	}, true
}

func (signal *Signal) queueTrade(trade kraken.TradeData) {
	if signal.pendingTrades == nil {
		signal.pendingTrades = make(map[string]map[tradeIdentity]kraken.TradeData)
	}

	if signal.pendingTrades[trade.Symbol] == nil {
		signal.pendingTrades[trade.Symbol] = make(map[tradeIdentity]kraken.TradeData)
	}

	identity := tradeIdentity{at: trade.Timestamp, id: trade.TradeID}
	signal.pendingTrades[trade.Symbol][identity] = trade
}

func (signal *Signal) bracketedTrades(
	symbol string,
	from time.Time,
	through time.Time,
) []kraken.TradeData {
	bracketed := make([]kraken.TradeData, 0)

	for _, trade := range signal.pendingTrades[symbol] {
		if trade.Timestamp.After(from) && !trade.Timestamp.After(through) {
			bracketed = append(bracketed, trade)
		}
	}

	sort.SliceStable(bracketed, func(leftIndex, rightIndex int) bool {
		left := bracketed[leftIndex]
		right := bracketed[rightIndex]

		if left.Timestamp.Equal(right.Timestamp) {
			return left.TradeID < right.TradeID
		}

		return left.Timestamp.Before(right.Timestamp)
	})

	return bracketed
}

func (signal *Signal) removePending(trade kraken.TradeData) {
	delete(signal.pendingTrades[trade.Symbol], tradeIdentity{
		at: trade.Timestamp,
		id: trade.TradeID,
	})
}

func toxicityMeasurement(
	symbol string,
	previous touchSnapshot,
	current touchSnapshot,
	trades []kraken.TradeData,
) *types.Measurement {
	tradeVolume := 0.0
	bidFill := 0.0
	askFill := 0.0

	for _, trade := range trades {
		tradeVolume += trade.Qty

		if trade.Side == "sell" && trade.Price.Float64() == previous.bid.price {
			bidFill += trade.Qty
		}

		if trade.Side == "buy" && trade.Price.Float64() == previous.ask.price {
			askFill += trade.Qty
		}
	}

	bidFill = math.Min(bidFill, previous.bid.quantity)
	askFill = math.Min(askFill, previous.ask.quantity)
	bidRetreat, bidCancelled := touchChange(types.SideBuy, previous.bid, current.bid, bidFill)
	askRetreat, askCancelled := touchChange(types.SideSell, previous.ask, current.ask, askFill)
	touchQuantity := current.bid.quantity + current.ask.quantity

	return &types.Measurement{
		Source:       types.SourceToxicity,
		Symbol:       symbol,
		At:           current.asOf,
		ObservedFrom: previous.asOf,
		Horizon:      current.asOf.Sub(previous.asOf),
		Validity:     types.ObservationValidity(len(trades) + 2),
		Metrics: map[string]types.MetricSample{
			types.MetricKey(types.MetricTradeVolume, types.SideNone): {
				Raw:  tradeVolume,
				Unit: types.UnitBaseCurrency,
			},
			types.MetricKey(types.MetricFillVolume, types.SideBuy): {
				Raw:  bidFill,
				Unit: types.UnitBaseCurrency,
			},
			types.MetricKey(types.MetricFillVolume, types.SideSell): {
				Raw:  askFill,
				Unit: types.UnitBaseCurrency,
			},
			types.MetricKey(types.MetricBestPrice, types.SideBuy): {
				Raw:  current.bid.price,
				Unit: types.UnitQuoteCurrency,
			},
			types.MetricKey(types.MetricBestPrice, types.SideSell): {
				Raw:  current.ask.price,
				Unit: types.UnitQuoteCurrency,
			},
			types.MetricKey(types.MetricTouchQuantity, types.SideBuy): {
				Raw:        current.bid.quantity,
				Normalized: types.NormalizeRatio(current.bid.quantity, touchQuantity),
				Unit:       types.UnitBaseCurrency,
			},
			types.MetricKey(types.MetricTouchQuantity, types.SideSell): {
				Raw:        current.ask.quantity,
				Normalized: types.NormalizeRatio(current.ask.quantity, touchQuantity),
				Unit:       types.UnitBaseCurrency,
			},
			types.MetricKey(types.MetricRetreatingQuantity, types.SideBuy): {
				Raw:        bidRetreat,
				Normalized: types.NormalizeRatio(bidRetreat, current.bid.quantity+bidRetreat),
				Unit:       types.UnitBaseCurrency,
			},
			types.MetricKey(types.MetricRetreatingQuantity, types.SideSell): {
				Raw:        askRetreat,
				Normalized: types.NormalizeRatio(askRetreat, current.ask.quantity+askRetreat),
				Unit:       types.UnitBaseCurrency,
			},
			types.MetricKey(types.MetricCancelledQuantity, types.SideBuy): {
				Raw:  bidCancelled,
				Unit: types.UnitBaseCurrency,
			},
			types.MetricKey(types.MetricCancelledQuantity, types.SideSell): {
				Raw:  askCancelled,
				Unit: types.UnitBaseCurrency,
			},
		},
	}
}

func touchChange(
	side types.MeasurementSide,
	previous touchObservation,
	current touchObservation,
	executed float64,
) (float64, float64) {
	if previous.quantity <= 0 {
		return 0, 0
	}

	if side == types.SideBuy && current.price < previous.price {
		return math.Max(0, previous.quantity-executed), 0
	}

	if side == types.SideSell && current.price > previous.price {
		return math.Max(0, previous.quantity-executed), 0
	}

	if current.price != previous.price || current.quantity >= previous.quantity {
		return 0, 0
	}

	disappeared := previous.quantity - current.quantity

	return 0, math.Max(0, disappeared-executed)
}

func validTrade(row kraken.TradeData) bool {
	price := row.Price.Float64()

	return row.Symbol != "" && !row.Timestamp.IsZero() && price > 0 && row.Qty > 0 &&
		!math.IsNaN(price) && !math.IsInf(price, 0) && !math.IsNaN(row.Qty) &&
		!math.IsInf(row.Qty, 0) && (row.Side == "buy" || row.Side == "sell")
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
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

/*
Close releases the receiver's owned resources so shutdown does not leave
active market-data producers.
*/
func (signal *Signal) Close() error {
	signal.cancel()
	return nil
}
