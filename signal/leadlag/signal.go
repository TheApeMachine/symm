package leadlag

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/activate"
	"github.com/theapemachine/symm/focus"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/numeric/adaptive"
)

const (
	minAnchorMove   = 0.05
	minLagFraction  = 0.35
	publishInterval = 200 * time.Millisecond
	rawSubscriberID = "signal/leadlag:raw"
)

/*
Signal detects altcoins lagging a moving anchor pair (BTC/EUR) and maps the
lead-lag structure onto the anchor perspective. It is cross-asset: each
follower's verdict is its lagged Hayashi-Yoshida correlation against the anchor.

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
	ctx           context.Context
	cancel        context.CancelFunc
	pool          *qpool.Q
	broadcasts    map[string]*qpool.BroadcastGroup
	subscribers   map[string]*qpool.Subscriber
	symbols       sync.Map
	lastPublishMu sync.Mutex
	lastPublish   time.Time
	floor         *adaptive.SNRField
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

	activate.Boot("signal/leadlag ready")

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

			envelope, ok := message.Value.(public.SocketMessage)

			if !ok {
				continue
			}

			switch envelope.Channel {
			case public.TickerChannel:
				tickers, err := market.DecodeTickers(&envelope)

				if err != nil {
					return fmt.Errorf("leadlag: decode tickers: %w", err)
				}

				for _, ticker := range tickers {
					if ticker.Last <= 0 {
						continue
					}

					stored, _ := signal.symbols.LoadOrStore(ticker.Symbol, newSymbolState())
					stored.(*symbolState).observeTicker(ticker.ChangePct, ticker.Last, signal.timestamp(ticker))

					if err := signal.publish(); err != nil {
						return fmt.Errorf("leadlag: publish: %w", err)
					}
				}
			}
		}
	}
}

// timestamp parses the ticker's wire timestamp, falling back to now.
func (signal *Signal) timestamp(row market.TickerUpdate) time.Time {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05.000000Z"} {
		if at, err := time.Parse(layout, row.Timestamp); err == nil {
			return at
		}
	}

	return time.Now()
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
	anchorMoved := anchor.change() >= minAnchorMove

	var publishErr error

	signal.symbols.Range(func(key, value any) bool {
		if key.(string) == anchorName {
			return true
		}

		follower := value.(*symbolState)
		measurement, standout, err := signal.measure(anchor, anchorMoved, follower)

		if err != nil {
			publishErr = fmt.Errorf("leadlag: measure %s: %w", key.(string), err)
			return false
		}

		if measurement.Source == perspectives.SourceNone {
			return true
		}

		measurement.Symbol = key.(string)
		measurement.Last = follower.lastPrice()
		if err := perspectives.AssignCategorySNR(
			&measurement, signal.floor, standout,
		); err != nil {
			publishErr = fmt.Errorf("leadlag: snr %s: %w", key.(string), err)
			return false
		}

		activate.Once("signal/leadlag:measurement")
		signal.broadcasts["measurements"].Send(&qpool.QValue[any]{Value: measurement})

		return true
	})

	return publishErr
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

// measure classifies one follower's lead-lag relationship to the anchor.
func (signal *Signal) measure(
	anchor *symbolState,
	anchorMoved bool,
	state *symbolState,
) (perspectives.Measurement, float64, error) {
	if !anchorMoved {
		category, evidence := leadlagReading(false, anchor.change(), 0, 0)
		standout := evidence

		confidence, err := state.tracked.Observe(category, evidence, standout)

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

	if bars, corr, ok := state.crossLag(anchor); ok {
		category, evidence := leadlagReading(true, anchor.change(), corr, bars)
		standout := evidence

		confidence, err := state.tracked.Observe(category, evidence, standout)

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

	category, evidence := leadlagReading(true, anchor.change(), corr, 0)
	standout := evidence

	confidence, err := state.tracked.Observe(category, evidence, standout)

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
