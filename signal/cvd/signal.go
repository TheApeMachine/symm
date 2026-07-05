package cvd

import (
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
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
type Signal struct {
	ctx    context.Context
	cancel context.CancelFunc
	err    error
	trade  *Trade
}

/*
NewSignal constructs the CVD signal. The tree is held for the shared signal
constructor contract; the trade role owns its rolling artifact clock.
*/
func NewSignal(ctx context.Context) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	return &Signal{
		ctx:    ctx,
		cancel: cancel,
		trade:  NewTrade(),
	}
}

func (signal *Signal) IngestRoles() []string {
	return []string{"trade"}
}

/*
Measure routes trade rows into the CVD trade-flow role object.
*/
func (signal *Signal) Measure(
	input market.Input,
	crossSection *market.CrossSection,
) ([]*logic.Measurement, error) {
	if input.Role != "trade" {
		return nil, nil
	}

	measurements := make([]*logic.Measurement, 0, len(input.Trade))
	for _, row := range input.Trade {
		measurement, err := signal.trade.Measure(row)

		if err != nil {
			return nil, errnie.Error(errnie.Err(
				errnie.UnprocessableContent, err.Error(), err,
			))
		}

		if measurement == nil {
			continue
		}

		measurements = append(measurements, measurement)
	}

	return measurements, nil
}

func (signal *Signal) Error() error {
	return errnie.Error(signal.err)
}

func (signal *Signal) Close() (err error) {
	err = signal.err
	signal.cancel()

	return errnie.Error(err)
}
