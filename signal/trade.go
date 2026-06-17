package signal

import (
	"context"
	"io"
	"sync"
	"time"

	"github.com/bytedance/sonic/ast"
	"github.com/theapemachine/datura"
)

/*
TradeSnapshot holds input facts for a symbol: data the feed already knows.
*/
type TradeSnapshot struct {
	Price    float64
	Volume   float64
	Elapsed  float64
	Observed time.Time
	Net      float64
}

/*
Trade stores scoped trade updates in per-symbol DMT episodic memory.
OnUpdate runs after each accepted trade update.
*/
type Trade struct {
	ctx          context.Context
	cancel       context.CancelFunc
	Scope        string
	WireProfile  TradeWireProfile
	OnUpdate     func(*TradeRecord)
	symbols      *sync.Map
	grossHistory *sync.Map
}

/*
NewTrade returns a trade feed backed by per-symbol episodic memory.
*/
func NewTrade(ctx context.Context) *Trade {
	ctx, cancel := context.WithCancel(ctx)

	return &Trade{
		ctx:          ctx,
		cancel:       cancel,
		symbols:      &sync.Map{},
		grossHistory: &sync.Map{},
	}
}

/*
TradeWindow holds one symbol's recent trade window.
*/
type TradeWindow struct {
	First   *TradeRecord
	Latest  *TradeRecord
	Prices  []float64
	Volume  float64
	Elapsed float64
}

/*
Window returns the scoped symbol's trade window.
*/
func (trade *Trade) Window(symbol string) (TradeWindow, bool) {
	memory, memoryOK := trade.symbolMemory(symbol)

	if !memoryOK {
		return TradeWindow{}, false
	}

	events := memory.eventsSnapshot()

	var window TradeWindow

	for index := range events {
		record := events[index]

		if record.Price <= 0 || record.Qty <= 0 {
			continue
		}

		if window.First == nil {
			first := record
			window.First = &first
		}

		latest := record
		window.Latest = &latest
		window.Volume += record.Qty
		window.Prices = append(window.Prices, record.Price)
	}

	if window.Latest == nil || len(window.Prices) < 2 {
		return TradeWindow{}, false
	}

	if window.First != nil {
		window.Elapsed = window.Latest.Timestamp.Sub(window.First.Timestamp).Seconds()
	}

	return window, true
}

/*
Scan visits each trade update in the scoped symbol window.
*/
func (trade *Trade) Scan(symbol string, visit func(*TradeRecord)) bool {
	memory, memoryOK := trade.symbolMemory(symbol)

	if !memoryOK {
		return false
	}

	events := memory.eventsSnapshot()

	for index := range events {
		record := events[index]
		visit(&record)
	}

	return true
}

func (trade *Trade) Update(artifact *datura.Artifact) {
	if artifact == nil {
		return
	}

	datura.PayloadEach(artifact, func(index int, element ast.Node) bool {
		symbol, symbolOK := payloadString(element, "symbol")

		if !symbolOK || symbol == "" {
			return true
		}

		price, priceOK := payloadFloat(element, "price")
		qty, qtyOK := payloadFloat(element, "qty")

		if !priceOK || !qtyOK || price <= 0 || qty <= 0 {
			return true
		}

		side, _ := payloadString(element, "side")
		timestamp, timestampOK := payloadTime(element, "timestamp")

		if !timestampOK {
			timestamp = time.Now().UTC()
		}

		record := TradeRecord{
			Symbol:    symbol,
			Side:      side,
			Price:     price,
			Qty:       qty,
			Timestamp: timestamp,
		}

		memory := trade.loadSymbolMemory(symbol)
		timestampNanos := uint64(timestamp.UnixNano())

		if timestampNanos == 0 {
			timestampNanos = uint64(time.Now().UnixNano())
		}

		memory.appendEvent(record, timestampNanos, tradeSensorySequence(record))

		if trade.OnUpdate != nil {
			trade.OnUpdate(&record)
		}

		return true
	})
}

func (trade *Trade) Read(buffer []byte) (int, error) {
	if trade.Scope == "" {
		return 0, io.EOF
	}

	if trade.WireProfile != TradeWireEvent {
		artifact := trade.batchArtifact(trade.Scope)

		if artifact == nil {
			return 0, io.EOF
		}

		return ReadFeatureArtifact(buffer, artifact)
	}

	memory, memoryOK := trade.symbolMemory(trade.Scope)

	if !memoryOK {
		return 0, io.EOF
	}

	return memory.readNext(buffer, trade.Scope, func(record TradeRecord) ([]byte, error) {
		return marshalRecordJSON(record)
	})
}

/*
Snapshot returns the scoped symbol's latest input facts.
*/
func (trade *Trade) Snapshot(symbol string) TradeSnapshot {
	window, windowOK := trade.Window(symbol)

	if !windowOK || window.Latest == nil {
		return TradeSnapshot{}
	}

	batch, batchOK := trade.flowBatch(symbol)
	net := 0.0

	if batchOK {
		net = batch.Net
	}

	var notionalVolume float64

	trade.Scan(symbol, func(record *TradeRecord) {
		if record == nil {
			return
		}

		notionalVolume += record.Price * record.Qty
	})

	return TradeSnapshot{
		Price:    window.Latest.Price,
		Volume:   notionalVolume,
		Elapsed:  window.Elapsed,
		Observed: window.Latest.Timestamp,
		Net:      net,
	}
}

func (trade *Trade) ResetReadHead() {
	trade.symbols.Range(func(key, value any) bool {
		memory, memoryOK := value.(*symbolEpisodicMemory[TradeRecord])

		if memoryOK {
			memory.resetReadHead()
		}

		return true
	})
}

func (trade *Trade) loadSymbolMemory(symbol string) *symbolEpisodicMemory[TradeRecord] {
	value, _ := trade.symbols.LoadOrStore(
		symbol,
		newSymbolEpisodicMemory[TradeRecord]("trade", "trade", FeedRingCapacity()),
	)

	return value.(*symbolEpisodicMemory[TradeRecord])
}

func (trade *Trade) symbolMemory(symbol string) (*symbolEpisodicMemory[TradeRecord], bool) {
	value, ok := trade.symbols.Load(symbol)

	if !ok {
		return nil, false
	}

	memory, memoryOK := value.(*symbolEpisodicMemory[TradeRecord])

	return memory, memoryOK
}

func (trade *Trade) Close() error {
	trade.cancel()

	return nil
}
