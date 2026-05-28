package leadlag

import (
	"context"
	"fmt"
	"iter"
	"math"
	"sync"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/numeric/adaptive"
	"github.com/theapemachine/symm/runstats"
)

const (
	leadlagSource  = "leadlag"
	anchorSymbol   = "BTC/EUR"
	minAnchorMove  = 0.05
	minLagFraction = 0.35
)

// publishInterval throttles leadlag's cross-correlation recompute. The
// per-symbol crossLag work is O(ringSize × maxLagBars) — orders of magnitude
// more expensive than the old changePct gap calc that ran per ticker. With
// ~64 symbols this would saturate a core if it ran on every tick.
// 200ms is well below the runway of any signal the trader acts on so
// lead-lag freshness is unaffected; what changes is that we don't burn
// CPU recomputing a 256-bar Pearson three times per second.
const publishInterval = 200 * time.Millisecond

/*
LeadLag detects altcoins lagging a moving anchor pair.
*/
type LeadLag struct {
	ctx           context.Context
	cancel        context.CancelFunc
	pool          *qpool.Q
	broadcasts    map[string]*qpool.BroadcastGroup
	subscribers   map[string]*qpool.Subscriber
	symbols       sync.Map
	peakMu        sync.Mutex
	peak          *adaptive.Peak
	pending       []string
	requested     sync.Map
	lastPublishMu sync.Mutex
	lastPublish   time.Time
}

func NewLeadLag(ctx context.Context, pool *qpool.Q) *LeadLag {
	ctx, cancel := context.WithCancel(ctx)

	leadlag := &LeadLag{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		broadcasts:  make(map[string]*qpool.BroadcastGroup),
		subscribers: make(map[string]*qpool.Subscriber),
		symbols:     sync.Map{},
		peak:        adaptive.NewPeak(),
		requested:   sync.Map{},
	}

	for _, channel := range []string{"symbols", "tick", "feedback"} {
		group := pool.CreateBroadcastGroup(channel, 10*time.Millisecond)
		leadlag.subscribers[channel] = group.Subscribe("leadlag:"+channel, 128)
	}

	leadlag.broadcasts["measurements"] = pool.CreateBroadcastGroup("measurements", 10*time.Millisecond)
	leadlag.broadcasts["subscriptions"] = pool.CreateBroadcastGroup("subscriptions", 10*time.Millisecond)

	return leadlag
}

func (leadlag *LeadLag) Start() error        { return nil }
func (leadlag *LeadLag) State() engine.State { return engine.READY }

func (leadlag *LeadLag) Tick() error {
	errnie.Info("starting leadlag tick")

	var wg sync.WaitGroup

	wg.Go(func() {
		for {
			select {
			case <-leadlag.ctx.Done():
				return
			case value, ok := <-leadlag.subscribers["symbols"].Incoming:
				if !ok {
					errnie.Error(fmt.Errorf("leadlag symbols channel closed"))
					return
				}

				pairs, pairsOK := value.Value.(map[string]*asset.Pair)
				if !pairsOK {
					errnie.Error(fmt.Errorf("leadlag: invalid symbols payload: %T", value.Value))
					continue
				}

				for symbol, pair := range pairs {
					if pair == nil {
						continue
					}

					leadlag.symbols.Store(symbol, newSymbolState(*pair))

					if pair.Quote != config.System.QuoteCurrency {
						continue
					}

					if _, seen := leadlag.requested.Load(symbol); seen {
						continue
					}

					if symbol == anchorSymbol {
						leadlag.requested.Store(symbol, struct{}{})
						leadlag.broadcasts["subscriptions"].Send(&qpool.QValue[any]{Value: []string{symbol}})
						continue
					}

					leadlag.pending = append(leadlag.pending, symbol)
				}

				leadlag.publishPulse()
			}
		}
	})

	wg.Go(func() {
		for {
			select {
			case <-leadlag.ctx.Done():
				return
			case value, ok := <-leadlag.subscribers["tick"].Incoming:
				if !ok {
					errnie.Error(fmt.Errorf("leadlag tick channel closed"))
					return
				}

				row, rowOK := value.Value.(market.TickerRow)
				if !rowOK {
					errnie.Error(fmt.Errorf("leadlag: invalid ticker payload: %T", value.Value))
					continue
				}

				raw, ok := leadlag.symbols.Load(row.Symbol)

				if ok && row.Last > 0 {
					state := raw.(*symbolState)
					// Push every ticker observation into the price ring so
					// the cross-correlation has a real time-series to lag
					// against. Without this the ring stays empty and
					// crossLag returns (0, 0, false) for every symbol.
					eventTime := parseLeadlagTimestamp(row.Timestamp)

					if eventTime.IsZero() {
						eventTime = time.Now()
					}

					state.observeTicker(
						row.ChangePct,
						row.Last,
						row.Bid,
						row.Ask,
						eventTime,
					)

					leadlag.publishPulse()
				}

			}
		}
	})

	wg.Go(func() {
		for {
			select {
			case <-leadlag.ctx.Done():
				return
			case value, ok := <-leadlag.subscribers["feedback"].Incoming:
				if !ok {
					errnie.Error(fmt.Errorf("leadlag feedback channel closed"))
					return
				}

				fb, fbOK := value.Value.(engine.PredictionFeedback)
				if !fbOK {
					errnie.Error(fmt.Errorf("leadlag: invalid feedback payload: %T", value.Value))
					continue
				}

				leadlag.Feedback(fb)
				leadlag.publishPulse()
			}
		}
	})

	wg.Wait()
	return leadlag.ctx.Err()
}

