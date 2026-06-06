package causal

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/bus"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/market/perspectives/types"
	"github.com/theapemachine/symm/numeric"
	"github.com/theapemachine/symm/numeric/adaptive"
	"github.com/theapemachine/symm/rawdump"
	"github.com/theapemachine/symm/ring"
	signalpool "github.com/theapemachine/symm/signal"
)

/*
Signal scores Pearl's ladder — association, intervention (backdoor-adjusted),
counterfactual uplift — over a DAG of MacroMomentum → PriceVelocity ← LocalFlow
with Liquidity as a backdoor control, switching to a panic regime when
cross-asset contagion or collinearity spikes. It consumes trades (flow), ticks
(macro change), and book (liquidity); the heavy fit lives in CausalSymbol.

causalPublishInterval throttles the cross-sectional causal fit. The fit is
O(symbols²) (each symbol's reading depends on the macro median of the rest), so
running it on every trade saturates a core; the structural picture does not
change meaningfully faster than this.

| Category         | Active Regime | Dominant Factor       | Market "Feel"      |
|:-----------------|:--------------|:----------------------|:-------------------|
| Endogenous Alpha | Normal        | Counterfactual Uplift | Driven/Independent |
| Systemic Beta    | Normal        | Macro Momentum        | Drifting/Passive   |
| Liquidity Shock  | Panic         | Liquidity Void        | Fragile/Inverted   |
| Causal Noise     | Variable      | None                  | Stochastic/Unclear |
*/
const (
	causalPublishInterval = 500 * time.Millisecond
	rawSubscriberID       = "signal/causal:raw"

	// contagionCheckInterval bounds how often the cheap Hayashi-Yoshida contagion
	// tripwire is recomputed — far faster than the heavy fit, but not every tick.
	contagionCheckInterval = 100 * time.Millisecond
	// causalEmergencyBackoffBase/Max bound the escalating circuit breaker after a
	// velocity-triggered refit, so a violent contagion ramp cannot refit every tick
	// and drive the core into a death spiral.
	causalEmergencyBackoffBase = 100 * time.Millisecond
	causalEmergencyBackoffMax  = 2 * time.Second
)

var causalDefaultBandEdges = []float64{0.5, 1.5, 3.0}

type Signal struct {
	ctx              context.Context
	cancel           context.CancelFunc
	pool             *qpool.Q
	broadcasts       map[string]*qpool.BroadcastGroup
	subscribers      map[string]*qpool.Subscriber
	symbols          sync.Map
	lastPublish      time.Time
	contagionAt      time.Time
	cachedContagion  float64
	contagionSpread  ring.FloatRing
	fitContagion     float64
	emergencyBackoff time.Duration
	backoffUntil     time.Time
	publishScratch   []publishEntry
	surpriseField    *types.CategorySurpriseField
	classifier       *adaptive.Classifier
	calibrator       *numeric.BandCalibrator
	rawDump          *rawdump.Writer
}

type publishEntry struct {
	symbol string
	state  *CausalSymbol
	change float64
}

func NewSignal(ctx context.Context, pool *qpool.Q) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	pooledCalibrator := numeric.NewSignalCalibrator(
		causalDefaultBandEdges,
		[]float64{0, 1, 2, 3},
		[]string{"causal_noise", "systemic_beta", "endogenous_alpha", "liquidity_shock"},
		[]float64{0.40, 0.30, 0.20, 0.10},
		numeric.DefaultCalibratorConfig("strength"),
		"causal",
	)

	signal := &Signal{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		broadcasts:  make(map[string]*qpool.BroadcastGroup),
		subscribers: make(map[string]*qpool.Subscriber),
		surpriseField: types.NewCategorySurpriseField([]types.CategoryType{
			types.CategoryCausalNoise,
			types.CategorySystemicBeta,
			types.CategoryEndogenousAlpha,
			types.CategoryLiquidityShock,
		}, types.DefaultCategorySurpriseAlpha),
		classifier:      pooledCalibrator.Classifier,
		calibrator:      pooledCalibrator.Calibrator,
		contagionSpread: ring.NewFloatRing(64),
		rawDump:         rawdump.Open("causal"),
	}

	for _, channel := range []string{"raw"} {
		signal.broadcasts[channel] = bus.Group(pool, channel, 10*time.Millisecond)
		signal.subscribers[channel] = signal.broadcasts[channel].Subscribe(rawSubscriberID, 1024)
	}

	signal.broadcasts["measurements"] = bus.Group(pool, "measurements", 10*time.Millisecond)
	signal.broadcasts["ui"] = bus.Group(pool, "ui", 10*time.Millisecond)

	errnie.Info("signal/causal ready", "signal/causal")

	return signal
}

