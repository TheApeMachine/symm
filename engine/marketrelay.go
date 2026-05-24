package engine

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/market"
)

const tradeEventCap = 512

/*
MarketReader serves symbol snapshots from broadcast-backed cache.
*/
type MarketReader interface {
	Read(symbol string) Snapshot
	ReadFresh(symbol string, now time.Time, ttl time.Duration) Snapshot
}

/*
MarketRelay drains tick, trade, and book broadcast groups into per-symbol cache.
*/
type MarketRelay struct {
	ctx           context.Context
	cancel        context.CancelFunc
	subscriptions map[string]*qpool.Subscriber
	mu            sync.RWMutex
	bySymbol      map[string]*symbolMarketState
}

type symbolMarketState struct {
	last         float64
	lastAt       time.Time
	lastOK       bool
	volumeBase   float64
	volumeOK     bool
	changePct    float64
	changeOK     bool
	batchVolume  float64
	tradesAt     time.Time
	batchOK      bool
	buyPressure  float64
	pressureOK   bool
	spreadBPS    float64
	bookAt       time.Time
	spreadOK     bool
	imbalance    float64
	imbalanceOK  bool
	density      float64
	densityOK    bool
	depthSlope   float64
	depthSlopeOK bool
	bidLevels    []market.BookLevel
	askLevels    []market.BookLevel
	depthOK      bool
	ticks        []market.TradeTick
	ticksReady   bool
}

var _ MarketReader = (*MarketRelay)(nil)

var _ Ticker = (*MarketRelay)(nil)

/*
NewMarketRelay subscribes to tick, trade, and book on the shared broadcast groups.
*/
func NewMarketRelay(
	ctx context.Context,
	tick, trade, book *qpool.BroadcastGroup,
) (*MarketRelay, error) {
	if tick == nil || trade == nil || book == nil {
		return nil, fmt.Errorf("market relay requires tick, trade, and book groups")
	}

	ctx, cancel := context.WithCancel(ctx)

	relay := &MarketRelay{
		ctx:           ctx,
		cancel:        cancel,
		subscriptions: make(map[string]*qpool.Subscriber),
		bySymbol:      make(map[string]*symbolMarketState),
	}

	relay.subscriptions["tick"] = tick.Subscribe("engine:market", 65536)
	relay.subscriptions["trade"] = trade.Subscribe("engine:market", 65536)
	relay.subscriptions["book"] = book.Subscribe("engine:market", 65536)

	return relay, errnie.Require(map[string]any{
		"ctx":           ctx,
		"cancel":        cancel,
		"subscriptions": relay.subscriptions,
	})
}

/*
Tick drains one market broadcast message into relay cache.
*/
func (relay *MarketRelay) Tick() bool {
	select {
	case <-relay.ctx.Done():
		return false
	case value := <-relay.subscriptions["tick"].Incoming:
		if value == nil {
			return false
		}

		relay.applyTick(value)

		return true
	case value := <-relay.subscriptions["trade"].Incoming:
		if value == nil {
			return false
		}

		relay.applyTrade(value)

		return true
	case value := <-relay.subscriptions["book"].Incoming:
		if value == nil {
			return false
		}

		relay.applyBook(value)

		return true
	default:
		return false
	}
}

/*
Drain ingests up to limit pending market broadcast messages.
*/
func (relay *MarketRelay) Drain(limit int) int {
	if limit <= 0 {
		return 0
	}

	drained := 0

	for drained < limit {
		if !relay.Tick() {
			return drained
		}

		drained++
	}

	return drained
}

/*
Read returns the latest cached market values for one symbol.
*/
func (relay *MarketRelay) Read(symbol string) Snapshot {
	relay.mu.RLock()
	defer relay.mu.RUnlock()

	state, ok := relay.bySymbol[symbol]

	if !ok {
		return Snapshot{}
	}

	return snapshotFromState(state)
}

/*
ReadFresh returns a snapshot with stale fields cleared relative to now and ttl.
*/
func (relay *MarketRelay) ReadFresh(
	symbol string,
	now time.Time,
	ttl time.Duration,
) Snapshot {
	snapshot := relay.Read(symbol)

	if ttl <= 0 {
		return snapshot
	}

	if snapshot.LastOK && snapshotStale(snapshot.LastAt, now, ttl) {
		snapshot.LastOK = false
	}

	if snapshot.BatchOK && snapshotStale(snapshot.TradesAt, now, ttl) {
		snapshot.BatchOK = false
		snapshot.PressureOK = false
	}

	if snapshot.SpreadOK && snapshotStale(snapshot.BookAt, now, ttl) {
		snapshot.SpreadOK = false
		snapshot.ImbalanceOK = false
		snapshot.DensityOK = false
		snapshot.DepthSlopeOK = false
		snapshot.DepthOK = false
		snapshot.BidLevels = nil
		snapshot.AskLevels = nil
	}

	return snapshot
}

func snapshotStale(at, now time.Time, ttl time.Duration) bool {
	if at.IsZero() {
		return true
	}

	return now.Sub(at) > ttl
}

