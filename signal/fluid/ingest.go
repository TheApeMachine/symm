package fluid

import (
	"fmt"
	"runtime"
	"sync/atomic"
	"time"

	disruptor "github.com/smarty/go-disruptor"
	"github.com/theapemachine/errnie"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
)

type ingestKind uint8

const (
	ingestNone ingestKind = iota
	ingestTicker
	ingestBook
	ingestTrade
)

const ingestBufferSize = 1024 * 64

type ingestEvent struct {
	symbol    string
	kind      ingestKind
	ticker    krakenmarket.TickerUpdate
	book      krakenmarket.Book
	tickerAt  time.Time
	bookAt    time.Time
	tradeAt   time.Time
	tradeQty  float64
	tradeSide string
}

type ingestHandler struct {
	system    *System
	processed atomic.Int64
}

func (handler *ingestHandler) Handle(lowerSequence, upperSequence int64) {
	for sequence := lowerSequence; sequence <= upperSequence; sequence++ {
		event := handler.system.ingestRing[sequence&(ingestBufferSize-1)]

		if applyErr := handler.system.applyIngest(event); applyErr != nil {
			handler.system.ingestErr.Store(applyErr)
		}

		handler.processed.Store(sequence)
	}
}

func (system *System) startIngest() error {
	handler := &ingestHandler{system: system}

	instance, err := disruptor.New(
		disruptor.Options.BufferCapacity(ingestBufferSize),
		disruptor.Options.WriterCount(1),
		disruptor.Options.NewHandlerGroup(handler),
	)

	if err != nil {
		return fmt.Errorf("fluid: initialize ingest disruptor: %w", err)
	}

	system.ingest = instance
	system.ingestHandler = handler
	system.ingestRing = make([]ingestEvent, ingestBufferSize)
	system.ingestWaitGroup.Add(1)

	go func() {
		defer system.ingestWaitGroup.Done()
		instance.Listen()
	}()

	return nil
}

func (system *System) closeIngest() {
	if system.ingest == nil {
		return
	}

	_ = system.ingest.Close()
	system.ingestWaitGroup.Wait()
}

func (system *System) publishIngest(event ingestEvent) error {
	if system.ingest == nil {
		return system.applyIngest(event)
	}

	for spin := 0; ; spin++ {
		upperSequence := system.ingest.TryReserve(1)

		switch upperSequence {
		case disruptor.ErrCapacityUnavailable:
			system.backoffIngestReservation(spin)
			continue
		case disruptor.ErrReservationSize:
			return errnie.Error(fmt.Errorf("fluid: invalid ingest reservation"))
		default:
			system.ingestRing[upperSequence&(ingestBufferSize-1)] = event
			system.ingest.Commit(upperSequence, upperSequence)
			system.awaitIngest(upperSequence)

			if stored := system.ingestErr.Load(); stored != nil {
				if applyErr, ok := stored.(error); ok {
					return applyErr
				}
			}

			return nil
		}
	}
}

func (system *System) backoffIngestReservation(spin int) {
	if spin < 64 {
		runtime.Gosched()
		return
	}

	time.Sleep(time.Duration(50+spin%100) * time.Microsecond)
}

func (system *System) awaitIngest(sequence int64) {
	handler := system.ingestHandler

	if handler == nil {
		return
	}

	for handler.processed.Load() < sequence {
		runtime.Gosched()
	}
}

func (system *System) applyIngest(event ingestEvent) error {
	state := system.loadSymbol(event.symbol)

	if state == nil {
		return errnie.Error(fmt.Errorf("fluid: symbol %q not found", event.symbol))
	}

	switch event.kind {
	case ingestTicker:
		return state.FeedTicker(event.ticker, event.tickerAt)
	case ingestBook:
		return state.FeedBook(event.book, event.bookAt)
	case ingestTrade:
		return state.FeedTradeSide(event.tradeAt, event.tradeQty, event.tradeSide)
	default:
		return errnie.Error(fmt.Errorf("fluid: unknown ingest kind %d", event.kind))
	}
}
