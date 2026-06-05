package leadlag

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/focus"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/numeric/adaptive"
	signalpool "github.com/theapemachine/symm/signal"
)

const (
	minLagFraction  = 0.35
	publishInterval = 200 * time.Millisecond
	rawSubscriberID = "signal/leadlag:raw"
)

/*
Signal detects altcoins lagging a moving anchor pair (BTC/EUR) and maps the
lead-lag structure onto the anchor perspective. It is cross-asset: each
follower's verdict is its lagged Hayashi-Yoshida correlation against the anchor.

Anchor movement is derived from the anchor ticker ring over the lag search
window and scored against the anchor's own adaptive move baseline — not Kraken's
24h change_pct. Measurements publish for every symbol in the universe; focus
applies only to UI chart streams elsewhere.

Confidence is classification clarity — margin to the lag-fraction or correlation
boundary; SNR is how surprising that clarity is versus the follower's own recent
baseline, not the lag correlation strength.

| Category           | Lag structure                              |
|:-------------------|:-------------------------------------------|
| Anchor Stall       | anchor not moving — no lead to follow      |
| Decoupled Move     | follower uncorrelated with the anchor      |
| Inefficient Lag    | follows the anchor with an exploitable lag |
| Synchronized Drift | moves with the anchor, no usable lag       |

The cross-correlation recompute is throttled (publishInterval); it is O(ring ×
maxLagBars) per follower and would otherwise saturate a core at ticker rate.
*/
type Signal struct {
	ctx             context.Context
	cancel          context.CancelFunc
	pool            *qpool.Q
	broadcasts      map[string]*qpool.BroadcastGroup
	subscribers     map[string]*qpool.Subscriber
	symbols         sync.Map
	anchorBaseline  moveBaseline
	followerScratch []string
	lastPublishMu   sync.Mutex
	lastPublish     time.Time
	floor           *adaptive.SNRField
}

func NewSignal(ctx context.Context, pool *qpool.Q) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		ctx:            ctx,
		cancel:         cancel,
		pool:           pool,
		broadcasts:     make(map[string]*qpool.BroadcastGroup),
		subscribers:    make(map[string]*qpool.Subscriber),
		anchorBaseline: *newMoveBaseline(),
		floor:          adaptive.NewSNRField(),
	}

	for _, channel := range []string{"raw"} {
		signal.broadcasts[channel] = pool.CreateBroadcastGroup(channel, 10*time.Millisecond)
		signal.subscribers[channel] = signal.broadcasts[channel].Subscribe(rawSubscriberID, 1024)
	}

	signal.broadcasts["measurements"] = pool.CreateBroadcastGroup("measurements", 10*time.Millisecond)
	signal.broadcasts["ui"] = pool.CreateBroadcastGroup("ui", 10*time.Millisecond)

	errnie.Info("signal/leadlag ready", "signal/leadlag")

	return signal
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
			case public.TickerChannel:
				tickers := signalpool.GetTickers(sm)

				ingested := false

				for _, ticker := range tickers {
					if ticker.Last <= 0 {
						continue
					}

					at, err := signal.timestamp(ticker)

					if err != nil {
						errnie.Error(err, "leadlag: timestamp %s", ticker.Symbol)
						continue
					}

					stored, _ := signal.symbols.LoadOrStore(ticker.Symbol, newSymbolState())
					stored.(*symbolState).observeTicker(ticker.Last, at)
					ingested = true
				}

				if !ingested {
					continue
				}

				if err := signal.publish(); err != nil {
					errnie.Error(err, "leadlag: publish")
				}
			}
		}
	}
}

// timestamp parses the ticker's wire timestamp.
func (signal *Signal) timestamp(row market.TickerUpdate) (time.Time, error) {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05.000000Z"} {
		if at, err := time.Parse(layout, row.Timestamp); err == nil {
			return at, nil
		}
	}

	return time.Time{}, fmt.Errorf("ticker timestamp is required for %s", row.Symbol)
}

// publish recomputes lead-lag against the anchor for every follower, throttled.
func (signal *Signal) publish() error {
	if !signal.throttle() {
		return nil
	}

	anchorName := focus.AnchorSymbol()
	anchorRaw, ok := signal.symbols.Load(anchorName)

	if !ok {
		return nil
	}

	anchor := anchorRaw.(*symbolState)
	move := signal.anchorMoveStatus(anchor)

	if !move.ready {
		return nil
	}

	if !move.moved {
		return signal.publishAnchorStall(anchorName, anchor, move.stallMargin)
	}

	return signal.publishFollowers(anchorName, anchor)
}

