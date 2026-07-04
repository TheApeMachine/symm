package cvd

import (
	"context"
	"iter"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
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
	tree   *dmt.Tree
	trade  *Trade
}

/*
NewSignal constructs the CVD signal. The tree is held for the shared signal
constructor contract; the trade role owns its rolling artifact clock.
*/
func NewSignal(
	ctx context.Context,
	tree *dmt.Tree,
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	return &Signal{
		ctx:    ctx,
		cancel: cancel,
		tree:   tree,
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
	datapoint *datura.Artifact,
	crossSection *market.CrossSection,
) iter.Seq[*datura.Artifact] {
	return func(yield func(*datura.Artifact) bool) {
		role := datura.Peek[string](datapoint, "role")

		if role != "trade" {
			return
		}

		data := datura.Peek[[]any](datapoint, "data")

		for _, item := range data {
			row, ok := item.(map[string]any)

			if !ok {
				if !yield(datapoint.WithError(errnie.Error(errnie.Err(
					errnie.UnprocessableContent,
					"cvd: row object required",
					nil,
				)))) {
					return
				}

				continue
			}

			symbol, ok := row["symbol"].(string)

			if !ok {
				if !yield(datapoint.WithError(errnie.Error(errnie.Err(
					errnie.UnprocessableContent,
					"cvd: row symbol required",
					nil,
				)))) {
					return
				}

				continue
			}

			rowArtifact := datura.Acquire(
				"cvd", datura.APPJSON,
			).WithRole(
				"measurement",
			).WithScope(
				symbol,
			).WithPayload(
				datura.Map[any](row).Marshal(),
			)
			rowArtifact.SetTimestamp(datapoint.Timestamp())
			errnie.Error(rowArtifact.SetOrigin(string(logic.SourceCVD)))

			measurement := signal.trade.Measure(rowArtifact, crossSection)
			if measurement == nil {
				continue
			}

			if !yield(measurement) {
				return
			}
		}
	}
}

func (signal *Signal) Error() error {
	return errnie.Error(signal.err)
}

func (signal *Signal) Close() (err error) {
	err = signal.err
	signal.cancel()

	return errnie.Error(err)
}

func completeMeasurement(frame *datura.Artifact) *datura.Artifact {
	evidence := datura.Peek[float64](frame, "output", "absorption") +
		datura.Peek[float64](frame, "output", "drive") +
		datura.Peek[float64](frame, "output", "balance") +
		datura.Peek[float64](frame, "output", "starvation")

	if evidence <= 0 {
		return nil
	}

	if datura.Peek[float64](frame, "output", "value") > 0 &&
		datura.Peek[float64](frame, "output", "confidence") > 0 &&
		datura.Peek[float64](frame, "output", "entry_baseline") > 0 &&
		datura.Peek[float64](frame, "output", "exit_baseline") > 0 {
		frame.Poke("output", "root")
		return frame
	}

	return nil
}
