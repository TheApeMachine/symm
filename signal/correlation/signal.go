package correlation

import (
	"context"
	"math/bits"
	"math/rand"
	"sync"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/numeric"
	"github.com/theapemachine/symm/numeric/adaptive"
)

const (
	// gridBars is how many recent bar returns feed each SimHash hyperplane dot product.
	gridBars = 32
	// hashBits is the fingerprint width: one uint64 signature per coin per batch.
	hashBits = 64
	// correlationBatchInterval is the cross-section window. Trades accumulate per
	// symbol and the herd pass runs once per interval — correlation is a property
	// of the cross-section, not of a single print.
	correlationBatchInterval = 250 * time.Millisecond
	// energyFloor excludes coins whose variance is below this fraction of the slow
	// market-energy baseline — dead-flat and illiquid coins cannot vote as herd.
	energyFloor = 0.0625
)

/*
symbolState holds one coin's rolling return ring, its slow energy estimate, and
the classification pipeline that maps (correlation, energy) into a perspective
category. filled counts ring writes toward gridBars — until the window is full,
the coin is excluded from the herd vote because an all-zero hist makes every
hyperplane dot tie at zero and fingerprint as all-ones (false perfect herd).
*/
type symbolState struct {
	prev   float64
	hist   [gridBars]float64
	cursor int
	filled int
	energy *adaptive.EMA
	pipe   *numeric.Classed
}

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
type Signal struct {
	ctx           context.Context
	cancel        context.CancelFunc
	pool          *qpool.Q
	broadcasts    map[string]*qpool.BroadcastGroup
	subscribers   map[string]*qpool.Subscriber
	symbols       sync.Map
	planes        [hashBits][gridBars]float64
	marketEnergy  *adaptive.EMA
	categories    map[string]perspectives.CategoryType
	activeScratch []live
	floor         *adaptive.SNRField
}

/*
NewSignal wires the measurements broadcast and installs one fixed random
hyperplane set. The seed is fixed so every coin is stamped with the same
projection — otherwise bit agreement would be meaningless.
*/
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

	// Fixed seed: one shared projection for the whole universe.
	rng := rand.New(rand.NewSource(1))

	for planeIndex := range signal.planes {
		for barIndex := range signal.planes[planeIndex] {
			signal.planes[planeIndex][barIndex] = float64(rng.Intn(2)*2 - 1)
		}
	}

	signal.broadcasts["measurements"] = pool.CreateBroadcastGroup(
		"measurements", 10*time.Millisecond,
	)

	return signal
}

func correlationFuse(_ float64, values []float64) float64 {
	return values[1] * (1 + 2*values[0]) / values[2]
}

func newSymbolState() *symbolState {
	return &symbolState{
		energy: adaptive.NewEMA(0),
		pipe: numeric.NewClassed(
			adaptive.NewClassifier(
				[]float64{-0.30, 0.40, 2.00},
				[]float64{0, 1, 2, 3},
				[]string{
					"divergent_stress",
					"stochastic_noise",
					"decoupled_alpha",
					"systemic_herd",
				},
			),
			numeric.NewProjectScalar(correlationFuse),
			adaptive.NewEMA(0),
			adaptive.NewSigmaClamp(3, 8, 0.0625),
		),
	}
}

func (signal *Signal) state(symbol string) *symbolState {
	if stored, ok := signal.symbols.Load(symbol); ok {
		return stored.(*symbolState)
	}

	created := newSymbolState()
	stored, loaded := signal.symbols.LoadOrStore(symbol, created)

	if loaded {
		return stored.(*symbolState)
	}

	return created
}

/*
fingerprint stamps one coin's recent return ring into a 64-bit SimHash: for each
hyperplane, dot(return_window, plane) >= 0 sets the corresponding bit. An
all-zero window ties every dot at zero and yields all-ones — callers must gate
on filled and energy before voting or scoring.
*/
func (signal *Signal) fingerprint(state *symbolState) uint64 {
	var sig uint64

	for planeIndex := range signal.planes {
		dot := 0.0

		for barIndex := range gridBars {
			dot += signal.planes[planeIndex][barIndex] * state.hist[barIndex]
		}

		if dot >= 0 {
			sig |= 1 << uint(planeIndex)
		}
	}

	return sig
}

