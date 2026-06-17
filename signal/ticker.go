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
Ticker stores scoped ticker updates in per-symbol DMT episodic memory.
OnUpdate runs after each accepted ticker update.
*/
type Ticker struct {
	ctx      context.Context
	cancel   context.CancelFunc
	Scope    string
	OnUpdate func(*TickerRecord)
	symbols  *sync.Map
}

/*
NewTicker returns a ticker feed backed by per-symbol episodic memory.
*/
func NewTicker(ctx context.Context) *Ticker {
	ctx, cancel := context.WithCancel(ctx)

	return &Ticker{
		ctx:     ctx,
		cancel:  cancel,
		symbols: &sync.Map{},
	}
}

/*
TickerSnapshot holds input facts for a symbol ticker window.
*/
type TickerSnapshot struct {
	Last      float64
	Bid       float64
	Ask       float64
	Volume    float64
	Change    float64
	ChangePct float64
	Observed  time.Time
	Elapsed   float64
}

/*
Snapshot returns the scoped symbol's latest ticker facts.
*/
func (ticker *Ticker) Snapshot(symbol string) TickerSnapshot {
	memory, memoryOK := ticker.symbolMemory(symbol)

	if !memoryOK {
		return TickerSnapshot{}
	}

	events := memory.eventsSnapshot()

	var (
		first  *TickerRecord
		latest *TickerRecord
	)

	for index := range events {
		record := events[index]
		current := record

		if first == nil {
			first = &current
		}

		latest = &current
	}

	if latest == nil {
		return TickerSnapshot{}
	}

	observed := latest.Timestamp
	elapsed := 0.0

	if first != nil && !observed.IsZero() {
		firstAt := first.Timestamp

		if firstAt.IsZero() {
			firstAt = observed
		}

		elapsed = observed.Sub(firstAt).Seconds()
	}

	return TickerSnapshot{
		Last:      latest.Last,
		Bid:       latest.Bid,
		Ask:       latest.Ask,
		Volume:    latest.Volume,
		Change:    latest.Change,
		ChangePct: latest.ChangePct,
		Observed:  observed,
		Elapsed:   elapsed,
	}
}

func (ticker *Ticker) Update(artifact *datura.Artifact) {
	if artifact == nil {
		return
	}

	datura.PayloadEach(artifact, func(index int, element ast.Node) bool {
		symbol, symbolOK := payloadString(element, "symbol")

		if !symbolOK || symbol == "" {
			return true
		}

		record := TickerRecord{Symbol: symbol}
		record.Ask, _ = payloadFloat(element, "ask")
		record.AskQty, _ = payloadFloat(element, "ask_qty")
		record.Bid, _ = payloadFloat(element, "bid")
		record.BidQty, _ = payloadFloat(element, "bid_qty")
		record.Change, _ = payloadFloat(element, "change")
		record.ChangePct, _ = payloadFloat(element, "change_pct")
		record.High, _ = payloadFloat(element, "high")
		record.Last, _ = payloadFloat(element, "last")
		record.Low, _ = payloadFloat(element, "low")
		record.Volume, _ = payloadFloat(element, "volume")
		record.VWAP, _ = payloadFloat(element, "vwap")

		timestamp, timestampOK := payloadTime(element, "timestamp")

		if timestampOK {
			record.Timestamp = timestamp
		}

		memory := ticker.loadSymbolMemory(symbol)
		timestampNanos := uint64(record.Timestamp.UnixNano())

		if timestampNanos == 0 {
			timestampNanos = uint64(index + 1)
		}

		memory.appendEvent(record, timestampNanos, tickerSensorySequence(record))

		if ticker.OnUpdate != nil {
			ticker.OnUpdate(&record)
		}

		return true
	})
}

func (ticker *Ticker) Read(buffer []byte) (int, error) {
	if ticker.Scope == "" {
		return 0, io.EOF
	}

	memory, memoryOK := ticker.symbolMemory(ticker.Scope)

	if !memoryOK {
		return 0, io.EOF
	}

	return memory.readNext(buffer, ticker.Scope, func(record TickerRecord) ([]byte, error) {
		return marshalRecordJSON(record)
	})
}

func (ticker *Ticker) ResetReadHead() {
	ticker.symbols.Range(func(key, value any) bool {
		memory, memoryOK := value.(*symbolEpisodicMemory[TickerRecord])

		if memoryOK {
			memory.resetReadHead()
		}

		return true
	})
}

func (ticker *Ticker) Close() error {
	ticker.cancel()

	return nil
}

func (ticker *Ticker) loadSymbolMemory(symbol string) *symbolEpisodicMemory[TickerRecord] {
	value, _ := ticker.symbols.LoadOrStore(
		symbol,
		newSymbolEpisodicMemory[TickerRecord]("ticker", "ticker", FeedRingCapacity()),
	)

	return value.(*symbolEpisodicMemory[TickerRecord])
}

func (ticker *Ticker) symbolMemory(symbol string) (*symbolEpisodicMemory[TickerRecord], bool) {
	value, ok := ticker.symbols.Load(symbol)

	if !ok {
		return nil, false
	}

	memory, memoryOK := value.(*symbolEpisodicMemory[TickerRecord])

	return memory, memoryOK
}
