package pumpdump

import (
	"context"
	"sync"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/numeric"
	"github.com/theapemachine/symm/numeric/adaptive"
)

/*
Signal measuring Pump and Dump market dynamics.

The PumpDump signal identifies pre-pump microstructure by
looking for sudden "verticality" in volume and price.
  - Volume Lift (RVOL):** Measures fast and medium-term
    volume spikes against a median hourly baseline.
  - Precursor Move: Uses a $PositiveMove$ dynamic to score how much
    the price has already begun to detach from its recent anchor.
  - Spread Compression: Scores how much the bid/ask spread
    has tightened versus its own baseline.
  - Move Classifier: A state-free primitive that maps these
    metrics into an explicit "Pump" or "Dump" class.

| Category           | Volume Lift | Price Precursor | Market "Feel"        |
|:-------------------|:------------|:----------------|:---------------------|
| Vertical Ignition  | High Spike  | High            | Launching / Breakout |
| Coiled Compression | Moderate    | Low             | Pre-Pump / Loaded    |
| Organic Trend      | Low/Steady  | Moderate        | Healthy Momentum     |
| Faded Exhaustion   | Falling     | Flat            | Leg is Dead          |

NOTE: input migration to the shared trade feed is deferred until the classifier
is settled. The pipeline still needs its last/anchor/typical inputs wired, and as
written it reshapes the value vector with a Project then reads an original index
past it — which panics once real trades flow. It currently consumes the (idle)
qpool "trades" group so it does not run.
*/
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
		symbols:     sync.Map{},
		categories: map[string]perspectives.CategoryType{
			"vertical_ignition":  perspectives.CategoryVerticalIgnition,
			"coiled_compression": perspectives.CategoryCoiledCompression,
			"faded_exhaustion":   perspectives.CategoryFadedExhaustion,
			"organic_trend":      perspectives.CategoryOrganicTrend,
		},
	}

	tradeGroup := pool.CreateBroadcastGroup("trades", 10*time.Millisecond)
	signal.subscribers["trades"] = tradeGroup.Subscribe("trades", 128)

	signal.broadcasts["measurements"] = pool.CreateBroadcastGroup(
		"measurements", 10*time.Millisecond,
	)

	return signal
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

				var last, anchor, typical float64
				var buffer any
				var category string

				for _, trade := range trades {
					if buffer, ok = signal.symbols.LoadOrStore(
						trade.Symbol, numeric.NewClassed(
							errnie.Does(func() (*adaptive.Classifier, error) {
								return adaptive.NewClassifier(
									[]float64{-0.10, 0.50, 2.00},
									[]float64{0, 1, 2, 3},
									[]string{
										"faded_exhaustion",
										"organic_trend",
										"coiled_compression",
										"vertical_ignition",
									},
								), nil
							}).Or(func(err error) {
								errnie.Error(err)
							}).Value(),

							numeric.NewAccumulate(
								numeric.NewDerived(numeric.WithDynamics(
									numeric.NewProject(func(_ float64, v []float64) []float64 {
										return []float64{v[0], v[1]} // last, anchor
									}),
									adaptive.NewRelativeMove(),
								)),
								nil,
							),

							numeric.NewAccumulate(
								numeric.NewDerived(numeric.WithDynamics(
									numeric.NewProject(func(_ float64, v []float64) []float64 {
										return []float64{v[2], v[3]} // unix nanos, qty
									}),
									adaptive.NewWindow(time.Minute),
									numeric.NewProject(func(out float64, v []float64) []float64 {
										return []float64{out, v[4]} // recent volume, typical volume
									}),
									adaptive.NewRatio(0),
								)),
								nil,
							),

							adaptive.NewSigmaClamp(3, 8, 0.0625),
						),
					); ok {
						buffer = buffer.(*numeric.Classed)
					}

					category = buffer.(*numeric.Classed).Label(
						errnie.Does(func() (float64, error) {
							return buffer.(*numeric.Classed).Push(
								last,
								anchor,
								float64(trade.Timestamp.UnixNano()),
								trade.Qty,
								typical,
							)
						}).Or(func(err error) {
							errnie.Error(err)
						}).Value(),
					)
				}

				signal.broadcasts["measurements"].Send(&qpool.QValue[any]{
					Value: perspectives.Measurement{
						Source:     perspectives.SourcePumpDump,
						Category:   signal.categories[category],
						Confidence: 1,
						SNR:        1,
					},
				})
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