func (leadlag *LeadLag) requestedCount() int {
	count := 0

	leadlag.requested.Range(func(key, value any) bool {
		count++
		return true
	})

	return count
}

func (leadlag *LeadLag) publishPulse() {
	anchorRaw, ok := leadlag.symbols.Load(anchorSymbol)

	if ok {
		anchor := anchorRaw.(*symbolState)

		if anchor.change() >= minAnchorMove {
			scanCap := max(config.System.MaxScanSymbols/8, 1)
			requested := leadlag.requestedCount()

			if len(leadlag.pending) > 0 && requested < scanCap {
				remaining := scanCap - requested
				batch := min(min(config.System.SubscribeBatch, remaining), len(leadlag.pending))

				symbols := leadlag.pending[:batch]
				leadlag.pending = leadlag.pending[batch:]

				for _, symbol := range symbols {
					leadlag.requested.Store(symbol, struct{}{})
				}

				leadlag.broadcasts["subscriptions"].Send(&qpool.QValue[any]{Value: symbols})
			}
		}
	}

	leadlag.publishMeasurements()
}

func (leadlag *LeadLag) publishMeasurements() {
	// Skip if we've published within the throttle window. crossLag is
	// expensive enough that running it per tick saturates a core; the
	// throttle bounds the worst-case rate to 5Hz regardless of incoming
	// ticker volume.
	leadlag.lastPublishMu.Lock()
	if time.Since(leadlag.lastPublish) < publishInterval {
		leadlag.lastPublishMu.Unlock()
		runstats.LeadlagThrottle()
		return
	}

	leadlag.lastPublish = time.Now()
	leadlag.lastPublishMu.Unlock()
	runstats.LeadlagRecompute()

	anchorRaw, ok := leadlag.symbols.Load(anchorSymbol)

	if !ok {
		return
	}

	anchor := anchorRaw.(*symbolState)
	scores := leadlag.lagScores(anchor)
	waiters := make([]chan *qpool.QValue[any], 0, len(scores))

	for symbol, score := range scores {
		if _, subscribed := leadlag.requested.Load(symbol); !subscribed {
			continue
		}

		raw, loaded := leadlag.symbols.Load(symbol)

		if !loaded {
			continue
		}

		state := raw.(*symbolState)
		score := score
		waiters = append(
			waiters,
			leadlag.pool.ScheduleFast(leadlag.ctx, func(ctx context.Context) (any, error) {
				peerScores := adaptive.PeerValues(toRatioMap(scores), symbol)
				leadlag.peakMu.Lock()
				peakLag, err := leadlag.peak.Next(score.correlation, peerScores...)
				leadlag.peakMu.Unlock()

				if err != nil {
					return nil, err
				}

				measurement, ok := lagMeasurement(anchor, state, peakLag, score.correlation)

				if !ok {
					return nil, nil
				}

				measurement.Reason = score.reason
				return measurement, nil
			}),
		)
	}

	for _, waiter := range waiters {
		value := <-waiter

		if value == nil {
			continue
		}

		if value.Error != nil {
			errnie.Error(value.Error)
			continue
		}

		measurement, ok := value.Value.(engine.Measurement)

		if !ok {
			continue
		}

		leadlag.broadcasts["measurements"].Send(&qpool.QValue[any]{
			Value: measurement,
		})
	}
}

func (leadlag *LeadLag) Close() error {
	leadlag.cancel()
	return nil
}