func (signal *Signal) state(symbol string) *CausalSymbol {
	if stored, ok := signal.symbols.Load(symbol); ok {
		return stored.(*CausalSymbol)
	}

	created := NewCausalSymbol()
	stored, loaded := signal.symbols.LoadOrStore(symbol, created)

	if loaded {
		return stored.(*CausalSymbol)
	}

	return created
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

			sm, ok := signalpool.SocketMessageFromValue(message.Value)

			if !ok {
				continue
			}

			switch sm.Channel {
			case public.TradesChannel:
				trades := signalpool.GetTrades(sm)
				publishAt := time.Time{}

				for _, trade := range trades {
					if trade.Timestamp.IsZero() {
						errnie.Error(
							fmt.Errorf("causal: trade timestamp is required"),
							"causal: feed trade %s",
							trade.Symbol,
						)
						continue
					}

					if err := signal.state(trade.Symbol).FeedTrade(trade); err != nil {
						errnie.Error(err, "causal: feed trade %s", trade.Symbol)
						continue
					}

					if trade.Timestamp.After(publishAt) {
						publishAt = trade.Timestamp
					}
				}

				if publishAt.IsZero() {
					continue
				}

				if err := signal.publishAt(publishAt); err != nil {
					errnie.Error(err, "causal: publish")
					continue
				}
			case public.TickerChannel:
				tickers := signalpool.GetTickers(sm)
				publishAt := time.Time{}

				for _, ticker := range tickers {
					at, err := causalTickerTime(ticker)

					if err != nil {
						errnie.Error(err, "causal: feed ticker %s", ticker.Symbol)
						continue
					}

					signal.state(ticker.Symbol).FeedTicker(ticker)

					if at.After(publishAt) {
						publishAt = at
					}
				}

				if publishAt.IsZero() {
					continue
				}

				if err := signal.publishAt(publishAt); err != nil {
					errnie.Error(err, "causal: publish")
					continue
				}
			case public.BookChannel:
				books := signalpool.GetBooks(sm)
				publishAt := time.Time{}

				for _, delta := range books {
					at, err := causalBookTime(delta)

					if err != nil {
						errnie.Error(err, "causal: feed book %s", delta.Symbol)
						continue
					}

					signal.state(delta.Symbol).FeedBook(delta)

					if at.After(publishAt) {
						publishAt = at
					}
				}

				if publishAt.IsZero() {
					continue
				}

				if err := signal.publishAt(publishAt); err != nil {
					errnie.Error(err, "causal: publish")
					continue
				}
			}
		}
	}
}

// causalContagionShift is how far cross-asset contagion must move since the last
// fit to bypass the time gate. A liquidation cascade flips the causal structure in
// milliseconds — faster than a rigid timer would catch.
func causalContagionShift() float64 {
	shift := viper.GetViper().GetFloat64("signals.causal.contagion_shift")

	if shift > 0 {
		return shift
	}

	return 0.2
}

// causalPublishIntervalForRegime shortens the fit cadence in calm regimes and
// lengthens it during hostile price action so O(symbols²) work concentrates when
// the structural picture is stable.
func causalPublishIntervalForRegime(regime types.Regime) time.Duration {
	switch regime {
	case types.RegimeDead, types.RegimeTrending, types.RegimeBullish:
		return causalPublishInterval / 2
	case types.RegimeChoppy, types.RegimeBearish:
		return causalPublishInterval * 2
	default:
		return causalPublishInterval
	}
}

/*
throttle decides whether to rerun the O(symbols²) causal fit. It fires on the time
gate as before, OR immediately when contagion has broken away since the last fit
(the Hayashi-Yoshida liquidation signature) — velocity-aware, not blindly
time-based, so the structural model is not navigating on a stale snapshot during a
flash crash. An exponential backoff then caps how often the emergency refit can
re-fire: the freshly computed panic DAG is relied on for an escalating window so a
sustained spike cannot saturate the core. now is passed in so the gate is testable.
*/
func (signal *Signal) throttle(now time.Time, contagion float64) bool {
	interval := causalPublishIntervalForRegime(perspectives.CurrentRegime())

	if now.Sub(signal.lastPublish) >= interval {
		signal.commitFit(now, contagion, false)

		return true
	}

	shift := math.Abs(contagion - signal.fitContagion)

	if shift >= causalContagionShift() && !now.Before(signal.backoffUntil) {
		signal.commitFit(now, contagion, true)

		return true
	}

	return false
}

// commitFit records a fit and, for a velocity-triggered emergency, escalates the
// backoff circuit breaker; a normal time-gated fit means the storm (if any) has
// passed, so the backoff resets.
func (signal *Signal) commitFit(now time.Time, contagion float64, emergency bool) {
	signal.lastPublish = now
	signal.fitContagion = contagion

	if !emergency {
		signal.emergencyBackoff = 0

		return
	}

	if signal.emergencyBackoff == 0 {
		signal.emergencyBackoff = causalEmergencyBackoffBase
	} else {
		signal.emergencyBackoff *= 2

		if signal.emergencyBackoff > causalEmergencyBackoffMax {
			signal.emergencyBackoff = causalEmergencyBackoffMax
		}
	}

	signal.backoffUntil = now.Add(signal.emergencyBackoff)
}

// publish runs the causal fit for every symbol against the current cross-asset
// macro momentum and contagion, emitting one structural reading each.
func (signal *Signal) publish() error {
	return signal.publishAt(time.Now())
}

