package trader

import (
	"sync"
	"time"

	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken/market"
)

type restingMakerEntry struct {
	clOrdID     string
	symbol      string
	maker       broker.Maker
	tracker     *broker.MakerQueueTracker
	intent      orderIntent
	opportunity opportunity
	playbook    string
	spreadBPS   float64
	postedAt    time.Time
	waitTicks   int
	orderID     string
}

type makerDesk struct {
	mu        sync.Mutex
	bySymbol  map[string]*restingMakerEntry
	byClOrdID map[string]*restingMakerEntry
}

func newMakerDesk() *makerDesk {
	return &makerDesk{
		bySymbol:  make(map[string]*restingMakerEntry),
		byClOrdID: make(map[string]*restingMakerEntry),
	}
}

func (desk *makerDesk) track(entry *restingMakerEntry) {
	if desk == nil || entry == nil || entry.clOrdID == "" || entry.symbol == "" {
		return
	}

	desk.mu.Lock()
	defer desk.mu.Unlock()

	desk.bySymbol[entry.symbol] = entry
	desk.byClOrdID[entry.clOrdID] = entry
}

func (desk *makerDesk) drop(clOrdID, symbol string) {
	if desk == nil {
		return
	}

	desk.mu.Lock()
	defer desk.mu.Unlock()

	if clOrdID != "" {
		delete(desk.byClOrdID, clOrdID)
	}

	if symbol != "" {
		delete(desk.bySymbol, symbol)
	}
}

func (desk *makerDesk) HasPending(symbol string) bool {
	if desk == nil || symbol == "" {
		return false
	}

	desk.mu.Lock()
	defer desk.mu.Unlock()

	_, ok := desk.bySymbol[symbol]

	return ok
}

func (desk *makerDesk) bindOrderID(clOrdID, orderID string) {
	if desk == nil || clOrdID == "" || orderID == "" {
		return
	}

	desk.mu.Lock()
	defer desk.mu.Unlock()

	entry, ok := desk.byClOrdID[clOrdID]

	if !ok {
		return
	}

	entry.orderID = orderID
}

func (desk *makerDesk) entryFor(clOrdID string) (*restingMakerEntry, bool) {
	if desk == nil || clOrdID == "" {
		return nil, false
	}

	desk.mu.Lock()
	defer desk.mu.Unlock()

	entry, ok := desk.byClOrdID[clOrdID]

	return entry, ok
}

func (desk *makerDesk) observeTrade(trade market.TradeUpdate) {
	if desk == nil || trade.Symbol == "" {
		return
	}

	desk.mu.Lock()
	defer desk.mu.Unlock()

	entry, ok := desk.bySymbol[trade.Symbol]

	if !ok || entry == nil || entry.tracker == nil {
		return
	}

	entry.tracker.ObserveTrade(trade)
}

func (desk *makerDesk) pendingPaperEntries() []*restingMakerEntry {
	if desk == nil {
		return nil
	}

	desk.mu.Lock()
	defer desk.mu.Unlock()

	entries := make([]*restingMakerEntry, 0, len(desk.bySymbol))

	for _, entry := range desk.bySymbol {
		if entry != nil {
			entries = append(entries, entry)
		}
	}

	return entries
}

func (desk *makerDesk) advanceWaitTicks(liveEnabled bool) []*restingMakerEntry {
	if desk == nil {
		return nil
	}

	desk.mu.Lock()
	defer desk.mu.Unlock()

	fallbackTicks := configMakerFallbackTicks()
	ready := make([]*restingMakerEntry, 0, len(desk.bySymbol))

	for _, entry := range desk.bySymbol {
		if entry == nil {
			continue
		}

		entry.waitTicks++

		if entry.waitTicks < fallbackTicks {
			continue
		}

		if liveEnabled && entry.orderID == "" && entry.waitTicks < fallbackTicks*2 {
			continue
		}

		ready = append(ready, entry)
	}

	return ready
}

func configMakerFallbackTicks() int {
	ticks := config.System.ExecutionMakerFallbackTicks

	if ticks <= 0 {
		return 4
	}

	return ticks
}
