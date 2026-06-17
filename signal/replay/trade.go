package replay

import (
	"sync"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/nomagique/algorithm"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/signal/codec"
	"github.com/theapemachine/symm/signal/toxicity"
)

const tradeWindowCapacity = 256

type scopedTradeWindow struct {
	buyTimes     []float64
	sellTimes    []float64
	buyNotional  float64
	sellNotional float64
	prices       []float64
	horizon      time.Time
	windowStart  time.Time
}

var tradeWindows sync.Map

/*
IngestTradeBatch encodes Hawkes excitation and CVD flow payloads into the shared tree.
*/
func IngestTradeBatch(tree *dmt.Tree, batch *datura.Artifact) {
	if tree == nil || batch == nil {
		return
	}

	for _, update := range datura.As[krakenmarket.TradeUpdates](batch) {
		ingestTradeUpdate(tree, update)
	}
}

func ingestTradeUpdate(tree *dmt.Tree, update *krakenmarket.TradeUpdate) {
	if update == nil || update.Symbol == "" || update.Price <= 0 || update.Qty <= 0 {
		return
	}

	observed := update.Timestamp

	if observed.IsZero() {
		observed = time.Now()
	}

	raw, _ := tradeWindows.LoadOrStore(update.Symbol, &scopedTradeWindow{})
	window := raw.(*scopedTradeWindow)

	seconds := float64(observed.UnixNano()) / float64(time.Second)
	notional := update.Price * update.Qty

	if update.Side == "buy" {
		window.buyTimes = append(window.buyTimes, seconds)
		window.buyNotional += notional
	} else {
		window.sellTimes = append(window.sellTimes, seconds)
		window.sellNotional += notional
	}

	window.prices = append(window.prices, update.Price)

	if window.windowStart.IsZero() || observed.Before(window.windowStart) {
		window.windowStart = observed
	}

	if observed.After(window.horizon) {
		window.horizon = observed
	}

	trimTradeWindow(window)
	insertTradePayloads(tree, update.Symbol, window)

	pair := toxicity.PairFromTick(update.Symbol, tickSize(update.Symbol))
	toxicity.IngestTrade(update.Symbol, pair, update.Price, update.Price*update.Qty, observed)

	vertical := verticalityState(update.Symbol)
	vertical.prices = appendSeries(vertical.prices, update.Price, verticalityCapacity)
	vertical.volumes = appendSeries(vertical.volumes, update.Price*update.Qty, verticalityCapacity)

	if payload := verticalityPayload(update.Symbol, vertical); payload != nil {
		insertScopedPayload(tree, "trade", update.Symbol, codec.EncodePayload(payload...))
	}
}

func trimTradeWindow(window *scopedTradeWindow) {
	if len(window.buyTimes) > tradeWindowCapacity {
		window.buyTimes = window.buyTimes[len(window.buyTimes)-tradeWindowCapacity:]
	}

	if len(window.sellTimes) > tradeWindowCapacity {
		window.sellTimes = window.sellTimes[len(window.sellTimes)-tradeWindowCapacity:]
	}

	if len(window.prices) > tradeWindowCapacity {
		window.prices = window.prices[len(window.prices)-tradeWindowCapacity:]
	}
}

func insertTradePayloads(tree *dmt.Tree, scope string, window *scopedTradeWindow) {
	if window == nil || window.horizon.IsZero() {
		return
	}

	excitationPayload := excitationPayload(window)

	if len(excitationPayload) > 0 {
		insertScopedPayload(tree, "trade", scope, excitationPayload)
	}

	flowPayload := flowPayload(window)

	if len(flowPayload) > 0 {
		insertScopedPayload(tree, "trade", scope, flowPayload)
	}
}

func excitationPayload(window *scopedTradeWindow) []byte {
	if len(window.buyTimes)+len(window.sellTimes) < 4 {
		return nil
	}

	span := window.horizon.Sub(window.windowStart)

	if span <= 0 {
		span = time.Second
	}

	cooldown := algorithm.DeriveFitCooldown(span).Seconds()
	horizonSeconds := float64(window.horizon.UnixNano()) / float64(time.Second)
	samples := []float64{
		horizonSeconds,
		cooldown,
		float64(len(window.buyTimes)),
		float64(len(window.sellTimes)),
	}
	samples = append(samples, window.buyTimes...)
	samples = append(samples, window.sellTimes...)

	payload := codec.EncodePayload(samples...)

	if !codec.ValidExcitationPayload(payload) {
		return nil
	}

	return payload
}

func flowPayload(window *scopedTradeWindow) []byte {
	tradeCount := len(window.prices)

	if tradeCount < 2 {
		return nil
	}

	gross := window.buyNotional + window.sellNotional

	if gross <= 0 {
		return nil
	}

	medianNotional := gross / float64(tradeCount)
	grossFloor := medianNotional

	if grossFloor <= 0 {
		grossFloor = gross / float64(tradeCount)
	}

	samples := []float64{
		window.buyNotional,
		window.sellNotional,
		float64(tradeCount),
		grossFloor,
		medianNotional,
	}
	samples = append(samples, window.prices...)

	payload := codec.EncodePayload(samples...)

	if !codec.ValidFlowPayload(payload) {
		return nil
	}

	return payload
}

func insertScopedPayload(tree *dmt.Tree, role string, scope string, payload []byte) {
	row := datura.Acquire("replay", datura.Artifact_Type_json)

	if row == nil {
		return
	}

	row.WithRole(role)
	row.WithScope(scope)
	row.WithPayload(payload)

	tree.Insert(row.Prefix(), row.Marshal())
	row.Release()
}
