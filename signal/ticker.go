package signal

import (
	"context"
	"sync"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/structure"
	"github.com/theapemachine/symm/kraken/market"
)

/*
Ticker stores scoped ticker updates on a click clock.
OnUpdate runs after each accepted ticker update.
*/
type Ticker struct {
	ctx      context.Context
	cancel   context.CancelFunc
	Scope    string
	OnUpdate func(*market.TickerUpdate)
	symbols  *sync.Map
}

/*
NewTicker returns a ticker feed backed by a per-symbol click clock.
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
	value, ok := ticker.symbols.Load(symbol)

	if !ok {
		return TickerSnapshot{}
	}

	ring := value.(*structure.ClockRing[*market.TickerUpdate])

	var (
		first  *market.TickerUpdate
		latest *market.TickerUpdate
	)

	ring.Do(func(slot structure.ClockSlot[*market.TickerUpdate]) {
		update := slot.Payload

		if update == nil {
			return
		}

		if first == nil {
			first = update
		}

		latest = update
	})

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

func (ticker *Ticker) Update(update market.TickerUpdates) {
	for _, tickerUpdate := range update {
		if tickerUpdate == nil || tickerUpdate.Symbol == "" {
			continue
		}

		ring, _ := ticker.symbols.LoadOrStore(
			tickerUpdate.Symbol, structure.NewClockRing[*market.TickerUpdate](
				10, 100, 1000,
				datura.Acquire(
					"ticker", datura.Artifact_Type_json,
				).WithRole("ticker"),
			),
		)

		ring.(*structure.ClockRing[*market.TickerUpdate]).ObserveSecond(
			tickerUpdate.Timestamp, tickerUpdate,
		)

		if ticker.OnUpdate != nil {
			ticker.OnUpdate(tickerUpdate)
		}
	}
}

func (ticker *Ticker) Read(buffer []byte) (int, error) {
	var total int

	ticker.symbols.Range(func(key, value any) bool {
		ring := value.(*structure.ClockRing[*market.TickerUpdate])
		read, err := ring.Read(buffer)

		if err != nil {
			return false
		}

		total += read

		return true
	})

	return total, nil
}

func (ticker *Ticker) Close() error {
	ticker.cancel()

	return nil
}