/*
Tick accumulates the latest trade price per symbol and runs one herd pass per
batch tick. Per-trade processing would restamp fingerprints on every print;
correlationBatchInterval batches enough cross-section activity to be stable.
*/
func (signal *Signal) Tick() error {
	trades := market.NewTradeSubscription(signal.ctx, viper.GetViper().GetStringSlice("market.symbols")...)
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
			clear(latest)
		}
	}
}

/*
live is one coin's stamped fingerprint and the handles needed to emit for it
after the batch herd pass.
*/
type live struct {
	symbol string
	price  float64
	state  *symbolState
	sig    uint64
}

/*
process runs one SimHash herd pass for every symbol that traded this window:
update return rings, stamp fingerprints, vote market mode, emit per coin.
O(symbols) per batch, not per trade.
*/
func (signal *Signal) process(latest map[string]float64) {
	active := signal.activeScratch[:0]
	meanEnergy := 0.0

	// Slow market-energy from prior batches; 0 on cold start.
	base := signal.marketEnergy.Value()

	for symbol, price := range latest {
		state := signal.state(symbol)

		if state.prev <= 0 {
			state.prev = price
			continue
		}

		ret := price/state.prev - 1
		state.prev = price
		state.hist[state.cursor] = ret
		state.cursor = (state.cursor + 1) % gridBars

		if state.filled < gridBars {
			state.filled++
		}

		energy, err := state.energy.Next(0, ret*ret)

		if err != nil {
			errnie.Error(err)
			continue
		}

		if state.filled < gridBars || energy <= base*energyFloor {
			continue
		}

		active = append(active, live{
			symbol: symbol,
			price:  price,
			state:  state,
			sig:    signal.fingerprint(state),
		})

		meanEnergy += energy
	}

	signal.activeScratch = active

	if len(active) == 0 {
		return
	}

	meanEnergy /= float64(len(active))

	baseline, err := signal.marketEnergy.Next(0, meanEnergy)

	if err != nil {
		errnie.Error(err)
		return
	}

	if baseline <= 0 {
		return
	}

	signal.emitActive(active, signal.marketMode(active), baseline)
}

/*
marketMode is the cross-section consensus fingerprint: for each of the 64 bits,
count how many active coins have that bit set; if strictly more than half do,
the mode gets a 1 at that position. The result is the dominant shared direction
— "what the market looks like" this batch — against which each coin is scored.
*/
func (signal *Signal) marketMode(active []live) uint64 {
	var ones [hashBits]int

	for _, coin := range active {
		for bitIndex := range hashBits {
			ones[bitIndex] += int(coin.sig >> uint(bitIndex) & 1)
		}
	}

	var mode uint64

	for bitIndex := range hashBits {
		if ones[bitIndex]*2 > len(active) {
			mode |= 1 << uint(bitIndex)
		}
	}

	return mode
}

/*
emitActive scores each coin's agreement with market mode and publishes one
measurement. Agreement is Hamming similarity mapped to [-1, 1]; raw strength
is energy-weighted correlation normalised by the slow market-energy baseline.
Confidence is classification clarity — how clearly the fused score sits inside
its assigned category band; SNR is how surprising that clarity is versus the
coin's own recent baseline, not how large the strength is.
*/
func (signal *Signal) emitActive(active []live, mode uint64, baseline float64) {
	for _, coin := range active {
		agree := hashBits - bits.OnesCount64(coin.sig^mode)
		corr := (float64(agree) - float64(hashBits)/2) / (float64(hashBits) / 2)
		energy := coin.state.energy.Value()

		code, err := coin.state.pipe.Push(corr, energy, baseline)

		if err != nil {
			errnie.Error(err)
			continue
		}

		raw := energy * (1 + 2*corr) / baseline

		measurement := perspectives.Measurement{
			Symbol:     coin.symbol,
			Source:     perspectives.SourceCorrelation,
			Category:   signal.categories[coin.state.pipe.Label(code)],
			Last:       coin.price,
			Strength:   raw,
			Confidence: coin.state.pipe.Confidence(),
		}
		measurement.SNR = signal.floor.Score(measurement.Symbol, measurement.Confidence)
		signal.broadcasts["measurements"].Send(&qpool.QValue[any]{Value: measurement})
	}
}

func (signal *Signal) Close() error {
	signal.cancel()
	return nil
}
