package correlation

import (
	"context"
	"math/bits"
	"math/rand"
	"sync"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/numeric"
	"github.com/theapemachine/symm/numeric/adaptive"
)

const (
	gridBars = 32 // length of each coin's movement fingerprint
	hashBits = 64 // fingerprint width: one uint64
	// correlationBatchInterval is the cross-section window: trades accumulate
	// per symbol and the SimHash herd pass runs once per interval, since the
	// correlation structure is meaningful across the cross-section, not per trade.
	correlationBatchInterval = 250 * time.Millisecond
)

/*
Signal measuring cross-asset "herd behavior" via the dominant eigenmode of the
return field — read cheaply as bit-agreement with the market's majority
fingerprint, so the whole universe is classified in one O(n) pass per tick with
no pairwise correlation matrix.

  - Fingerprint: each coin's recent movement is stamped into one 64-bit
    signature through a fixed set of random hyperplanes (SimHash).
  - Market mode: the bit-by-bit majority vote across all signatures is the
    dominant shared direction — "the market."
  - Correlation: a coin's bit-agreement with that majority (a single popcount)
    is how hard it is herding; anti-agreement is divergence.
  - Energy: each coin's exponentially-weighted return variance, normalised by a
    slow market-energy baseline, separates active regimes from quiet noise.

| Category         | Correlation | Variance | Market "Feel"   |
|:-----------------|:------------|:---------|:----------------|
| Systemic Herd    | High >0.85  | High     | Global Beta     |
| Decoupled Alpha  | Low         | High     | Unique Driver   |
| Stochastic Noise | Low         | Low      | Quiet           |
| Divergent Stress | Negative    | High     | Contrarian Move |
*/
type symbolState struct {
	prev   float64
	hist   [gridBars]float64
	cursor int
	energy *adaptive.EMA
	pipe   *numeric.Classed
}

type Signal struct {
	ctx          context.Context
	cancel       context.CancelFunc
	pool         *qpool.Q
	broadcasts   map[string]*qpool.BroadcastGroup
	subscribers  map[string]*qpool.Subscriber
	symbols      sync.Map
	planes       [hashBits][gridBars]float64 // random hyperplanes: the fingerprint stamp
	marketEnergy *adaptive.EMA               // slow activity baseline (the Noise gate)
	categories   map[string]perspectives.CategoryType
	floor        *adaptive.SNRField
}

func NewSignal(ctx context.Context, pool *qpool.Q) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		ctx:          ctx,
		cancel:       cancel,
		pool:         pool,
		broadcasts:   make(map[string]*qpool.BroadcastGroup),
		subscribers:  make(map[string]*qpool.Subscriber),
		marketEnergy: adaptive.NewEMA(0),
		categories: map[string]perspectives.CategoryType{
			"divergent_stress": perspectives.CategoryDivergentStress,
			"stochastic_noise": perspectives.CategoryStochasticNoise,
			"decoupled_alpha":  perspectives.CategoryDecoupledAlpha,
			"systemic_herd":    perspectives.CategorySystemicHerd,
		},
		floor: adaptive.NewSNRField(),
	}

	// One fixed random stamp, shared by every coin so fingerprints are comparable.
	rng := rand.New(rand.NewSource(1))

	for k := range signal.planes {
		for j := range signal.planes[k] {
			signal.planes[k][j] = float64(rng.Intn(2)*2 - 1) // -1 or +1
		}
	}

	signal.broadcasts["measurements"] = pool.CreateBroadcastGroup(
		"measurements", 10*time.Millisecond,
	)

	return signal
}

// newClassed is one coin's classification pipeline:
// fused = clamp(EMA( energy · (1 + 2·corr) / slow market energy )), then banded.
// The normalization by the slow market-energy baseline happens in the projection
// (baseline is guaranteed > 0 by the caller), so the chain is a single reshape
// into EMA then clamp — the market energy moves slowly enough that smoothing the
// ratio is equivalent to smoothing the numerator and dividing.
func newClassed() *numeric.Classed {
	return numeric.NewClassed(
		adaptive.NewClassifier(
			[]float64{-0.30, 0.40, 2.00}, // divergent | noise | alpha | herd
			[]float64{0, 1, 2, 3},
			[]string{"divergent_stress", "stochastic_noise", "decoupled_alpha", "systemic_herd"},
		),

		numeric.NewProject(func(_ float64, v []float64) []float64 {
			return []float64{v[1] * (1 + 2*v[0]) / v[2]} // (energy · (1 + 2·corr)) / market energy
		}),
		adaptive.NewEMA(0),
		adaptive.NewSigmaClamp(3, 8, 0.0625),
	)
}