/*
RecentTicks returns stored trade events for one symbol, optionally filtered by time.
*/
func (relay *MarketRelay) RecentTicks(symbol string, since time.Time) ([]market.TradeTick, bool) {
	relay.mu.RLock()
	defer relay.mu.RUnlock()

	state, ok := relay.bySymbol[symbol]

	if !ok || !state.ticksReady {
		return nil, false
	}

	if since.IsZero() {
		cp := append([]market.TradeTick(nil), state.ticks...)

		return cp, len(cp) > 0
	}

	filtered := make([]market.TradeTick, 0, len(state.ticks))

	for _, tick := range state.ticks {
		if !tick.Timestamp.Before(since) {
			filtered = append(filtered, tick)
		}
	}

	return filtered, len(filtered) > 0
}

func (relay *MarketRelay) applyTick(value *qpool.QValue[any]) {
	update, ok := value.Value.(TickUpdate)

	if !ok || update.Symbol == "" || update.Last <= 0 {
		return
	}

	relay.mu.Lock()
	defer relay.mu.Unlock()

	state := relay.ensure(update.Symbol)
	state.last = update.Last
	state.lastOK = true
	state.volumeBase = update.VolumeBase
	state.volumeOK = update.VolumeBase > 0
	state.changePct = update.ChangePct
	state.changeOK = true

	if parsed := parseExchangeTime(update.Timestamp); !parsed.IsZero() {
		state.lastAt = parsed
	}
}

func (relay *MarketRelay) applyTrade(value *qpool.QValue[any]) {
	update, ok := value.Value.(TradeUpdate)

	if !ok || update.Symbol == "" || update.BatchVolume <= 0 {
		return
	}

	relay.mu.Lock()
	defer relay.mu.Unlock()

	state := relay.ensure(update.Symbol)
	state.batchVolume = update.BatchVolume
	state.batchOK = true
	state.buyPressure = update.BuyPressure
	state.pressureOK = true
	state.tradesAt = update.UpdatedAt
	state.ticksReady = true

	for _, tick := range update.Ticks {
		if tick.Symbol != update.Symbol {
			continue
		}

		state.ticks = append(state.ticks, tick)
	}

	if len(state.ticks) > tradeEventCap {
		state.ticks = state.ticks[len(state.ticks)-tradeEventCap:]
	}
}

func (relay *MarketRelay) applyBook(value *qpool.QValue[any]) {
	update, ok := value.Value.(BookUpdate)

	if !ok || update.Symbol == "" || update.SpreadBPS <= 0 {
		return
	}

	relay.mu.Lock()
	defer relay.mu.Unlock()

	state := relay.ensure(update.Symbol)
	state.spreadBPS = update.SpreadBPS
	state.spreadOK = true
	state.imbalance = update.Imbalance
	state.imbalanceOK = true
	state.density = update.Density
	state.densityOK = true
	state.depthSlope = update.DepthSlope
	state.depthSlopeOK = true
	state.bidLevels = append([]market.BookLevel(nil), update.BidLevels...)
	state.askLevels = append([]market.BookLevel(nil), update.AskLevels...)
	state.depthOK = len(update.BidLevels) > 0 && len(update.AskLevels) > 0
	state.bookAt = update.UpdatedAt
}

func (relay *MarketRelay) ensure(symbol string) *symbolMarketState {
	state, ok := relay.bySymbol[symbol]

	if ok {
		return state
	}

	state = &symbolMarketState{}
	relay.bySymbol[symbol] = state

	return state
}

func snapshotFromState(state *symbolMarketState) Snapshot {
	return Snapshot{
		Last:         state.last,
		LastAt:       state.lastAt,
		LastOK:       state.lastOK,
		VolumeBase:   state.volumeBase,
		VolumeOK:     state.volumeOK,
		BatchVolume:  state.batchVolume,
		TradesAt:     state.tradesAt,
		BatchOK:      state.batchOK,
		BuyPressure:  state.buyPressure,
		PressureOK:   state.pressureOK,
		SpreadBPS:    state.spreadBPS,
		BookAt:       state.bookAt,
		SpreadOK:     state.spreadOK,
		Imbalance:    state.imbalance,
		ImbalanceOK:  state.imbalanceOK,
		Density:      state.density,
		DensityOK:    state.densityOK,
		ChangePct:    state.changePct,
		ChangeOK:     state.changeOK,
		DepthSlope:   state.depthSlope,
		DepthSlopeOK: state.depthSlopeOK,
		BidLevels:    append([]market.BookLevel(nil), state.bidLevels...),
		AskLevels:    append([]market.BookLevel(nil), state.askLevels...),
		DepthOK:      state.depthOK,
	}
}

func parseExchangeTime(raw string) time.Time {
	if raw == "" {
		return time.Time{}
	}

	parsed, err := time.Parse(time.RFC3339Nano, raw)

	if err != nil {
		return time.Time{}
	}

	return parsed
}
