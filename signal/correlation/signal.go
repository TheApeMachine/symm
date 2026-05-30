package correlation

import (
	"context"
	"math/bits"
	"math/rand"
	"sync"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/numeric"
	"github.com/theapemachine/symm/numeric/adaptive"
)

const (
	gridBars = 32 // length of each coin's movement fingerprint
	hashBits = 64 // fingerprint width: one uint64
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
	}

	// One fixed random stamp, shared by every coin so fingerprints are comparable.
	rng := rand.New(rand.NewSource(1))

	for k := range signal.planes {
		for j := range signal.planes[k] {
			signal.planes[k][j] = float64(rng.Intn(2)*2 - 1) // -1 or +1
		}
	}

	tradeGroup := pool.CreateBroadcastGroup("trades", 10*time.Millisecond)
	signal.subscribers["trades"] = tradeGroup.Subscribe("trades", 128)

	signal.broadcasts["measurements"] = pool.CreateBroadcastGroup(
		"measurements", 10*time.Millisecond,
	)

	return signal
}

// newClassed is one coin's classification pipeline:
// fused = EMA(energy · (1 + 2·corr)) / slow market energy, clamped, then banded.
func newClassed() *numeric.Classed {
	return numeric.NewClassed(
		adaptive.NewClassifier(
			[]float64{-0.30, 0.40, 2.00}, // divergent | noise | alpha | herd
			[]float64{0, 1, 2, 3},
			[]string{"divergent_stress", "stochastic_noise", "decoupled_alpha", "systemic_herd"},
		),

		numeric.NewProject(func(_ float64, v []float64) []float64 {
			return []float64{v[1] * (1 + 2*v[0])} // energy · (1 + 2·correlation)
		}),
		adaptive.NewEMA(0),
		numeric.NewProject(func(out float64, v []float64) []float64 {
			return []float64{out, v[2]} // numerator, slow market-energy baseline
		}),
		adaptive.NewRatio(0),
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
	var wg sync.WaitGroup

	wg.Go(func() {
		for {
			select {
			case <-signal.ctx.Done():
				return
			case value, ok := <-signal.subscribers["trades"].Incoming:
				if !ok || value.Value == nil {
					continue
				}

				trades, ok := value.Value.([]market.TradeUpdate)

				if !ok {
					continue
				}

				latest := make(map[string]float64, len(trades))

				for _, trade := range trades {
					if trade.Price > 0 {
						latest[trade.Symbol] = trade.Price
					}
				}

				// Pass 1 — update each coin's fingerprint + energy. O(n).
				type live struct {
					state *symbolState
					sig   uint64
				}

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
					active = append(active, live{state: state, sig: signal.fingerprint(state)})
					meanEnergy += energy
				}

				if len(active) == 0 {
					continue
				}

				meanEnergy /= float64(len(active))

				baseline, err := signal.marketEnergy.Next(0, meanEnergy)

				if err != nil || baseline <= 0 {
					continue
				}

				// The market's fingerprint = bit-by-bit majority vote (the dominant mode). O(n).
				var ones [hashBits]int

				for _, coin := range active {
					for k := 0; k < hashBits; k++ {
						ones[k] += int(coin.sig >> uint(k) & 1)
					}
				}

				var market uint64

				for k := 0; k < hashBits; k++ {
					if ones[k]*2 > len(active) {
						market |= 1 << uint(k)
					}
				}

				// Pass 2 — each coin's bit-agreement with the market is its correlation. O(n).
				for _, coin := range active {
					agree := hashBits - bits.OnesCount64(coin.sig^market)                    // 0..64
					corr := (float64(agree) - float64(hashBits)/2) / (float64(hashBits) / 2) // -1..1

					code, err := coin.state.pipe.Push(corr, coin.state.energy.Value(), baseline)

					if err != nil {
						errnie.Error(err)
						continue
					}

					signal.broadcasts["measurements"].Send(&qpool.QValue[any]{
						Value: perspectives.Measurement{
							Source:   perspectives.SourceCorrelation,
							Category: signal.categories[coin.state.pipe.Label(code)],
							SNR:      1,
						},
					})
				}
			}
		}
	})

	wg.Wait()
	return signal.ctx.Err()
}

func (signal *Signal) Close() error {
	signal.cancel()
	return nil
}
