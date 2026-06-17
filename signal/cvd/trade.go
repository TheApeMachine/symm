package cvd

import (
	"context"
	"encoding/binary"
	"io"
	"math"
	"sync"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/structure"
	"github.com/theapemachine/nomagique/statistic"
	"github.com/theapemachine/symm/kraken/market"
	feed "github.com/theapemachine/symm/signal"
	"gonum.org/v1/gonum/stat"
)

const (
	grossHistoryCap  = 64
	minGrossHistory  = 8
)

type Trade struct {
	ctx          context.Context
	cancel       context.CancelFunc
	err          error
	entity       string
	scope        string
	symbols      *sync.Map
	grossHistory *sync.Map
}

func NewTrade(ctx context.Context) *Trade {
	ctx, cancel := context.WithCancel(ctx)

	return &Trade{
		ctx:          ctx,
		cancel:       cancel,
		entity:       "trade",
		symbols:      &sync.Map{},
		grossHistory: &sync.Map{},
	}
}

func (trade *Trade) Entity() string {
	return trade.entity
}

func (trade *Trade) Update(update market.TradeUpdates) {
	for _, tradeUpdate := range update {
		if tradeUpdate == nil || tradeUpdate.Symbol == "" {
			continue
		}

		ring, _ := trade.symbols.LoadOrStore(
			tradeUpdate.Symbol, structure.NewClockRing[*market.TradeUpdate](
				10, 100, 1000,
				datura.Acquire(
					"cvd", datura.Artifact_Type_json,
				).WithRole("trade"),
			),
		)

		ring.(*structure.ClockRing[*market.TradeUpdate]).ObserveSecond(tradeUpdate.Timestamp, tradeUpdate)
	}
}

func (trade *Trade) Artifact() *datura.Artifact {
	value, ok := trade.symbols.Load(trade.scope)

	if !ok {
		return nil
	}

	ring := value.(*structure.ClockRing[*market.TradeUpdate])
	batch, batchOK := trade.windowBatch(ring)

	if !batchOK {
		return nil
	}

	const cvdHeaderFloats = 5

	maxFloats := feed.MaxFeatureFloats(
		"cvd",
		"trade",
		trade.scope,
		cvdHeaderFloats,
	)
	maxPrices := maxFloats - cvdHeaderFloats

	if maxPrices < 2 {
		return nil
	}

	if len(batch.prices) > maxPrices {
		batch.prices = feed.TrimOldestFloats(batch.prices, maxPrices)
	}

	artifact := datura.Acquire("cvd", datura.Artifact_Type_json)
	artifact.WithRole("trade")
	artifact.WithScope(trade.scope)
	artifact.WithPayload(encodeFlowBatch(batch))

	return artifact
}

func (trade *Trade) Read(p []byte) (int, error) {
	artifact := trade.Artifact()

	if artifact == nil {
		return 0, io.EOF
	}

	return feed.ReadFeatureArtifact(p, artifact)
}

type windowBatch struct {
	buyNotional    float64
	sellNotional   float64
	tradeCount     float64
	grossFloor     float64
	medianNotional float64
	prices         []float64
	volume         float64
	elapsed        float64
	observed       time.Time
	net            float64
}

func (trade *Trade) windowBatch(
	ring *structure.ClockRing[*market.TradeUpdate],
) (windowBatch, bool) {
	var (
		first        *market.TradeUpdate
		latest       *market.TradeUpdate
		buyNotional  float64
		sellNotional float64
		volume       float64
		prices       []float64
		notionals    []float64
		tradeCount   int
	)

	ring.Do(func(slot structure.ClockSlot[*market.TradeUpdate]) {
		update := slot.Payload

		if update == nil {
			return
		}

		if first == nil {
			first = update
		}

		latest = update
		tradeCount++

		notional := update.Price * update.Qty
		notionals = append(notionals, notional)
		prices = append(prices, update.Price)
		volume += notional

		if update.Side == "buy" {
			buyNotional += notional
		}

		if update.Side == "sell" {
			sellNotional += notional
		}
	})

	if latest == nil || tradeCount < 2 || len(prices) < 2 {
		return windowBatch{}, false
	}

	gross := buyNotional + sellNotional

	if gross <= 0 {
		return windowBatch{}, false
	}

	trade.recordGross(trade.scope, gross)

	elapsed := 0.0

	if first != nil {
		elapsed = latest.Timestamp.Sub(first.Timestamp).Seconds()
	}

	medianValue := statistic.MedianOf(notionals)

	return windowBatch{
		buyNotional:    buyNotional,
		sellNotional:   sellNotional,
		tradeCount:     float64(tradeCount),
		grossFloor:     trade.grossFloor(trade.scope),
		medianNotional: medianValue,
		prices:         prices,
		volume:         volume,
		elapsed:        elapsed,
		observed:       latest.Timestamp,
		net:            buyNotional - sellNotional,
	}, true
}

func encodeFlowBatch(batch windowBatch) []byte {
	values := make([]float64, 0, 5+len(batch.prices))
	values = append(values,
		batch.buyNotional,
		batch.sellNotional,
		batch.tradeCount,
		batch.grossFloor,
		batch.medianNotional,
	)
	values = append(values, batch.prices...)

	payload := make([]byte, 8*len(values))

	for index, sample := range values {
		offset := index * 8
		binary.BigEndian.PutUint64(
			payload[offset:offset+8],
			math.Float64bits(sample),
		)
	}

	return payload
}

/*
TradeSnapshot holds input facts for the scoped symbol.
*/
type TradeSnapshot struct {
	Price    float64
	Volume   float64
	Elapsed  float64
	Observed time.Time
	Net      float64
}

func (trade *Trade) Snapshot(symbol string) TradeSnapshot {
	value, ok := trade.symbols.Load(symbol)

	if !ok {
		return TradeSnapshot{}
	}

	ring := value.(*structure.ClockRing[*market.TradeUpdate])
	batch, ok := trade.windowBatch(ring)

	if !ok {
		return TradeSnapshot{}
	}

	price := batch.prices[len(batch.prices)-1]

	return TradeSnapshot{
		Price:    price,
		Volume:   batch.volume,
		Elapsed:  batch.elapsed,
		Observed: batch.observed,
		Net:      batch.net,
	}
}

func (trade *Trade) recordGross(symbol string, gross float64) {
	if gross <= 0 {
		return
	}

	value, _ := trade.grossHistory.LoadOrStore(symbol, make([]float64, 0, grossHistoryCap))
	history := value.([]float64)
	history = append(history, gross)

	if len(history) > grossHistoryCap {
		history = history[len(history)-grossHistoryCap:]
	}

	trade.grossHistory.Store(symbol, history)
}

func (trade *Trade) grossFloor(symbol string) float64 {
	value, ok := trade.grossHistory.Load(symbol)

	if !ok {
		return 0
	}

	history := value.([]float64)

	if len(history) < minGrossHistory {
		return 0
	}

	return stat.Quantile(0.1, stat.LinInterp, history, nil)
}