func (leadlag *LeadLag) Source() string { return leadlagSource }

func (leadlag *LeadLag) Measure() iter.Seq[engine.Measurement] {
	return func(yield func(engine.Measurement) bool) {
		anchorRaw, ok := leadlag.symbols.Load(anchorSymbol)

		if !ok {
			return
		}

		anchor := anchorRaw.(*symbolState)
		scores := leadlag.lagScores(anchor)
		peerMap := toRatioMap(scores)

		for symbol, score := range scores {
			leadlag.peakMu.Lock()
			peakLag, err := leadlag.peak.Next(score.correlation, adaptive.PeerValues(peerMap, symbol)...)
			leadlag.peakMu.Unlock()

			if err != nil {
				errnie.Error(err)
				continue
			}

			raw, loaded := leadlag.symbols.Load(symbol)

			if !loaded {
				continue
			}

			state := raw.(*symbolState)
			measurement, ok := lagMeasurement(anchor, state, peakLag, score.correlation)

			if !ok {
				continue
			}

			measurement.Reason = score.reason

			if !yield(measurement) {
				return
			}
		}
	}
}

func (leadlag *LeadLag) Feedback(feedback engine.PredictionFeedback) {
	if !engine.FeedbackIncludesSource(feedback, leadlagSource) || feedback.Symbol == "" || feedback.PredictedReturn <= 0 {
		return
	}

	raw, ok := leadlag.symbols.Load(feedback.Symbol)

	if !ok {
		return
	}

	state := raw.(*symbolState)

	if err := state.applyFeedback(feedback.PredictedReturn, feedback.ActualReturn); err != nil {
		errnie.Error(err)
	}
}

/*
lagScore packages the output of crossLag plus a reason string the
measurement can carry forward. It replaces the prior dispersion-based
lagRatios, which computed (anchor.changePct - state.changePct) at the
same instant — a cross-section spread, not a lag.
*/
type lagScore struct {
	bars        int
	correlation float64
	reason      string
}

func (leadlag *LeadLag) lagScores(anchor *symbolState) map[string]lagScore {
	scores := make(map[string]lagScore)

	leadlag.symbols.Range(func(key, value any) bool {
		symbol := key.(string)

		if symbol == anchorSymbol {
			return true
		}

		state := value.(*symbolState)
		bars, corr, ok := crossLag(anchor, state)

		if !ok || bars <= 0 || corr <= 0 {
			return true
		}

		scores[symbol] = lagScore{
			bars:        bars,
			correlation: corr,
			reason:      "leadlag_follower",
		}

		return true
	})

	return scores
}

func toRatioMap(scores map[string]lagScore) map[string]float64 {
	ratios := make(map[string]float64, len(scores))

	for symbol, score := range scores {
		ratios[symbol] = score.correlation
	}

	return ratios
}

func parseLeadlagTimestamp(value string) time.Time {
	if value == "" {
		return time.Time{}
	}

	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05.000000Z"} {
		if t, err := time.Parse(layout, value); err == nil {
			return t
		}
	}

	return time.Time{}
}

func lagMeasurement(
	anchor *symbolState,
	state *symbolState,
	peakLag float64,
	lagRatio float64,
) (engine.Measurement, bool) {
	anchorSnapshot := anchor.snapshot()
	stateSnapshot := state.snapshot()
	anchorStrength := engine.ExcessRatio(anchorSnapshot.changePct / minAnchorMove)
	scale := stateSnapshot.scale
	score := peakLag * scale

	if score <= 0 {
		score = lagRatio * scale
	}

	confidence := engine.AlignConfidence(score, anchorStrength)

	if confidence <= 0 {
		confidence = engine.ProvisionalConfidence(
			0,
			math.Abs(anchorSnapshot.changePct-stateSnapshot.changePct)*scale,
		)
	}

	if confidence <= 0 && stateSnapshot.changePct != 0 {
		confidence = engine.ConfidenceFromScore(math.Abs(stateSnapshot.changePct) * scale)
	}

	if confidence <= 0 {
		return engine.Measurement{}, false
	}

	return engine.Measurement{
		Type:       engine.LeadLag,
		Source:     leadlagSource,
		Regime:     "cross_asset",
		Reason:     "anchor_lag",
		Pairs:      []asset.Pair{stateSnapshot.pair},
		Confidence: confidence,
		Last:       stateSnapshot.last,
		Bid:        stateSnapshot.bid,
		Ask:        stateSnapshot.ask,
	}, true
}
