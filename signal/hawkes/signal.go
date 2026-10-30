package hawkes

import (
	"context"
	"math"
	"sync"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/algorithm/excitation"
	"github.com/theapemachine/nomagique/data"
	"github.com/theapemachine/nomagique/hawkes"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/kraken"
	parent "github.com/theapemachine/symm/signal"
)

/*
symbolModel owns the per-symbol arrival sampler and the asynchronous Hawkes
excitation process. RunTrades may invoke several prints for the same pair in
one tick; mu serializes sample, process, and gate history for that pair.
*/
type symbolModel struct {
	mu sync.Mutex

	sample  *excitation.Sample
	process *excitation.Process

	gates *gates
}

// gates is a short retained history of fit statistics from which the regime
// gates are derived. It bounds memory without sharing a fixed event window
// across symbols of different activity rates.
type gates struct {
	radii       []float64
	asymmetries []float64
	limit       int
}

const gatesLimit = 256

func newGates() *gates {
	return &gates{limit: gatesLimit}
}

func (g *gates) observe(radius, asymmetry float64) {
	if math.IsNaN(radius) || math.IsInf(radius, 0) {
		return
	}

	g.radii = append(g.radii, radius)
	g.asymmetries = append(g.asymmetries, asymmetry)

	if len(g.radii) > g.limit {
		g.radii = g.radii[len(g.radii)-g.limit:]
		g.asymmetries = g.asymmetries[len(g.asymmetries)-g.limit:]
	}
}

// Gates returns the fit-history-derived regime gates, ready once enough fits
// have been observed.
func (g *gates) Gates() (hawkes.FitGates, bool) {
	return hawkes.FitGatesFromHistory(g.radii, g.asymmetries)
}

type Signal struct {
	*runtime.System
	models *sync.Map
	keys   []string
}

func NewSignal(ctx context.Context) *Signal {
	signal := &Signal{
		models: &sync.Map{},
		keys: []string{
			"supercritical_cascade",
		},
	}

	signal.System = runtime.NewSystem(ctx, "hawkes", signal)
	return signal
}

/*
Step processes every trade entry of the tick's frame; per-symbol state is keyed
by the entry's symbol and each entry writes its own (reserved) layer.
*/
func (signal *Signal) Step(value *data.Value) *data.Value {
	var trades []kraken.TradeData
	value.Lookup(&trades, "spot:trade")

	if err := parent.RunTrades(signal.Context(), trades, func(trade kraken.TradeData) {
		signal.step(value, trade)
	}); err != nil {
		signal.Error(err)
	}

	return value
}

func (signal *Signal) step(value *data.Value, trade kraken.TradeData) *data.Value {
	if signal.Status() != runtime.READY || !parent.HasFrame(value) {
		return value
	}

	side := trade.Side

	if side != "buy" && side != "sell" {
		return value
	}

	price := trade.Price.Float64()
	qty := trade.Qty

	if price <= 0 || qty <= 0 {
		return value
	}

	at := time.Unix(0, int64(value.Timestamp))

	if at.IsZero() {
		return value
	}

	untypedState, _ := signal.models.LoadOrStore(trade.Symbol, &symbolModel{
		sample:  excitation.NewSample(),
		process: excitation.NewProcess(),
		gates:   newGates(),
	})

	state := untypedState.(*symbolModel)
	state.mu.Lock()
	defer state.mu.Unlock()

	input, ready, err := state.sample.MeasureArrival(excitation.TradeInput{
		Symbol:    trade.Symbol,
		Side:      side,
		Timestamp: at,
		UnixNano:  at.UnixNano(),
	})

	// Measure requires a horizon that includes the latest arrival. The trade's
	// own timestamp is the newest event in the window, so it satisfies that.
	input.Horizon = at

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.Internal,
			"[hawkes] arrival sample failed for "+trade.Symbol,
			err,
		))

		return value
	}

	if !ready {
		// Not enough arrivals yet for a stream: nothing to publish.
		return value
	}

	// Measure runs a projection with the latest retained parameters when the
	// process holds a fit, and requests an asynchronous refit only when the
	// event-count schedule is satisfied, so the tick loop is never blocked by
	// an optimizer.
	outcome, measureReady, err := state.process.Measure(input)

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.Internal,
			"[hawkes] excitation measure failed for "+trade.Symbol,
			err,
		))

		return value
	}

	if !measureReady {
		return value
	}

	state.gates.observe(outcome.Fit.SpectralRadius, outcome.Fit.Asymmetry(false))

	keys := []string{
		signal.Name(),
		trade.Symbol,
	}

	out := value.Append([]float64{
		outcome.Fit.SpectralRadius,
	}, append(keys, signal.keys...)...)

	audit.Record.Write(map[string]any{
		"signal":         signal.Name(),
		"trade":          trade,
		"spectralRadius": outcome.Fit.SpectralRadius,
	})

	return out
}
