package cvd

import (
	"context"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

/*
Signal: The Absorption Perspective

What it measures exactly (in isolation)

The CVD signal measures signed aggressor-buy versus aggressor-sell notional in
the current trade-flow window. The window is derived from observed notional
history by the shared trade-flow sampler, not from a fixed wall-clock horizon.

* Net Fraction: The ratio of net notional (buys minus sells) to gross notional.
The directional gate is derived from the active trade count.
* Price Suppression: It compares price drift against the median absolute move
inside the active flow window.
* Tick Integrity: It reads the executed trade tape rather than L2 book shape, so
it does not infer spoof/cancel intent from aggregate book deltas.

Semantically, what story does it tell?

* The "Iceberg" Story: It identifies when a massive participant is "hidden" in the book, absorbing every market order without letting the price move.
It tells us that what looks like a range-bound market is actually a site of heavy accumulation or distribution.
* The "Authentic Move" Story: It verifies price trends. If price is rising but CVD is flat or negative, the move is a "trap" or "low-conviction."
If price and CVD move together, the trend is **structurally supported**.

#### **Probability Visualization Categories**

| Category               | Net Volume | Price Drift | Market "Feel"                    |
|:-----------------------|:-----------|:------------|:---------------------------------|
| **Hidden Absorption**  | High       | Flat        | **Bullish/Bearish Iceberg**      |
| **Aggressive Drive**   | High       | High        | **Strong Trend Support**         |
| **Stochastic Balance** | Low        | Variable    | **Equilibrium/Choppy**           |
| **Volume Starvation**  | Very Low   | Flat        | **Dying Interest**               |
*/
type Signal[T any] struct {
	ctx    context.Context
	cancel context.CancelFunc
	err    error
	trade  *Trade
}

/*
NewSignal constructs the CVD signal. The tree is held for the shared signal
constructor contract; the trade role owns its rolling artifact clock.
*/
func NewSignal[T any](ctx context.Context) *Signal[T] {
	ctx, cancel := context.WithCancel(ctx)

	return &Signal[T]{
		ctx:    ctx,
		cancel: cancel,
		trade:  NewTrade(),
	}
}

func (signal *Signal[T]) IngestRoles() []string {
	return []string{"trade"}
}

func (signal *Signal[T]) Measure(
	input T,
	crossSection *types.CrossSection,
) ([]*types.Measurement, error) {
	switch row := any(input).(type) {
	case kraken.TradeData:
		return signal.trade.Measure(row)
	}

	return nil, nil
}

func (signal *Signal[T]) Error() error {
	return signal.err
}

func (signal *Signal[T]) Close() (err error) {
	err = signal.err
	signal.cancel()

	return err
}
