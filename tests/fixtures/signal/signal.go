package signal

import (
	"iter"
	"time"
)

/*
State names the deterministic market regimes understood by every fixture.
*/
type State uint8

const (
	Baseline State = iota
	FastPump
	SlowPump
	FastDump
	SlowDump
	VolumeAbsorption
	LowVolumeLift
	SpreadCompression
	ThinLiquidity
	LoadedLiquidity
	LiquidityRetreat
	SpoofLiquidity
	DepthThinning
	SlowCadenceLift
	SmallDisplacementLift
	SpreadControl
	LeaderFollower
	AdverseDivergence
	BullTrend
	BearTrend
	SidewaysChop
	VolatilitySpike
	SuddenReversal
	FlashCrash
)

const (
	idleObservations         = 16
	fastLegObservations      = 8
	slowLegObservations      = 16
	settleObservations       = 4
	initialPrice             = 100.0
	PriceIncrement           = 0.01
	QuantityIncrement        = 0.00000001
	idleAmplitudeFraction    = 0.0005
	eventMoveFraction        = 0.12
	smallMoveFraction        = eventMoveFraction / 4
	idleVolume               = 10.0
	idleVolumeWaveFraction   = 0.2
	bookLevels               = 2
	bestQuoteTicks           = 2
	initialOrderQuantity     = 10_000.0
	spreadCycleLength        = 4
	spreadWidenPhase         = 0
	spreadTightenPhase       = 2
	leaderLagObservations    = fastLegObservations / 2
	sustainedLegObservations = 24
)

var epoch = time.Date(2026, 7, 21, 9, 0, 0, 0, time.UTC)

/*
Order is one authoritative resting Level3 order from which the fixtures derive
both Kraken book feeds and the ticker touch.
*/
type Order struct {
	ID       string
	Side     string
	Price    float64
	Qty      float64
	Priority uint64
	At       time.Time
}

/*
Statistics contains the trade-derived cumulative values written into ticker
frames so no downstream fixture independently invents market history.
*/
type Statistics struct {
	Open   float64
	High   float64
	Low    float64
	Volume float64
	VWAP   float64
}

/*
Quote is the current executable touch used by the simulated paper venue.
*/
type Quote struct {
	Bid    float64
	BidQty float64
	Ask    float64
	AskQty float64
}

/*
Fill is one real match against an authoritative resting order.
*/
type Fill struct {
	Side    string
	Price   float64
	Qty     float64
	TradeID uint64
	At      time.Time
}

/*
Sample is one shared per-symbol market value written into every JSON template.
*/
type Sample struct {
	Symbol       string
	Side         string
	Price        float64
	TradePrice   float64
	Volume       float64
	TradeID      uint64
	At           time.Time
	Traded       bool
	Fills        []Fill
	BookChanged  bool
	TouchChanged bool
	Bids         []Order
	Asks         []Order
	Statistics   Statistics
}

/*
symbolState owns the authoritative book and cumulative trade state for a symbol.
*/
type symbolState struct {
	bids     []Order
	asks     []Order
	volume   float64
	notional float64
	open     float64
	high     float64
	low      float64
	lastSide string
	last     float64
	lastQty  float64
	tradeID  uint64
}

/*
Signal generates replayable per-symbol market tapes for the current transition.
*/
type Signal struct {
	symbols      []string
	markets      map[string]*symbolState
	at           time.Time
	phase        int
	state        State
	tape         [][]Sample
	nextID       uint64
	nextTrade    uint64
	bootstrapped bool
}

/*
New creates one deterministic signal shared by all fixtures in a market.
*/
func New(symbols []string) *Signal {
	return NewAt(symbols, epoch)
}

/*
NewAt creates a deterministic signal at an explicit event time so tests can
align a simulated clock without relying on wall time.
*/
func NewAt(symbols []string, at time.Time) *Signal {
	signal := &Signal{
		symbols: append([]string(nil), symbols...),
		markets: make(map[string]*symbolState, len(symbols)),
		at:      at,
	}

	for _, symbol := range symbols {
		signal.markets[symbol] = &symbolState{}
		signal.markets[symbol].seed(symbol, initialPrice, &signal.nextID, signal.at)
	}

	return signal
}

/*
Bootstrap idempotently seeds one historical print per symbol and exposes the
resulting current-state sample without advancing the scenario clock.
*/
func (signal *Signal) Bootstrap() {
	if signal.bootstrapped {
		return
	}

	samples := make([]Sample, len(signal.symbols))

	for index, symbol := range signal.symbols {
		side := "buy"

		if index%2 != 0 {
			side = "sell"
		}

		prints, err := signal.markets[symbol].execute(
			side, idleVolume, signal.at, &signal.nextTrade,
		)

		if err != nil {
			panic(err)
		}

		restingSide := "sell"

		if side == "sell" {
			restingSide = "buy"
		}

		if err := signal.markets[symbol].refill(
			restingSide,
			idleVolume,
			signal.at,
		); err != nil {
			panic(err)
		}

		samples[index] = signal.markets[symbol].sample(
			symbol, signal.at, prints, true, true,
		)
	}

	signal.tape = [][]Sample{samples}
	signal.bootstrapped = true
}

