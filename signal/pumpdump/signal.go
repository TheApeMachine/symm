package pumpdump

import (
	"context"
	"math"
	"sync"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/activate"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/numeric"
	"github.com/theapemachine/symm/numeric/adaptive"
)

// pumpWindow is the recent-volume horizon the lift is measured over — short, because
// the signal is hunting "verticality": a sudden spike against the symbol's own norm.
const (
	pumpWindow      = time.Minute
	rawSubscriberID = "signal/pumpdump:raw"
)

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
	ctx         context.Context
	cancel      context.CancelFunc
	pool        *qpool.Q
	broadcasts  map[string]*qpool.BroadcastGroup
	subscribers map[string]*qpool.Subscriber
	symbols     sync.Map
	categories  map[string]perspectives.CategoryType
	floor       *adaptive.SNRField
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
			"faded_exhaustion":   perspectives.CategoryFadedExhaustion,
			"organic_trend":      perspectives.CategoryOrganicTrend,
			"coiled_compression": perspectives.CategoryCoiledCompression,
			"vertical_ignition":  perspectives.CategoryVerticalIgnition,
		},
		floor: adaptive.NewSNRField(),
	}

	for _, channel := range []string{"raw"} {
		signal.broadcasts[channel] = pool.CreateBroadcastGroup(channel, 10*time.Millisecond)
		signal.subscribers[channel] = signal.broadcasts[channel].Subscribe(rawSubscriberID, 1024)
	}

	signal.broadcasts["measurements"] = pool.CreateBroadcastGroup("measurements", 10*time.Millisecond)

	activate.Boot("signal/pumpdump ready")

	return signal
}

/*
pumpState is one symbol's ignition state. The volume window carries the recent
executed size and the window's opening price (its anchor); the two EMAs are the
self-scaling baselines lift and precursor are read against; pipe fuses and bands the
two axes into a category.
*/
type pumpState struct {
	volume   *adaptive.Window
	volBase  *adaptive.EMA
	moveBase *adaptive.EMA
	pipe     *numeric.Classed
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

			numeric.NewProjectScalar(func(_ float64, values []float64) float64 {
				return (values[0] - 1) * (1 + values[1])
			}),
			adaptive.NewEMA(0),
			adaptive.NewSigmaClamp(3, 8, 0.0625),
		),
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
	for {
		select {
		case <-signal.ctx.Done():
			return signal.ctx.Err()
		case message := <-signal.subscribers["raw"].Incoming:
			if message == nil || message.Value == nil {
				continue
			}

			envelope, ok := message.Value.(public.SocketMessage)

			if !ok {
				continue
			}

			switch envelope.Channel {
			case public.TradesChannel:
				trades, err := market.DecodeTrades(&envelope)

				if err != nil {
					errnie.Error(err, "pumpdump: decode trades")
					continue
				}

				for _, trade := range trades {
					if err := signal.observe(trade); err != nil {
						errnie.Error(err, "pumpdump: observe %s", trade.Symbol)
						continue
					}
				}
			}
		}
	}
}

// observe folds one executed trade into its symbol's window state and emits the
// ignition reading for that symbol.
func (signal *Signal) observe(trade market.TradeUpdate) error {
	if trade.Price <= 0 || trade.Qty <= 0 {
		return nil
	}

	stored, _ := signal.symbols.LoadOrStore(trade.Symbol, newPumpState())
	state := stored.(*pumpState)

	nanos := float64(trade.Timestamp.UnixNano())
	state.volume.Next(0, nanos, trade.Qty, trade.Price) // anchor = window opening price
	state.last = trade.Price

	anchor := state.volume.Anchor()

	if anchor <= 0 {
		return nil
	}

	rvol := state.scale(state.volume.Sum(), state.volBase)
	precursor := state.scale(math.Max(0, (state.last-anchor)/anchor), state.moveBase)

	code, err := state.pipe.Push(rvol, precursor)

	if err != nil {
		return err
	}

	ignition := math.Max(0, (rvol-1)*(1+precursor))

	measurement := perspectives.Measurement{
		Symbol:     trade.Symbol,
		Source:     perspectives.SourcePumpDump,
		Category:   signal.categories[state.pipe.Label(code)],
		Last:       trade.Price,
		Strength:   ignition,
		Confidence: state.pipe.Confidence(),
	}
	if err := perspectives.AssignCategorySNR(
		&measurement, signal.floor, state.pipe.Standout(),
	); err != nil {
		return err
	}

	activate.Once("signal/pumpdump:measurement")
	signal.broadcasts["measurements"].Send(&qpool.QValue[any]{Value: measurement})

	return nil
}

func (signal *Signal) Close() error {
	signal.cancel()
	return nil
}
