package exhaust

import (
	"encoding/binary"
	"math"
	"sync"

	"github.com/theapemachine/datura"
	nomadaptive "github.com/theapemachine/nomagique/adaptive"
	feed "github.com/theapemachine/symm/signal"
)

const featureRingCapacity = 24

/*
CrossSection accumulates per-symbol microstructure feature rings for decay scoring.
*/
type CrossSection struct {
	universe sync.Map
	capacity int
}

type featureState struct {
	snapshot    *feed.BookRecord
	bidDepths   []float64
	askDepths   []float64
	densities   []float64
	spreads     []float64
	pressures   []float64
	imbalances  []float64
	pressureEMA *nomadaptive.EMA
	lastPrice   float64
}

/*
NewCrossSection returns a cross-section store with capped float rings.
*/
func NewCrossSection(capacity int) *CrossSection {
	if capacity < 4 {
		capacity = featureRingCapacity
	}

	return &CrossSection{capacity: capacity}
}

func (crossSection *CrossSection) ensure(symbol string) *featureState {
	raw, _ := crossSection.universe.LoadOrStore(symbol, &featureState{
		pressureEMA: nomadaptive.NewEMA(),
	})

	state, ok := raw.(*featureState)

	if !ok {
		return nil
	}

	return state
}

func (crossSection *CrossSection) observeBook(book *feed.BookRecord) {
	if book == nil || book.Symbol == "" {
		return
	}

	state := crossSection.ensure(book.Symbol)

	if state == nil {
		return
	}

	if len(book.Bids) > 0 && len(book.Asks) > 0 {
		state.snapshot = book
	}

	if state.snapshot == nil {
		return
	}

	bids := book.Bids
	asks := book.Asks

	if len(bids) == 0 {
		bids = state.snapshot.Bids
	}

	if len(asks) == 0 {
		asks = state.snapshot.Asks
	}

	if len(bids) == 0 || len(asks) == 0 {
		return
	}

	bidDepth := crossSection.sideDepth(bids)
	askDepth := crossSection.sideDepth(asks)

	midPrice := (bids[0].Price + asks[0].Price) / 2
	touchSpread := asks[0].Price - bids[0].Price
	depth := bids[0].Qty + asks[0].Qty

	if bidDepth > 0 {
		pushRing(&state.bidDepths, bidDepth, crossSection.capacity)
	}

	if askDepth > 0 {
		pushRing(&state.askDepths, askDepth, crossSection.capacity)
	}

	if depth > 0 {
		pushRing(&state.densities, depth, crossSection.capacity)
	}

	spreadBPS := (touchSpread / midPrice) * 10000

	if spreadBPS > 0 {
		pushRing(&state.spreads, spreadBPS, crossSection.capacity)
	}

	imbalance, imbalanceOK := crossSection.level1Imbalance(bids, asks)

	if imbalanceOK {
		pushRing(&state.imbalances, imbalance, crossSection.capacity)
	}

	state.lastPrice = midPrice
}

func (crossSection *CrossSection) observeTrade(trade *feed.TradeRecord) {
	if trade == nil || trade.Symbol == "" {
		return
	}

	state := crossSection.ensure(trade.Symbol)

	if state == nil {
		return
	}

	sign := 0.0

	if trade.Side == "buy" {
		sign = 1
	}

	if trade.Side == "sell" {
		sign = -1
	}

	if sign == 0 {
		return
	}

	smoothed := emaObserve(state.pressureEMA, sign)

	pushRing(&state.pressures, smoothed, crossSection.capacity)

	if trade.Price > 0 {
		state.lastPrice = trade.Price
	}
}

func (crossSection *CrossSection) observeTick(ticker *feed.TickerRecord) {
	if ticker == nil || ticker.Symbol == "" {
		return
	}

	state := crossSection.ensure(ticker.Symbol)

	if state == nil {
		return
	}

	mid := ticker.Last

	if mid <= 0 {
		mid = (ticker.Ask + ticker.Bid) / 2
	}

	spread := ticker.Ask - ticker.Bid

	if mid > 0 && spread > 0 {
		pushRing(&state.spreads, (spread/mid)*10000, crossSection.capacity)
	}

	if mid > 0 {
		state.lastPrice = mid
	}
}

func (crossSection *CrossSection) payload(symbol string) ([]float64, bool) {
	raw, ok := crossSection.universe.Load(symbol)

	if !ok {
		return nil, false
	}

	state, ok := raw.(*featureState)

	if !ok || state.lastPrice <= 0 {
		return nil, false
	}

	series := [][]float64{
		state.bidDepths,
		state.askDepths,
		state.densities,
		state.spreads,
		state.pressures,
		state.imbalances,
	}

	payload := make([]float64, 0, 7+sumLens(series))
	payload = append(payload, state.lastPrice)

	for _, segment := range series {
		payload = append(payload, float64(len(segment)))
	}

	for _, segment := range series {
		payload = append(payload, segment...)
	}

	return payload, true
}

func (crossSection *CrossSection) sideDepth(levels []feed.BookLevelRecord) float64 {
	depth := 0.0

	for _, level := range levels {
		depth += level.Qty
	}

	return depth
}

func (crossSection *CrossSection) level1Imbalance(
	bids, asks []feed.BookLevelRecord,
) (float64, bool) {
	if len(bids) == 0 || len(asks) == 0 {
		return 0, false
	}

	total := bids[0].Qty + asks[0].Qty

	if total <= 0 {
		return 0, false
	}

	return (bids[0].Qty - asks[0].Qty) / total, true
}

func pushRing(buffer *[]float64, value float64, capacity int) {
	*buffer = append(*buffer, value)

	if len(*buffer) > capacity {
		*buffer = (*buffer)[len(*buffer)-capacity:]
	}
}

func sumLens(series [][]float64) int {
	total := 0

	for _, segment := range series {
		total += len(segment)
	}

	return total
}

func emaObserve(ema *nomadaptive.EMA, sample float64) float64 {
	inbound := datura.Acquire("ema-in", datura.Artifact_Type_json)
	payload := make([]byte, 8)
	binary.BigEndian.PutUint64(payload, math.Float64bits(sample))

	frame, marshalErr := inbound.WithPayload(payload).Message().Marshal()

	if marshalErr != nil {
		return sample
	}

	_, _ = ema.Write(frame)
	_, _ = ema.Read(make([]byte, len(frame)))

	return ema.Value()
}
