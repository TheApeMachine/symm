package hawkes

import (
	"context"
	"encoding/binary"
	"io"
	"math"
	"sync"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/structure"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/symm/kraken/market"
)

const feedRingCapacity = 512

type Trade struct {
	ctx     context.Context
	cancel  context.CancelFunc
	err     error
	entity  string
	scope   string
	symbols *sync.Map
}

func NewTrade(ctx context.Context) *Trade {
	ctx, cancel := context.WithCancel(ctx)

	return &Trade{
		ctx:     ctx,
		cancel:  cancel,
		entity:  "trade",
		symbols: &sync.Map{},
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
			tradeUpdate.Symbol, structure.NewListRing[*market.TradeUpdate](
				feedRingCapacity,
				datura.Acquire(
					"hawkes-trade", datura.Artifact_Type_json,
				).WithRole("trade"),
			),
		)

		ring.(*structure.ListRing[*market.TradeUpdate]).Push(tradeUpdate)
	}
}

func (trade *Trade) Read(p []byte) (int, error) {
	value, ok := trade.symbols.Load(trade.scope)

	if !ok {
		return 0, io.EOF
	}

	ring := value.(*structure.ListRing[*market.TradeUpdate])
	payload, _, payloadOK := trade.excitationPayload(ring)

	if !payloadOK {
		return 0, io.EOF
	}

	artifact := datura.Acquire("hawkes-trade", datura.Artifact_Type_json)
	artifact.WithRole("trade")
	artifact.WithScope(trade.scope)
	_ = artifact.SetPayload(payload)

	return artifact.Read(p)
}

type TradeSnapshot struct {
	Price    float64
	Volume   float64
	Elapsed  float64
	Observed time.Time
}

func (trade *Trade) Snapshot(symbol string) TradeSnapshot {
	value, ok := trade.symbols.Load(symbol)

	if !ok {
		return TradeSnapshot{}
	}

	ring := value.(*structure.ListRing[*market.TradeUpdate])

	var (
		first  *market.TradeUpdate
		latest *market.TradeUpdate
		volume float64
	)

	ring.Do(func(update *market.TradeUpdate) {
		if update == nil {
			return
		}

		if first == nil {
			first = update
		}

		latest = update
		volume += update.Price * update.Qty
	})

	if latest == nil {
		return TradeSnapshot{}
	}

	elapsed := 0.0

	if first != nil {
		elapsed = latest.Timestamp.Sub(first.Timestamp).Seconds()
	}

	return TradeSnapshot{
		Price:    latest.Price,
		Volume:   volume,
		Elapsed:  elapsed,
		Observed: latest.Timestamp,
	}
}

func (trade *Trade) excitationPayload(
	ring *structure.ListRing[*market.TradeUpdate],
) ([]byte, tradeWindow, bool) {
	var (
		first     *market.TradeUpdate
		latest    *market.TradeUpdate
		buyTimes  []float64
		sellTimes []float64
	)

	ring.Do(func(update *market.TradeUpdate) {
		if update == nil {
			return
		}

		if first == nil {
			first = update
		}

		latest = update
		seconds := float64(update.Timestamp.UnixNano()) / float64(time.Second)

		switch update.Side {
		case "buy":
			buyTimes = append(buyTimes, seconds)
		case "sell":
			sellTimes = append(sellTimes, seconds)
		}
	})

	if latest == nil || len(buyTimes)+len(sellTimes) < 2 {
		return nil, tradeWindow{}, false
	}

	horizon := float64(latest.Timestamp.UnixNano()) / float64(time.Second)
	windowSpan := latest.Timestamp.Sub(first.Timestamp)
	fitCooldown := algorithm.DeriveFitCooldown(windowSpan)

	values := make([]float64, 0, 4+len(buyTimes)+len(sellTimes))
	values = append(values,
		horizon,
		fitCooldown.Seconds(),
		float64(len(buyTimes)),
		float64(len(sellTimes)),
	)
	values = append(values, buyTimes...)
	values = append(values, sellTimes...)

	payload := make([]byte, 8*len(values))

	for index, sample := range values {
		offset := index * 8
		binary.BigEndian.PutUint64(payload[offset:offset+8], math.Float64bits(sample))
	}

	return payload, tradeWindow{
		horizon: latest.Timestamp,
		first:   first.Timestamp,
	}, true
}

type tradeWindow struct {
	horizon time.Time
	first   time.Time
}