func (signal *Signal) publishAt(now time.Time) error {

	// Recompute the cheap Hayashi-Yoshida contagion tripwire at a fast cadence —
	// far quicker than the heavy O(symbols²) fit, but not on every single tick.
	if now.Sub(signal.contagionAt) >= contagionCheckInterval {
		signal.cachedContagion = signal.contagion()
		signal.contagionAt = now
	}

	if !signal.throttle(now, signal.cachedContagion) {
		return nil
	}

	contagion := signal.cachedContagion
	entries := signal.snapshotEntries()
	macros := macroMedians(entries)

	tasks := make([]chan *qpool.QValue[any], 0, len(entries))

	for _, entry := range entries {
		tasks = append(tasks, signal.pool.ScheduleFast(signal.ctx, func(context.Context) (any, error) {
			measurement, standout, err := entry.state.Measure(macros[entry.symbol], contagion, now)

			if err != nil {
				return nil, fmt.Errorf("causal: measure %s: %w", entry.symbol, err)
			}

			if measurement.Source == types.SourceNone {
				return nil, nil
			}

			measurement.Symbol = entry.symbol

			telemetry, _ := numeric.ObserveGaugeTelemetry(
				signal.calibrator,
				signal.classifier,
				measurement.Strength,
				standout,
			)

			if err := types.AssignCategorySurpriseSNR(
				&measurement, signal.surpriseField, measurement.Category,
			); err != nil {
				return nil, fmt.Errorf("causal: snr %s: %w", entry.symbol, err)
			}

			if err := signal.rawDump.Write(rawRecord{
				Symbol:     measurement.Symbol,
				Category:   measurement.Category,
				Strength:   measurement.Strength,
				Confidence: measurement.Confidence,
				SNR:        measurement.SNR,
				Standout:   standout,
				Last:       measurement.Last,
				SpreadBPS:  measurement.SpreadBPS,
			}); err != nil {
				return nil, err
			}

			if err := measurement.Send(signal.pool); err != nil {
				return nil, err
			}

			if ui := signal.broadcasts["ui"]; ui != nil {
				ui.Send(&qpool.QValue[any]{
					Value: numeric.GaugePayload(
						measurement.Source.String(),
						measurement.Symbol,
						measurement.Category,
						measurement,
						telemetry,
					),
				})
			}

			return nil, nil
		}))
	}

	var err error

	for _, task := range tasks {
		value := <-task
		err = errors.Join(err, value.Error)
	}

	return err
}

func causalTickerTime(row market.TickerUpdate) (time.Time, error) {
	if row.Timestamp == "" {
		return time.Time{}, fmt.Errorf("causal: ticker timestamp is required for %s", row.Symbol)
	}

	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05.000000Z"} {
		if at, err := time.Parse(layout, row.Timestamp); err == nil {
			return at, nil
		}
	}

	return time.Time{}, fmt.Errorf("causal: ticker timestamp %s is invalid for %s", row.Timestamp, row.Symbol)
}

func causalBookTime(row market.Book) (time.Time, error) {
	if row.Timestamp == "" {
		return time.Time{}, fmt.Errorf("causal: book timestamp is required for %s", row.Symbol)
	}

	at, err := time.Parse(time.RFC3339Nano, row.Timestamp)

	if err != nil {
		return time.Time{}, fmt.Errorf("causal: book timestamp %s is invalid for %s: %w", row.Timestamp, row.Symbol, err)
	}

	return at, nil
}

func (signal *Signal) snapshotEntries() []publishEntry {
	entries := signal.publishScratch[:0]

	signal.symbols.Range(func(key, value any) bool {
		state := value.(*CausalSymbol)
		entries = append(entries, publishEntry{
			symbol: key.(string),
			state:  state,
			change: state.ChangePct(),
		})

		return true
	})

	signal.publishScratch = entries

	return entries
}

func macroMedians(entries []publishEntry) map[string]float64 {
	macros := make(map[string]float64, len(entries))
	changes := make([]float64, 0, len(entries))

	for _, entry := range entries {
		if entry.change != 0 {
			changes = append(changes, entry.change)
		}
	}

	for entryIndex, candidate := range entries {
		peerChanges := changes[:0]

		for peerIndex, peer := range entries {
			if peerIndex == entryIndex || peer.change == 0 {
				continue
			}

			peerChanges = append(peerChanges, peer.change)
		}

		if len(peerChanges) < 2 {
			continue
		}

		macros[candidate.symbol] = numeric.PercentileSorted(
			numeric.CopySorted(peerChanges),
			0.5,
		)
	}

	return macros
}

/*
macroMomentum returns the median change_pct across every symbol other than
candidate. The candidate's own change is excluded so it cannot appear on both
sides of the structural regression (outcome and macro regressor), which would
inject contemporaneous self-correlation into the backdoor estimand.
*/
func (signal *Signal) macroMomentum(candidate string) float64 {
	entries := signal.snapshotEntries()
	macros := macroMedians(entries)

	return macros[candidate]
}

func (signal *Signal) Close() error {
	signal.cancel()
	return signal.rawDump.Close()
}
