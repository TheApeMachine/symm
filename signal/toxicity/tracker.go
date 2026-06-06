package toxicity

import (
	"sync"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/market/perspectives/types"
	"github.com/theapemachine/symm/numeric/adaptive"
)

const (
	tradeMatchWindow         = 2 * time.Second
	priceMatchTol            = 0.0002 // tight: L3 gives exact prices
	fillCoverage             = 0.5    // matched trade vol >= this x removed qty => fill
	toxicMaxAge              = 10 * time.Second
	toxicProximityPct        = 0.005 // 0.5 % of mid
	largeBlockFrac           = 0.10  // >= 10 % of that side's visible depth = "large"
	toxicCooldown            = 30 * time.Second
	flowAlpha                = 0.05
	tradeRingCap             = 512
	flashChurnWindow         = 50 * time.Millisecond
	flashChurnRatioThreshold = 0.85
)

// SideBid and SideAsk are the byte side codes the tracker keys book levels by.
const (
	SideBid byte = 'b'
	SideAsk byte = 'a'
)

type orderState struct {
	side  byte // 'b' or 'a'
	price float64
	qty   float64
	addTs time.Time
}

type tradePrint struct {
	at     time.Time
	price  float64
	volume float64
}

// l2Level is the per-price aggregate the L2 fallback maintains in place of the
// per-order book L3 provides (§16.4.4). firstSeen is the age proxy.
type l2Level struct {
	qty       float64
	firstSeen time.Time
}

type l2Key struct {
	side  byte
	price float64
}

type levelChurnWindow struct {
	addVol    float64
	deleteVol float64
	started   time.Time
}

type symbolState struct {
	pair       market.Pair
	orders     map[string]*orderState // order_id -> resting order (L3)
	levels     map[l2Key]*l2Level     // (side, price) -> aggregate (L2 fallback)
	churn      map[l2Key]*levelChurnWindow
	bidTotal   float64 // summed visible bid qty
	askTotal   float64
	toxic      map[float64]time.Time // price -> expiry
	toxicChurn map[float64]float64   // price -> cancel/add ratio at flag time
	trades     []tradePrint
	mid        float64
	lastPrice  float64
	tracked    *types.Category
	cancelBid  float64
	fillBid    float64
	cancelAsk  float64
	fillAsk    float64
}

// Tracker classifies book-liquidity removals into fill vs cancel by joining the
// public trade tape, flags large young near-touch cancels as toxic, and reads a
// directional bias from the cancel-to-fill asymmetry. It is fed per-order by
// the authenticated L3 client (ApplyOrder) or per-level by the public L2 book
// fallback (ApplyBookLevel); both share the same classification core.
type Tracker struct {
	mu                   sync.Mutex
	symbols              map[string]*symbolState
	floor                *adaptive.SNRField
	minFillToCancelRatio float64
}

func NewTracker() *Tracker {
	return &Tracker{
		symbols: make(map[string]*symbolState),
		floor:   adaptive.NewSNRField(),
	}
}

/*
fillToCancelThreshold returns the configured cancel/fill asymmetry gate. The value
is read lazily because the process-wide defaultTracker is constructed at package
init, before viper loads cmd/cfg/config.yml.
*/
func (tracker *Tracker) fillToCancelThreshold() float64 {
	if tracker.minFillToCancelRatio > 0 {
		return tracker.minFillToCancelRatio
	}

	ratio := viper.GetFloat64("signals.min_fill_to_cancel_ratio")

	if ratio <= 0 {
		return 0
	}

	tracker.minFillToCancelRatio = ratio

	return ratio
}

func (tracker *Tracker) stateLocked(symbol string, pair market.Pair) *symbolState {
	state := tracker.symbols[symbol]

	if state == nil {
		state = &symbolState{
			pair:       pair,
			orders:     make(map[string]*orderState),
			levels:     make(map[l2Key]*l2Level),
			churn:      make(map[l2Key]*levelChurnWindow),
			toxic:      make(map[float64]time.Time),
			toxicChurn: make(map[float64]float64),
			tracked:    types.NewCategory(types.CategoryTypeNone),
		}
		tracker.symbols[symbol] = state
	}

	return state
}

func (tracker *Tracker) ObserveTrade(symbol string, pair market.Pair, price, volume float64, at time.Time) {
	if price <= 0 || volume <= 0 {
		return
	}

	tracker.mu.Lock()
	defer tracker.mu.Unlock()

	state := tracker.stateLocked(symbol, pair)
	state.lastPrice = price
	state.trades = append(state.trades, tradePrint{at: at, price: price, volume: volume})

	if len(state.trades) > tradeRingCap {
		state.trades = state.trades[len(state.trades)-tradeRingCap:]
	}
}

func (tracker *Tracker) ObserveMid(symbol string, pair market.Pair, mid float64) {
	if mid <= 0 {
		return
	}

	tracker.mu.Lock()
	defer tracker.mu.Unlock()

	tracker.stateLocked(symbol, pair).mid = mid
}

func (tracker *Tracker) ObserveLast(symbol string, pair market.Pair, last float64) {
	if last <= 0 {
		return
	}

	tracker.mu.Lock()
	defer tracker.mu.Unlock()

	tracker.stateLocked(symbol, pair).lastPrice = last
}
