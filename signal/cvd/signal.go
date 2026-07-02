package cvd

import (
	"context"
	"iter"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/market"
)

/*
Signal: The Absorption Perspective

What it measures exactly (in isolation)

The CVD signal measures the net difference between aggressor-buy volume and aggressor-sell volume over a 15-minute window. 
It specifically looks for a divergence between executed flow and price drift.

* Net Fraction: The ratio of net volume (buys minus sells) to gross volume (total trades). 
A directional read requires a fraction of at least 0.60.
* Price Suppression: It measures if the price is staying within a "flat band" ($\leq 0.3\%$) despite heavy one-sided buying or selling.
* Tick Integrity: Because it reads the executed trade tape rather than the book, it is immune to spoofing.

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
| **Volume Starvation**  | Very Low   | Flat        | **Dying Interest (< 40 trades)** |

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

			if !yield(signal.trade.Measure(rowArtifact, crossSection)) {
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