func (signal *Signal) publishFollowers(anchorName string, anchor *symbolState) error {
	followers := signal.followerScratch[:0]

	signal.symbols.Range(func(key, value any) bool {
		symbol := key.(string)

		if symbol == anchorName {
			return true
		}

		followers = append(followers, symbol)

		return true
	})

	signal.followerScratch = followers

	tasks := make([]chan *qpool.QValue[any], 0, len(followers))

	for _, symbolName := range followers {
		raw, ok := signal.symbols.Load(symbolName)

		if !ok {
			continue
		}

		follower := raw.(*symbolState)

		tasks = append(tasks, signal.pool.ScheduleFast(signal.ctx, func(context.Context) (any, error) {
			measurement, standout, err := signal.measureFollower(anchor, follower)

			if err != nil {
				return nil, fmt.Errorf("leadlag: measure %s: %w", symbolName, err)
			}

			if measurement.Source == perspectives.SourceNone {
				return nil, nil
			}

			measurement.Symbol = symbolName
			measurement.Last = follower.lastPrice()

			if err := signal.sendMeasurement(&measurement, standout); err != nil {
				return nil, fmt.Errorf("leadlag: snr %s: %w", symbolName, err)
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

func (signal *Signal) publishAnchorStall(
	anchorName string,
	anchor *symbolState,
	stallMargin float64,
) error {
	var joined error
	followers := signal.followerScratch[:0]

	signal.symbols.Range(func(key, value any) bool {
		symbol := key.(string)

		if symbol != anchorName {
			followers = append(followers, symbol)
		}

		return true
	})

	signal.followerScratch = followers
	measurement, standout, err := signal.measureStall(anchor, stallMargin)

	if err != nil {
		return fmt.Errorf("leadlag: measure anchor stall: %w", err)
	}

	measurement.Symbol = anchorName
	measurement.Last = anchor.lastPrice()

	joined = errors.Join(joined, signal.sendMeasurement(&measurement, standout))

	for _, symbolName := range followers {
		raw, ok := signal.symbols.Load(symbolName)

		if !ok {
			continue
		}

		follower := raw.(*symbolState)
		measurement, standout, err := signal.measureStall(follower, stallMargin)

		if err != nil {
			joined = errors.Join(joined, fmt.Errorf("leadlag: measure follower stall %s: %w", symbolName, err))
			continue
		}

		measurement.Symbol = symbolName
		measurement.Last = follower.lastPrice()
		joined = errors.Join(joined, signal.sendMeasurement(&measurement, standout))
	}

	return joined
}

func (signal *Signal) sendMeasurement(
	measurement *perspectives.Measurement,
	standout float64,
) error {
	if err := perspectives.AssignCategorySNR(
		measurement, signal.floor, standout,
	); err != nil {
		return err
	}

	signal.broadcasts["measurements"].Send(&qpool.QValue[any]{Value: *measurement})

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

// throttle reports whether enough time has passed to recompute; crossLag is
// expensive enough that running it per tick would saturate a core.
func (signal *Signal) throttle() bool {
	signal.lastPublishMu.Lock()
	defer signal.lastPublishMu.Unlock()

	if time.Since(signal.lastPublish) < publishInterval {
		return false
	}

	signal.lastPublish = time.Now()

	return true
}

func (signal *Signal) measureStall(
	symbol *symbolState,
	stallMargin float64,
) (perspectives.Measurement, float64, error) {
	category, clarity, standout := leadlagReading(false, stallMargin, 0, 0)

	confidence, err := symbol.tracked.Observe(category, clarity, standout)

	if err != nil {
		return perspectives.Measurement{}, 0, err
	}

	return perspectives.Measurement{
		Source:     perspectives.SourceLeadLag,
		Category:   category,
		Strength:   0,
		Confidence: confidence,
	}, standout, nil
}

// measureFollower classifies one follower's lead-lag relationship to the anchor.
func (signal *Signal) measureFollower(
	anchor *symbolState,
	state *symbolState,
) (perspectives.Measurement, float64, error) {
	if bars, corr, ok := state.crossLag(anchor); ok {
		category, clarity, standout := leadlagReading(true, 0, corr, bars)

		confidence, err := state.tracked.Observe(category, clarity, standout)

		if err != nil {
			return perspectives.Measurement{}, 0, err
		}

		return perspectives.Measurement{
			Source:     perspectives.SourceLeadLag,
			Category:   category,
			Strength:   corr / leadlagMinimumLagCorrelation,
			Confidence: confidence,
		}, standout, nil
	}

	corr, ok := state.contemporaneous(anchor)

	if !ok {
		return perspectives.Measurement{}, 0, nil
	}

	category, clarity, standout := leadlagReading(true, 0, corr, 0)

	confidence, err := state.tracked.Observe(category, clarity, standout)

	if err != nil {
		return perspectives.Measurement{}, 0, err
	}

	return perspectives.Measurement{
		Source:     perspectives.SourceLeadLag,
		Category:   category,
		Strength:   corr / leadlagMinimumLagCorrelation,
		Confidence: confidence,
	}, standout, nil
}

func (signal *Signal) Close() error {
	signal.cancel()
	return nil
}