// fingerprint stamps a coin's recent movement into one 64-bit signature.
func (signal *Signal) fingerprint(state *symbolState) uint64 {
	var sig uint64

	for k := range signal.planes {
		dot := 0.0

		for j := 0; j < gridBars; j++ {
			dot += signal.planes[k][j] * state.hist[j]
		}

		if dot >= 0 {
			sig |= 1 << uint(k)
		}
	}

	return sig
}

func (signal *Signal) Tick() error {
	trades := market.NewTradeSubscription(signal.ctx, config.System.Symbols...)
	batch := time.NewTicker(correlationBatchInterval)
	defer batch.Stop()

	latest := make(map[string]float64)

	for {
		select {
		case <-signal.ctx.Done():
			return signal.ctx.Err()
		case trade, ok := <-trades:
			if !ok {
				trades = nil
				continue
			}

			if trade != nil && trade.Price > 0 {
				latest[trade.Symbol] = trade.Price
			}
		case <-batch.C:
			if len(latest) == 0 {
				continue
			}

			signal.process(latest)
			latest = make(map[string]float64)
		}
	}
}

// live is one coin's per-batch fingerprint and the data needed to emit for it.
type live struct {
	symbol string
	price  float64
	state  *symbolState
	sig    uint64
}

// process runs one SimHash herd pass over the symbols that traded this window:
// stamp each coin's fingerprint, vote the market's dominant mode, then score each
// coin's agreement with it. O(symbols) per window, not per trade.
func (signal *Signal) process(latest map[string]float64) {
	active := make([]live, 0, len(latest))
	meanEnergy := 0.0

	for symbol, price := range latest {
		stored, _ := signal.symbols.LoadOrStore(symbol, &symbolState{
			energy: adaptive.NewEMA(0),
			pipe:   newClassed(),
		})
		state := stored.(*symbolState)

		if state.prev <= 0 {
			state.prev = price
			continue
		}

		ret := price/state.prev - 1
		state.prev = price
		state.hist[state.cursor] = ret
		state.cursor = (state.cursor + 1) % gridBars

		energy, _ := state.energy.Next(0, ret*ret)
		active = append(active, live{symbol: symbol, price: price, state: state, sig: signal.fingerprint(state)})
		meanEnergy += energy
	}

	if len(active) == 0 {
		return
	}

	meanEnergy /= float64(len(active))

	baseline, err := signal.marketEnergy.Next(0, meanEnergy)

	if err != nil || baseline <= 0 {
		return
	}

	signal.emitActive(active, signal.marketMode(active), baseline)
}

// marketMode is the bit-by-bit majority vote across fingerprints — the dominant
// shared direction of the cross-section.
func (signal *Signal) marketMode(active []live) uint64 {
	var ones [hashBits]int

	for _, coin := range active {
		for bit := 0; bit < hashBits; bit++ {
			ones[bit] += int(coin.sig >> uint(bit) & 1)
		}
	}

	var mode uint64

	for bit := 0; bit < hashBits; bit++ {
		if ones[bit]*2 > len(active) {
			mode |= 1 << uint(bit)
		}
	}

	return mode
}

// emitActive scores each coin's agreement with the market mode and publishes it.
func (signal *Signal) emitActive(active []live, mode uint64, baseline float64) {
	for _, coin := range active {
		agree := hashBits - bits.OnesCount64(coin.sig^mode)                     // 0..64
		corr := (float64(agree) - float64(hashBits)/2) / (float64(hashBits) / 2) // -1..1
		energy := coin.state.energy.Value()

		code, err := coin.state.pipe.Push(corr, energy, baseline)

		if err != nil {
			errnie.Error(err)
			continue
		}

		signal.broadcasts["measurements"].Send(&qpool.QValue[any]{
			Value: perspectives.Measurement{
				Symbol:   coin.symbol,
				Source:   perspectives.SourceCorrelation,
				Category: signal.categories[coin.state.pipe.Label(code)],
				SNR:      signal.floor.Score(coin.symbol, energy*(1+2*corr)/baseline),
				Last:     coin.price,
			},
		})
	}
}

func (signal *Signal) Close() error {
	signal.cancel()
	return nil
}
