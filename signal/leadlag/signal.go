package leadlag

import (
	"context"
	"sync"
	"time"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/market/perspectives"
)

const (
	anchorSymbol    = "BTC/EUR"
	minAnchorMove   = 0.05
	minLagFraction  = 0.35
	publishInterval = 200 * time.Millisecond
)

/*
Signal detects altcoins lagging a moving anchor pair (BTC/EUR) and maps the
lead-lag structure onto the anchor perspective. It is cross-asset: each
follower's verdict is its lagged Hayashi-Yoshida correlation against the anchor.

SNR is the lag correlation relative to the minimum that counts as a lead, so a
strong, exploitable lag clears the noise floor and incidental co-movement does not.

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
}

func NewSignal(ctx context.Context, pool *qpool.Q) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		broadcasts:  make(map[string]*qpool.BroadcastGroup),
		subscribers: make(map[string]*qpool.Subscriber),
	}

	signal.broadcasts["measurements"] = pool.CreateBroadcastGroup(
		"measurements", 10*time.Millisecond,
	)

	return signal
}

func (signal *Signal) Tick() error {
	for row := range market.NewTickerSubscription(signal.ctx, config.System.Symbols...) {
		if row == nil || row.Last <= 0 {
			continue
		}

		stored, _ := signal.symbols.LoadOrStore(row.Symbol, newSymbolState())
		stored.(*symbolState).observeTicker(row.ChangePct, row.Last, signal.timestamp(*row))

		signal.publish()
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

	anchorRaw, ok := signal.symbols.Load(anchorSymbol)

	if !ok {
		return
	}

	anchor := anchorRaw.(*symbolState)
	anchorMoved := anchor.change() >= minAnchorMove

	signal.symbols.Range(func(key, value any) bool {
		if key.(string) == anchorSymbol {
			return true
		}

		follower := value.(*symbolState)
		measurement, ok := signal.measure(anchor, anchorMoved, follower)

		if ok {
			measurement.Symbol = key.(string)
			measurement.Last = follower.lastPrice()
			measurement = perspectives.FinalizeSNR(
				measurement,
				measurement.SNR,
				follower.floor.Score,
			)
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
		return perspectives.Measurement{
			Source:   perspectives.SourceLeadLag,
			Category: perspectives.CategoryAnchorStall,
			SNR:      0,
		}, true
	}

	if bars, corr, ok := state.crossLag(anchor); ok {
		category := perspectives.CategorySynchronizedDrift

		if float64(bars)/float64(maxLagBars) >= minLagFraction {
			category = perspectives.CategoryInefficientLag
		}

		return perspectives.Measurement{
			Source:   perspectives.SourceLeadLag,
			Category: category,
			SNR:      corr / leadlagMinimumLagCorrelation,
		}, true
	}

	corr, ok := state.contemporaneous(anchor)

	if !ok {
		return perspectives.Measurement{}, false
	}

	category := perspectives.CategorySynchronizedDrift

	if corr < leadlagMinimumLagCorrelation {
		category = perspectives.CategoryDecoupledMove
	}

	return perspectives.Measurement{
		Source:   perspectives.SourceLeadLag,
		Category: category,
		SNR:      corr / leadlagMinimumLagCorrelation,
	}, true
}

func (signal *Signal) Close() error {
	signal.cancel()
	return nil
}
