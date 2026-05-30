package cvd

import (
	"context"
	"math"
	"sync"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/numeric"
	"github.com/theapemachine/symm/numeric/adaptive"
)

const cvdWindow = 15 * time.Minute // executed-flow horizon

/*
Signal measuring executed-flow absorption (cumulative volume delta).

It reads the trade tape — not the book — so it is immune to spoofing, and looks
for divergence between one-sided executed flow and price drift. Every threshold
is self-scaling: each axis is read as value / EMA(value), so "high", "low" and
"flat" mean relative to the symbol's own recent norm, not a hard-coded level.

  - Conviction: |net|/gross (taker buys − sells over total), versus its own norm.
  - Activity:   trade count in the window, versus its own norm.
  - Drift:      |price move| from the window's open, versus its own norm.

fused = activity · conviction · (1 + drift), self-scaling, then SigmaClamp,
then banded into the four absorption categories:

| Category           | Net Volume | Price Drift | Market "Feel"           |
|:-------------------|:-----------|:------------|:------------------------|
| Hidden Absorption  | High       | Flat        | Bullish/Bearish Iceberg |
| Aggressive Drive   | High       | High        | Strong Trend Support    |
| Stochastic Balance | Low        | Variable    | Equilibrium / Choppy    |
| Volume Starvation  | Very Low   | Flat        | Dying Interest          |
*/
type cvdState struct {
	signed    *adaptive.Window // taker-signed volume; anchored at the opening price
	gross     *adaptive.Window // absolute volume
	count     *adaptive.Window // trade count
	convBase  *adaptive.EMA    // self-scaling baseline for conviction
	actBase   *adaptive.EMA    // self-scaling baseline for activity
	driftBase *adaptive.EMA    // self-scaling baseline for drift
	pipe      *numeric.Classed
	last      float64
}

type Signal struct {
	ctx         context.Context
	cancel      context.CancelFunc
	pool        *qpool.Q
	broadcasts  map[string]*qpool.BroadcastGroup
	subscribers map[string]*qpool.Subscriber
	symbols     sync.Map
	categories  map[string]perspectives.CategoryType
}

func NewSignal(ctx context.Context, pool *qpool.Q) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		broadcasts:  make(map[string]*qpool.BroadcastGroup),
		subscribers: make(map[string]*qpool.Subscriber),
		categories: map[string]perspectives.CategoryType{
			"volume_starvation":  perspectives.CategoryVolumeStarvation,
			"stochastic_balance": perspectives.CategoryStochasticBalance,
			"hidden_absorption":  perspectives.CategoryHiddenAbsorption,
			"aggressive_drive":   perspectives.CategoryAggressiveDrive,
		},
	}

	tradeGroup := pool.CreateBroadcastGroup("trades", 10*time.Millisecond)
	signal.subscribers["trades"] = tradeGroup.Subscribe("trades", 128)

	signal.broadcasts["measurements"] = pool.CreateBroadcastGroup(
		"measurements", 10*time.Millisecond,
	)

	return signal
}

func newCVDState() *cvdState {
	return &cvdState{
		signed:    adaptive.NewWindow(cvdWindow),
		gross:     adaptive.NewWindow(cvdWindow),
		count:     adaptive.NewWindow(cvdWindow),
		convBase:  adaptive.NewEMA(0),
		actBase:   adaptive.NewEMA(0),
		driftBase: adaptive.NewEMA(0),
		pipe: numeric.NewClassed(
			adaptive.NewClassifier(
				[]float64{0.60, 1.50, 3.50}, // starvation | balance | absorption | drive
				[]float64{0, 1, 2, 3},
				[]string{"volume_starvation", "stochastic_balance", "hidden_absorption", "aggressive_drive"},
			),

			numeric.NewProject(func(_ float64, v []float64) []float64 {
				return []float64{v[0] * v[1] * (1 + v[2])} // activity · conviction · (1 + drift)
			}),
			adaptive.NewEMA(0),
			adaptive.NewSigmaClamp(3, 8, 0.0625),
		),
	}
}

// scale reads a value relative to its own running norm — the dimensionless,
// constant-free pivot (1.0 means "exactly normal").
func (state *cvdState) scale(value float64, base *adaptive.EMA) float64 {
	norm := base.Value()
	_, _ = base.Next(0, value)

	if norm <= 0 {
		return 1
	}

	return value / norm
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

				touched := make(map[string]*cvdState, len(trades))

				for _, trade := range trades {
					if trade.Price <= 0 || trade.Qty <= 0 {
						continue
					}

					stored, _ := signal.symbols.LoadOrStore(trade.Symbol, newCVDState())
					state := stored.(*cvdState)

					signed := trade.Qty // taker buy lifts the ask

					if trade.Side != "buy" {
						signed = -trade.Qty
					}

					nanos := float64(trade.Timestamp.UnixNano())
					state.signed.Next(0, nanos, signed, trade.Price) // anchor = opening price
					state.gross.Next(0, nanos, trade.Qty)
					state.count.Next(0, nanos, 1)
					state.last = trade.Price

					touched[trade.Symbol] = state
				}

				for _, state := range touched {
					gross := state.gross.Sum()
					anchor := state.signed.Anchor()

					if gross <= 0 || anchor <= 0 {
						continue
					}

					conviction := state.scale(math.Abs(state.signed.Sum()/gross), state.convBase)
					activity := state.scale(state.count.Sum(), state.actBase)
					drift := state.scale(math.Abs((state.last-anchor)/anchor), state.driftBase)

					code, err := state.pipe.Push(activity, conviction, drift)

					if err != nil {
						errnie.Error(err)
						continue
					}

					signal.broadcasts["measurements"].Send(&qpool.QValue[any]{
						Value: perspectives.Measurement{
							Source:   perspectives.SourceCVD,
							Category: signal.categories[state.pipe.Label(code)],
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
