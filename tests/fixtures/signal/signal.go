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
)

const (
	idleObservations       = 16
	fastLegObservations    = 8
	slowLegObservations    = 16
	settleObservations     = 4
	initialPrice           = 100.0
	PriceIncrement         = 0.01
	idleAmplitudeFraction  = 0.0005
	eventMoveFraction      = 0.12
	idleVolume             = 10.0
	idleVolumeWaveFraction = 0.2
)

var epoch = time.Date(2026, 7, 21, 9, 0, 0, 0, time.UTC)

/*
Order is one authoritative resting Level3 order from which the fixtures derive
both Kraken book feeds and the ticker touch.
*/
type Order struct {
	ID    string
	Side  string
	Price float64
	Qty   float64
	At    time.Time
}

/*
Statistics contains the trade-derived session values written into ticker
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
	Bid float64
	Ask float64
}

/*
Sample is one shared per-symbol market value written into every JSON template.
*/
type Sample struct {
	Symbol     string
	Side       string
	Price      float64
	TradePrice float64
	Volume     float64
	TradeID    uint64
	BookVolume float64
	At         time.Time
	Traded     bool
	Bids       []Order
	Asks       []Order
	Statistics Statistics
}

type symbolState struct {
	mid      float64
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
	symbols   []string
	markets   map[string]*symbolState
	at        time.Time
	phase     int
	state     State
	tape      [][]Sample
	nextID    uint64
	nextTrade uint64
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
		signal.markets[symbol] = &symbolState{mid: initialPrice}
		signal.seed(symbol)
	}

	return signal
}

/*
Bootstrap exposes one current-state sample without advancing the scenario clock
or warming any signal history.
*/
func (signal *Signal) Bootstrap() {
	samples := make([]Sample, len(signal.symbols))

	for index, symbol := range signal.symbols {
		side := "buy"

		if index%2 != 0 {
			side = "sell"
		}

		_, err := signal.execute(symbol, side, idleVolume)

		if err != nil {
			panic(err)
		}

		samples[index] = signal.sample(symbol, true)
	}

	signal.tape = [][]Sample{samples}
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

	if !exists || len(market.bids) == 0 || len(market.asks) == 0 {
		return Quote{}, false
	}

	return Quote{
		Bid: market.bids[touchIndex(market.bids, "buy")].Price,
		Ask: market.asks[touchIndex(market.asks, "sell")].Price,
	}, true
}

/*
Transition generates a finite event leg followed by idle samples at its result.
*/
func (signal *Signal) Transition(state State) error {
	steps, err := signal.Scenario(state)

	if err != nil {
		return err
	}

	tape := make([][]Sample, 0, len(steps))

	for _, step := range steps {
		if err := signal.Apply(step); err != nil {
			return err
		}

		tape = append(tape, signal.tape[0])
	}

	signal.tape = tape
	return nil
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
	case FastPump, FastDump:
		return fastLegObservations
	default:
		return idleObservations
	}
}

/*
valid rejects unknown regimes before they can emit a zero-direction event.
*/
func (state State) valid() bool {
	return state >= Baseline && state <= SlowDump
}

/*
interval returns the sampling cadence of the active event leg.
*/
func (state State) interval() time.Duration {
	switch state {
	case FastPump, FastDump:
		return 250 * time.Millisecond
	default:
		return time.Second
	}
}

/*
direction returns the signed displacement of the selected event.
*/
func (state State) direction() float64 {
	switch state {
	case FastPump, SlowPump:
		return 1
	case FastDump, SlowDump:
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
	case FastPump, FastDump:
		return 100
	case SlowPump, SlowDump:
		return 30
	default:
		return idleVolume
	}
}
