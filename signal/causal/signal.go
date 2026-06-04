package causal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/numeric"
	"github.com/theapemachine/symm/numeric/adaptive"
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
	fitContagion     float64
	emergencyBackoff time.Duration
	backoffUntil     time.Time
	publishScratch   []publishEntry
	floor            *adaptive.SNRField
}

type publishEntry struct {
	symbol string
	state  *CausalSymbol
	change float64
}

func NewSignal(ctx context.Context, pool *qpool.Q) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		broadcasts:  make(map[string]*qpool.BroadcastGroup),
		subscribers: make(map[string]*qpool.Subscriber),
		floor:       adaptive.NewSNRField(),
	}

	for _, channel := range []string{"raw"} {
		signal.broadcasts[channel] = pool.CreateBroadcastGroup(channel, 10*time.Millisecond)
		signal.subscribers[channel] = signal.broadcasts[channel].Subscribe(rawSubscriberID, 1024)
	}

	signal.broadcasts["measurements"] = pool.CreateBroadcastGroup("measurements", 10*time.Millisecond)
	signal.broadcasts["ui"] = pool.CreateBroadcastGroup("ui", 10*time.Millisecond)

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
					if err := signal.state(trade.Symbol).FeedTrade(trade); err != nil {
						errnie.Error(err, "causal: feed trade %s", trade.Symbol)
						continue
					}
				}

				if err := signal.publish(); err != nil {
					errnie.Error(err, "causal: publish")
					continue
				}
			case "tickers":
				tickers := signalpool.GetTickers(sm)

				for _, ticker := range tickers {
					signal.state(ticker.Symbol).FeedTicker(ticker)
				}

				if err := signal.publish(); err != nil {
					errnie.Error(err, "causal: publish")
					continue
				}
			case "books":
				books := signalpool.GetBooks(sm)

				for _, delta := range books {
					signal.state(delta.Symbol).FeedBook(delta)
				}

				if err := signal.publish(); err != nil {
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
	if now.Sub(signal.lastPublish) >= causalPublishInterval {
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
	now := time.Now()

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

			if measurement.Source == perspectives.SourceNone {
				return nil, nil
			}

			measurement.Symbol = entry.symbol

			if err := perspectives.AssignCategorySNR(
				&measurement, signal.floor, standout,
			); err != nil {
				return nil, fmt.Errorf("causal: snr %s: %w", entry.symbol, err)
			}

			signal.broadcasts["measurements"].Send(&qpool.QValue[any]{Value: measurement})

			if ui := signal.broadcasts["ui"]; ui != nil {
				ui.Send(&qpool.QValue[any]{
					Value: map[string]any{
						"chart":      "gauge",
						"source":     measurement.Source.String(),
						"confidence": measurement.Confidence,
						"snr":        measurement.SNR},
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
	return nil
}
