package correlation

import (
	"context"
	"errors"
	"fmt"
	"math/bits"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/numeric"
	"github.com/theapemachine/symm/numeric/adaptive"
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
live is one coin's stamped fingerprint and the handles needed to emit for it
after the batch herd pass.
*/
type live struct {
	symbol string
	price  float64
	state  *symbolState
	sig    uint64
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
process runs one SimHash herd pass for every symbol that traded this window:
update return rings, stamp fingerprints, vote market mode, emit per coin.
O(symbols) per batch, not per trade.
*/
func (signal *Signal) process(latest map[string]float64) error {
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
			return fmt.Errorf("correlation: energy %s: %w", symbol, err)
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
		return nil
	}

	meanEnergy /= float64(len(active))

	baseline, err := signal.marketEnergy.Next(0, meanEnergy)

	if err != nil {
		return fmt.Errorf("correlation: market energy: %w", err)
	}

	if baseline <= 0 {
		return fmt.Errorf("correlation: non-positive market energy baseline")
	}

	return signal.emitActive(active, signal.marketMode(active), baseline)
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
func (signal *Signal) emitActive(active []live, mode uint64, baseline float64) error {
	tasks := make([]chan *qpool.QValue[any], 0, len(active))

	for _, coin := range active {
		tasks = append(tasks, signal.pool.ScheduleFast(signal.ctx, func(context.Context) (any, error) {
			agree := hashBits - bits.OnesCount64(coin.sig^mode)
			corr := (float64(agree) - float64(hashBits)/2) / (float64(hashBits) / 2)
			energy := coin.state.energy.Value()

			code, err := coin.state.pipe.Push(corr, energy, baseline)

			if err != nil {
				return nil, fmt.Errorf("correlation: pipe %s: %w", coin.symbol, err)
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

			if err := perspectives.AssignCategorySNR(
				&measurement, signal.floor, coin.state.pipe.Standout(),
			); err != nil {
				return nil, fmt.Errorf("correlation: snr %s: %w", coin.symbol, err)
			}

			errnie.Info("signal/correlation:measurement")
			signal.broadcasts["measurements"].Send(&qpool.QValue[any]{Value: measurement})
			if ui := signal.broadcasts["ui"]; ui != nil {
				ui.Send(&qpool.QValue[any]{Value: map[string]any{"chart": "gauge", "source": measurement.Source.String(), "confidence": measurement.Confidence, "snr": measurement.SNR}})
			}

			return nil, nil
		}))
	}

	var err error

	for _, task := range tasks {
		value := <-task
		err = errors.Join(err, value.Error)
	}

	return err
}
