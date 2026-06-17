package signal

import (
	"context"
	"io"
	"sync"

	"github.com/bytedance/sonic/ast"
	"github.com/theapemachine/datura"
)

/*
Book stores scoped book updates in per-symbol DMT episodic memory.
OnUpdate runs after each accepted book update.
*/
type Book struct {
	ctx      context.Context
	cancel   context.CancelFunc
	Scope    string
	OnUpdate func(*BookRecord)
	symbols  *sync.Map
}

/*
NewBook returns a book feed backed by per-symbol episodic memory.
*/
func NewBook(ctx context.Context) *Book {
	ctx, cancel := context.WithCancel(ctx)

	return &Book{
		ctx:     ctx,
		cancel:  cancel,
		symbols: &sync.Map{},
	}
}

/*
Spread returns the latest top-of-book spread in basis points for the symbol.
*/
func (book *Book) Spread(symbol string) float64 {
	memory, memoryOK := book.symbolMemory(symbol)

	if !memoryOK {
		return 0
	}

	events := memory.eventsSnapshot()

	var latest *BookRecord

	for index := range events {
		record := events[index]
		latest = &record
	}

	if latest == nil || len(latest.Bids) == 0 || len(latest.Asks) == 0 {
		return 0
	}

	bid := latest.Bids[0].Price
	ask := latest.Asks[0].Price
	mid := (bid + ask) / 2

	if mid <= 0 {
		return 0
	}

	return (ask - bid) / mid * 10000
}

/*
BookWindow holds one symbol's recent book window.
*/
type BookWindow struct {
	Latest  *BookRecord
	Prices  []float64
	Spreads []float64
}

/*
Window returns the scoped symbol's book window.
*/
func (book *Book) Window(symbol string) (BookWindow, bool) {
	memory, memoryOK := book.symbolMemory(symbol)

	if !memoryOK {
		return BookWindow{}, false
	}

	var window BookWindow

	for _, record := range memory.eventsSnapshot() {
		if len(record.Bids) == 0 || len(record.Asks) == 0 {
			continue
		}

		latest := record
		window.Latest = &latest
		spread := record.Asks[0].Price - record.Bids[0].Price

		if spread <= 0 {
			continue
		}

		window.Spreads = append(window.Spreads, spread)

		for _, bid := range record.Bids {
			if bid.Qty > 0 {
				window.Prices = append(window.Prices, bid.Price)
			}
		}

		for _, ask := range record.Asks {
			if ask.Qty > 0 {
				window.Prices = append(window.Prices, ask.Price)
			}
		}
	}

	if window.Latest == nil || len(window.Prices) < 2 || len(window.Spreads) == 0 {
		return BookWindow{}, false
	}

	return window, true
}

/*
Scan visits each book update in the scoped symbol window.
*/
func (book *Book) Scan(symbol string, visit func(*BookRecord)) bool {
	memory, memoryOK := book.symbolMemory(symbol)

	if !memoryOK {
		return false
	}

	for _, record := range memory.eventsSnapshot() {
		current := record
		visit(&current)
	}

	return true
}

func (book *Book) Update(artifact *datura.Artifact) {
	if artifact == nil {
		return
	}

	datura.PayloadEach(artifact, func(index int, element ast.Node) bool {
		symbol, symbolOK := payloadString(element, "symbol")

		if !symbolOK || symbol == "" {
			return true
		}

		record := BookRecord{
			Symbol: symbol,
			Bids:   payloadBookLevels(element, "bids"),
			Asks:   payloadBookLevels(element, "asks"),
		}

		timestamp, timestampOK := payloadTime(element, "timestamp")

		if timestampOK {
			record.Timestamp = timestamp
		}

		if len(record.Bids) == 0 || len(record.Asks) == 0 {
			return true
		}

		memory := book.loadSymbolMemory(symbol)
		timestampNanos := uint64(record.Timestamp.UnixNano())

		if timestampNanos == 0 {
			timestampNanos = uint64(index + 1)
		}

		memory.appendEvent(record, timestampNanos, bookSensorySequence(record))

		if book.OnUpdate != nil {
			book.OnUpdate(&record)
		}

		return true
	})
}

func (book *Book) Read(buffer []byte) (int, error) {
	if book.Scope == "" {
		return 0, io.EOF
	}

	memory, memoryOK := book.symbolMemory(book.Scope)

	if !memoryOK {
		return 0, io.EOF
	}

	return memory.readNext(buffer, book.Scope, func(record BookRecord) ([]byte, error) {
		return marshalRecordJSON(record)
	})
}

func (book *Book) ResetReadHead() {
	book.symbols.Range(func(key, value any) bool {
		memory, memoryOK := value.(*symbolEpisodicMemory[BookRecord])

		if memoryOK {
			memory.resetReadHead()
		}

		return true
	})
}

func (book *Book) Close() error {
	book.cancel()

	return nil
}

func (book *Book) loadSymbolMemory(symbol string) *symbolEpisodicMemory[BookRecord] {
	value, _ := book.symbols.LoadOrStore(
		symbol,
		newSymbolEpisodicMemory[BookRecord]("book", "book", FeedRingCapacity()),
	)

	return value.(*symbolEpisodicMemory[BookRecord])
}

func (book *Book) symbolMemory(symbol string) (*symbolEpisodicMemory[BookRecord], bool) {
	value, ok := book.symbols.Load(symbol)

	if !ok {
		return nil, false
	}

	memory, memoryOK := value.(*symbolEpisodicMemory[BookRecord])

	return memory, memoryOK
}
