package pumpdump

import (
	"context"
	"math"
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

// pumpWindow is the recent-volume horizon the lift is measured over — short, because
// the signal is hunting "verticality": a sudden spike against the symbol's own norm.
const pumpWindow = time.Minute

/*
Signal measuring Pump and Dump market dynamics — the ignition perspective.

It reads the trade tape and looks for sudden verticality: a volume spike (RVOL)
detaching from the symbol's own recent norm, optionally amplified by a precursor
price move off the window's opening anchor. Both axes are self-scaling — read as
value / EMA(value), so "high", "moderate" and "falling" mean relative to this
symbol's own recent behaviour, never a hard-coded level — then fused, smoothed,
sigma-clamped, and banded into the four ignition categories:

| Category           | Volume Lift | Price Precursor | Market "Feel"        |
|:-------------------|:------------|:----------------|:---------------------|
| Vertical Ignition  | High Spike  | High            | Launching / Breakout |
| Coiled Compression | Moderate    | Low             | Pre-Pump / Loaded    |
| Organic Trend      | Low/Steady  | Moderate        | Healthy Momentum     |
| Faded Exhaustion   | Falling     | Flat            | Leg is Dead          |

Spread compression (a third axis in the written design) needs the book and is not
available on the trade tape this signal consumes, so it is left to the book-driven
signals; here ignition is read from executed volume and price alone.
*/
type Signal struct {
	ctx        context.Context
	cancel     context.CancelFunc
	pool       *qpool.Q
	broadcasts map[string]*qpool.BroadcastGroup
	symbols    sync.Map
	categories map[string]perspectives.CategoryType
}

func NewSignal(ctx context.Context, pool *qpool.Q) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		ctx:        ctx,
		cancel:     cancel,
		pool:       pool,
		broadcasts: make(map[string]*qpool.BroadcastGroup),
		categories: map[string]perspectives.CategoryType{
			"faded_exhaustion":   perspectives.CategoryFadedExhaustion,
			"organic_trend":      perspectives.CategoryOrganicTrend,
			"coiled_compression": perspectives.CategoryCoiledCompression,
			"vertical_ignition":  perspectives.CategoryVerticalIgnition,
		},
	}

	signal.broadcasts["measurements"] = pool.CreateBroadcastGroup(
		"measurements", 10*time.Millisecond,
	)

	return signal
}

/*
pumpState is one symbol's ignition state. The volume window carries the recent
executed size and the window's opening price (its anchor); the two EMAs are the
self-scaling baselines lift and precursor are read against; pipe fuses and bands the
two axes into a category; floor scores the ignition strength against its own noise.
*/
type pumpState struct {
	volume   *adaptive.Window // recent volume, anchored at the window's opening price
	volBase  *adaptive.EMA    // self-scaling baseline for volume lift (RVOL)
	moveBase *adaptive.EMA    // self-scaling baseline for the precursor move
	pipe     *numeric.Classed
	floor    *adaptive.SNR
	last     float64
}

func newPumpState() *pumpState {
	return &pumpState{
		volume:   adaptive.NewWindow(pumpWindow),
		volBase:  adaptive.NewEMA(0),
		moveBase: adaptive.NewEMA(0),
		pipe: numeric.NewClassed(
			adaptive.NewClassifier(
				[]float64{-0.10, 0.50, 2.00}, // faded | organic | coiled | ignition
				[]float64{0, 1, 2, 3},
				[]string{"faded_exhaustion", "organic_trend", "coiled_compression", "vertical_ignition"},
			),

			numeric.NewProject(func(_ float64, values []float64) []float64 {
				return []float64{(values[0] - 1) * (1 + values[1])} // (RVOL − 1) amplified by precursor
			}),
			adaptive.NewEMA(0),
			adaptive.NewSigmaClamp(3, 8, 0.0625),
		),
		floor: adaptive.NewSNR(),
	}
}

// scale reads a value relative to its own running norm — the dimensionless,
// constant-free pivot (1.0 means "exactly normal").
func (state *pumpState) scale(value float64, base *adaptive.EMA) float64 {
	norm := base.Value()
	_, _ = base.Next(0, value)

	if norm <= 0 {
		return 1
	}

	return value / norm
}

func (signal *Signal) Tick() error {
	for trade := range market.NewTradeSubscription(signal.ctx, config.System.Symbols...) {
		if trade != nil {
			signal.observe(*trade)
		}
	}

	return signal.ctx.Err()
}

// observe folds one executed trade into its symbol's window state and emits the
// ignition reading for that symbol.
func (signal *Signal) observe(trade market.TradeUpdate) {
	if trade.Price <= 0 || trade.Qty <= 0 {
		return
	}

	stored, _ := signal.symbols.LoadOrStore(trade.Symbol, newPumpState())
	state := stored.(*pumpState)

	nanos := float64(trade.Timestamp.UnixNano())
	state.volume.Next(0, nanos, trade.Qty, trade.Price) // anchor = window opening price
	state.last = trade.Price

	anchor := state.volume.Anchor()

	if anchor <= 0 {
		return
	}

	rvol := state.scale(state.volume.Sum(), state.volBase)
	precursor := state.scale(math.Max(0, (state.last-anchor)/anchor), state.moveBase)

	code, err := state.pipe.Push(rvol, precursor)

	if err != nil {
		errnie.Error(err)

		return
	}

	ignition := math.Max(0, (rvol-1)*(1+precursor)) // positive strength the floor scores

	measurement := perspectives.FinalizeMeasurement(perspectives.Measurement{
		Symbol:   trade.Symbol,
		Source:   perspectives.SourcePumpDump,
		Category: signal.categories[state.pipe.Label(code)],
		Last:     trade.Price,
	}, ignition, "ignition")
	signal.broadcasts["measurements"].Send(&qpool.QValue[any]{Value: measurement})
}

func (signal *Signal) Close() error {
	signal.cancel()
	return nil
}
