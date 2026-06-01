package leadlag

import (
	"context"
	"sync"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/numeric/adaptive"
)

const (
	minAnchorMove   = 0.05
	minLagFraction  = 0.35
	publishInterval = 200 * time.Millisecond
)

func resolvedSymbols() []string {
	symbols := viper.GetStringSlice("market.symbols")

	if len(symbols) > 0 {
		return symbols
	}

	defaults := viper.GetStringSlice("market.default_symbols")

	if len(defaults) > 0 {
		return defaults
	}

	return []string{"BTC/EUR"}
}

func anchorSymbol() string {
	return resolvedSymbols()[0]
}

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

	for _, channel := range []string{"ticker", "measurements"} {
		signal.broadcasts[channel] = pool.CreateBroadcastGroup(channel, 10*time.Millisecond)
		signal.subscribers[channel] = signal.broadcasts[channel].Subscribe(channel, 128)
	}

	return signal
}

func (signal *Signal) Tick() error {
	for message := range signal.subscribers["ticker"].Incoming {
		if message == nil || message.Value == nil {
			continue
		}

		envelope, ok := message.Value.(public.SocketMessage)

		if !ok {
			continue
		}

		rows, err := envelope.SplitDataRows()

		if err != nil {
			errnie.Error(err)

			continue
		}

		for _, row := range rows {
			ticker, err := market.DecodeTicker(row)

			if err != nil {
				errnie.Error(err)

				continue
			}

			if ticker.Last <= 0 {
				continue
			}

			stored, _ := signal.symbols.LoadOrStore(ticker.Symbol, newSymbolState())
			stored.(*symbolState).observeTicker(ticker.ChangePct, ticker.Last, signal.timestamp(ticker))

			signal.publish()
		}
	}

	return signal.ctx.Err()
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
func (signal *Signal) publish() {
	if !signal.throttle() {
		return
	}

	anchorRaw, ok := signal.symbols.Load(anchorSymbol())

	if !ok {
		return
	}

	anchor := anchorRaw.(*symbolState)
	anchorMoved := anchor.change() >= minAnchorMove

	signal.symbols.Range(func(key, value any) bool {
		if key.(string) == anchorSymbol() {
			return true
		}

		follower := value.(*symbolState)
		measurement, ok := signal.measure(anchor, anchorMoved, follower)

		if ok {
			measurement.Symbol = key.(string)
			measurement.Last = follower.lastPrice()
			measurement.SNR = signal.floor.Score(measurement.Symbol, measurement.Confidence)
			signal.broadcasts["measurements"].Send(&qpool.QValue[any]{Value: measurement})
		}

		return true
	})
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
) (perspectives.Measurement, bool) {
	if !anchorMoved {
		category, evidence := leadlagReading(false, anchor.change(), 0, 0)

		confidence, err := state.tracked.Observe(category, evidence)

		if err != nil {
			return perspectives.Measurement{}, false
		}

		return perspectives.Measurement{
			Source:     perspectives.SourceLeadLag,
			Category:   category,
			Strength:   0,
			Confidence: confidence,
		}, true
	}

	if bars, corr, ok := state.crossLag(anchor); ok {
		category, evidence := leadlagReading(true, anchor.change(), corr, bars)

		confidence, err := state.tracked.Observe(category, evidence)

		if err != nil {
			return perspectives.Measurement{}, false
		}

		return perspectives.Measurement{
			Source:     perspectives.SourceLeadLag,
			Category:   category,
			Strength:   corr / leadlagMinimumLagCorrelation,
			Confidence: confidence,
		}, true
	}

	corr, ok := state.contemporaneous(anchor)

	if !ok {
		return perspectives.Measurement{}, false
	}

	category, evidence := leadlagReading(true, anchor.change(), corr, 0)

	confidence, err := state.tracked.Observe(category, evidence)

	if err != nil {
		return perspectives.Measurement{}, false
	}

	return perspectives.Measurement{
		Source:     perspectives.SourceLeadLag,
		Category:   category,
		Strength:   corr / leadlagMinimumLagCorrelation,
		Confidence: confidence,
	}, true
}

func (signal *Signal) Close() error {
	signal.cancel()
	return nil
}
