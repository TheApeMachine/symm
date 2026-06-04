package cvd

import (
	"context"
	"encoding/json"
	"math"
	"sync"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/numeric"
	"github.com/theapemachine/symm/numeric/adaptive"
	"github.com/theapemachine/symm/ring"
	signalpool "github.com/theapemachine/symm/signal"
)

const (
	cvdWindow           = 15 * time.Minute // executed-flow horizon
	minCVDFusedSamples  = 12
	cvdFusedHistorySize = 1024
	rawSubscriberID     = "signal/cvd:raw"
)

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
then banded into the four absorption categories. Confidence is classification
clarity — margin to the nearest quartile boundary; SNR is how surprising that
clarity is versus the symbol's own recent baseline, not raw strength.

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
	sigma     *adaptive.SigmaClamp
	fusedHist ring.FloatRing
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
			"volume_starvation":  perspectives.CategoryVolumeStarvation,
			"stochastic_balance": perspectives.CategoryStochasticBalance,
			"hidden_absorption":  perspectives.CategoryHiddenAbsorption,
			"aggressive_drive":   perspectives.CategoryAggressiveDrive,
		},
		floor: adaptive.NewSNRField(),
	}

	for _, channel := range []string{"raw"} {
		signal.broadcasts[channel] = pool.CreateBroadcastGroup(channel, 10*time.Millisecond)
		signal.subscribers[channel] = signal.broadcasts[channel].Subscribe(rawSubscriberID, 1024)
	}

	signal.broadcasts["measurements"] = pool.CreateBroadcastGroup("measurements", 10*time.Millisecond)
	signal.broadcasts["ui"] = pool.CreateBroadcastGroup("ui", 10*time.Millisecond)

	errnie.Info("signal/cvd ready", "signal/cvd")

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
		sigma:     adaptive.NewSigmaClamp(3, 8, 0.0625),
		fusedHist: ring.NewFloatRing(cvdFusedHistorySize),
	}
}

var cvdBandCodes = []float64{0, 1, 2, 3}

var cvdBandLabels = []string{
	"volume_starvation",
	"stochastic_balance",
	"hidden_absorption",
	"aggressive_drive",
}

/*
measureFused sigma-clamps fused absorption strength, bands it against the
symbol's own fused history quartiles, and returns the category plus clarity
(classification confidence from the quartile boundaries).
*/
func (state *cvdState) measureFused(fused float64) (perspectives.CategoryType, float64, float64) {
	clamped, err := state.sigma.Next(0, fused)

	if err != nil {
		clamped = fused
	}

	state.fusedHist.Push(clamped)
	samples := state.fusedHist.Ordered()

	if len(samples) < minCVDFusedSamples {
		return perspectives.CategoryStochasticBalance, 0, 0
	}

	sorted := numeric.CopySorted(samples)
	q1 := numeric.PercentileSorted(sorted, 0.25)
	q2 := numeric.PercentileSorted(sorted, 0.50)
	q3 := numeric.PercentileSorted(sorted, 0.75)
	classifier := adaptive.NewClassifier(
		[]float64{q1, q2, q3},
		cvdBandCodes,
		cvdBandLabels,
	)
	clarity := classifier.Confidence(clamped)
	standout := classifier.Standout(clamped)

	switch {
	case clamped <= q1:
		return perspectives.CategoryVolumeStarvation, clarity, standout
	case clamped <= q2:
		return perspectives.CategoryStochasticBalance, clarity, standout
	case clamped <= q3:
		return perspectives.CategoryHiddenAbsorption, clarity, standout
	default:
		return perspectives.CategoryAggressiveDrive, clarity, standout
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
	for {
		select {
		case <-signal.ctx.Done():
			return signal.ctx.Err()
		case message := <-signal.subscribers["raw"].Incoming:
			if message == nil || message.Value == nil {
				continue
			}

			envelope, ok := message.Value.(map[string]any)

			if !ok {
				continue
			}

			channel, _ := envelope["channel"].(string)
			rawData, _ := envelope["data"].(json.RawMessage)
			sm := &public.SocketMessage{Channel: channel, Data: rawData}

			switch channel {
			case public.TradesChannel:
				trades := signalpool.GetTrades(sm)

				for _, trade := range trades {
					if err := signal.observe(trade); err != nil {
						errnie.Error(err, "cvd: observe %s", trade.Symbol)
						continue
					}
				}
			}
		}
	}
}

// observe folds one executed trade into its symbol's window state and emits the
// absorption reading for that symbol.
func (signal *Signal) observe(trade market.TradeUpdate) error {
	if trade.Price <= 0 || trade.Qty <= 0 {
		return nil
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

	gross := state.gross.Sum()
	anchor := state.signed.Anchor()

	if gross <= 0 || anchor <= 0 {
		return nil
	}

	conviction := state.scale(math.Abs(state.signed.Sum()/gross), state.convBase)
	activity := state.scale(state.count.Sum(), state.actBase)
	drift := state.scale(math.Abs((state.last-anchor)/anchor), state.driftBase)

	fused := activity * conviction * (1 + drift)
	category, clarity, standout := state.measureFused(fused)

	measurement := perspectives.Measurement{
		Symbol:     trade.Symbol,
		Source:     perspectives.SourceCVD,
		Category:   category,
		Last:       trade.Price,
		Strength:   fused,
		Confidence: clarity,
	}
	if err := perspectives.AssignCategorySNR(&measurement, signal.floor, standout); err != nil {
		return err
	}

	signal.broadcasts["measurements"].Send(&qpool.QValue[any]{Value: measurement})

	if ui := signal.broadcasts["ui"]; ui != nil {
		ui.Send(&qpool.QValue[any]{
			Value: map[string]any{
				"chart":      "gauge",
				"source":     measurement.Source.String(),
				"confidence": measurement.Confidence,
				"snr":        measurement.SNR,
			},
		})
	}

	return nil
}

func (signal *Signal) Close() error {
	signal.cancel()
	return nil
}