/*
Now returns the deterministic event time of the current simulated state.
*/
func (signal *Signal) Now() time.Time {
	return signal.at
}

/*
Quote returns the executable touch derived from the authoritative L3 state.
*/
func (signal *Signal) Quote(symbol string) (Quote, bool) {
	market, exists := signal.markets[symbol]

	if !exists {
		return Quote{}, false
	}

	return market.quote()
}

/*
Transition generates a finite event leg followed by idle samples at its result.
Optional symbols isolate the event while all other instruments remain idle.
*/
func (signal *Signal) Transition(state State, symbols ...string) error {
	draft := signal.clone()
	steps, err := draft.Scenario(state, symbols...)

	if err != nil {
		return err
	}

	tape := make([][]Sample, 0, len(steps))

	for _, step := range steps {
		if err := draft.Apply(step); err != nil {
			return err
		}

		tape = append(tape, draft.tape[0])
	}

	draft.tape = tape
	draft.state = state
	*signal = *draft
	return nil
}

/*
Complete records a semantic state only after every generated step has reached
its consumer successfully.
*/
func (signal *Signal) Complete(state State) {
	signal.state = state
}

/*
clone copies the small authoritative venue so a failed action cannot leak
clock, order, counter, or tape mutations into the next test step.
*/
func (signal *Signal) clone() *Signal {
	draft := *signal
	draft.symbols = append([]string(nil), signal.symbols...)
	draft.markets = make(map[string]*symbolState, len(signal.markets))

	for symbol, market := range signal.markets {
		copied := *market
		copied.bids = append([]Order(nil), market.bids...)
		copied.asks = append([]Order(nil), market.asks...)
		draft.markets[symbol] = &copied
	}

	return &draft
}

/*
Generate replays the current transition identically for every injected fixture.
*/
func (signal *Signal) Generate() iter.Seq[[]Sample] {
	return func(yield func([]Sample) bool) {
		for _, step := range signal.tape {
			if !yield(step) {
				return
			}
		}
	}
}

/*
State returns the semantic state represented by the current generated tape.
*/
func (signal *Signal) State() State {
	return signal.state
}

/*
observations returns the finite duration of the active event leg.
*/
func (state State) observations() int {
	switch state {
	case SlowPump, SlowDump:
		return slowLegObservations
	case BullTrend, BearTrend, SidewaysChop:
		return sustainedLegObservations
	case FastPump, FastDump, VolumeAbsorption, LowVolumeLift, SpreadCompression,
		SlowCadenceLift, SmallDisplacementLift, SpreadControl, LeaderFollower,
		AdverseDivergence, VolatilitySpike, SuddenReversal, FlashCrash:
		return fastLegObservations
	default:
		return idleObservations
	}
}

/*
valid rejects unknown regimes before they can emit a zero-direction event.
*/
func (state State) valid() bool {
	return state >= Baseline && state <= FlashCrash
}

/*
interval returns the sampling cadence of the active event leg.
*/
func (state State) interval() time.Duration {
	switch state {
	case FastPump, FastDump, VolumeAbsorption, LowVolumeLift,
		SmallDisplacementLift, LeaderFollower, AdverseDivergence,
		VolatilitySpike, SuddenReversal, FlashCrash:
		return 250 * time.Millisecond
	case SidewaysChop:
		return 500 * time.Millisecond
	default:
		return time.Second
	}
}

/*
direction returns the signed displacement of the selected event.
*/
func (state State) direction() float64 {
	switch state {
	case FastPump, SlowPump, LowVolumeLift, SlowCadenceLift,
		SmallDisplacementLift, LeaderFollower, AdverseDivergence, BullTrend:
		return 1
	case FastDump, SlowDump, BearTrend, FlashCrash:
		return -1
	default:
		return 0
	}
}

/*
volume returns the executed quantity generated during the active event leg.
*/
func (state State) volume() float64 {
	switch state {
	case FastPump, FastDump, VolumeAbsorption, SlowCadenceLift,
		SmallDisplacementLift, LeaderFollower, AdverseDivergence,
		VolatilitySpike, SuddenReversal, FlashCrash:
		return 100
	case BullTrend, BearTrend:
		return 60
	case SidewaysChop:
		return 40
	case SlowPump, SlowDump:
		return 30
	default:
		return idleVolume
	}
}
