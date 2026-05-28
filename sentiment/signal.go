package sentiment

import (
	"context"
	"fmt"
	"iter"
	"sync"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/numeric/adaptive"
)

const (
	sentimentSource = "sentiment"
	minBreadth      = 0.55
)

/*
Sentiment measures cross-section bullish breadth from ticker change percentages.
*/
type Sentiment struct {
	ctx         context.Context
	cancel      context.CancelFunc
	pool        *qpool.Q
	broadcasts  map[string]*qpool.BroadcastGroup
	subscribers map[string]*qpool.Subscriber
	symbols     sync.Map
	peakMu      sync.Mutex
	peak        *adaptive.Peak
	pendingMu   sync.Mutex
	pendingSeen sync.Map
	pending     []string
	requested   sync.Map
}

func NewSentiment(ctx context.Context, pool *qpool.Q) *Sentiment {
	ctx, cancel := context.WithCancel(ctx)

	sentiment := &Sentiment{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		broadcasts:  make(map[string]*qpool.BroadcastGroup),
		subscribers: make(map[string]*qpool.Subscriber),
		symbols:     sync.Map{},
		peak:        adaptive.NewPeak(),
		requested:   sync.Map{},
	}

	for _, channel := range []string{"symbols", "tick", "feedback", "measurements", "subscriptions"} {
		sentiment.broadcasts[channel] = pool.CreateBroadcastGroup(channel, 10*time.Millisecond)
	}

	for _, channel := range []string{"symbols", "tick", "feedback"} {
		sentiment.subscribers[channel] = sentiment.broadcasts[channel].Subscribe("sentiment:"+channel, 128)
	}

	return sentiment
}

func (sentiment *Sentiment) Start() error        { return nil }
func (sentiment *Sentiment) State() engine.State { return engine.READY }

func (sentiment *Sentiment) Tick() error {
	errnie.Info("starting sentiment tick")

	var workers sync.WaitGroup
	errs := make(chan error, 1)
	fail := func(err error) {
		select {
		case errs <- err:
			sentiment.cancel()
		default:
		}
	}

	workers.Go(func() {
		for {
			select {
			case <-sentiment.ctx.Done():
				return
			case value, ok := <-sentiment.subscribers["symbols"].Incoming:
				if !ok {
					fail(fmt.Errorf("sentiment symbols channel closed"))
					return
				}

				pairs, pairsOK := value.Value.(map[string]*asset.Pair)
				if !pairsOK {
					errnie.Error(fmt.Errorf("sentiment: invalid symbols payload: %T", value.Value))
					continue
				}

				for symbol, pair := range pairs {
					if pair == nil {
						continue
					}

					sentiment.symbols.Store(symbol, newSymbolState(*pair))

					if pair.Quote != config.System.QuoteCurrency {
						continue
					}

					if _, seen := sentiment.requested.Load(symbol); seen {
						continue
					}

					sentiment.queuePending(symbol)
				}

				sentiment.publishPulse()
			}
		}
	})

	workers.Go(func() {
		for {
			select {
			case <-sentiment.ctx.Done():
				return
			case value, ok := <-sentiment.subscribers["tick"].Incoming:
				if !ok {
					fail(fmt.Errorf("sentiment tick channel closed"))
					return
				}

				row, rowOK := value.Value.(market.TickerRow)
				if !rowOK {
					errnie.Error(fmt.Errorf("sentiment: invalid ticker payload: %T", value.Value))
					continue
				}

				raw, ok := sentiment.symbols.Load(row.Symbol)

				if ok && row.ChangePct != 0 {
					state := raw.(*symbolState)
					state.observeTicker(row)

					sentiment.publishPulse()
				}

			}
		}
	})

	workers.Go(func() {
		for {
			select {
			case <-sentiment.ctx.Done():
				return
			case value, ok := <-sentiment.subscribers["feedback"].Incoming:
				if !ok {
					fail(fmt.Errorf("sentiment feedback channel closed"))
					return
				}

				fb, fbOK := value.Value.(engine.PredictionFeedback)
				if !fbOK {
					errnie.Error(fmt.Errorf("sentiment: invalid feedback payload: %T", value.Value))
					continue
				}

				sentiment.Feedback(fb)
				sentiment.publishPulse()
			}
		}
	})

	done := make(chan struct{})

	go func() {
		workers.Wait()
		close(done)
	}()

	select {
	case err := <-errs:
		workers.Wait()
		return errnie.Error(err)
	case <-sentiment.ctx.Done():
		workers.Wait()
		return sentiment.ctx.Err()
	case <-done:
		return sentiment.ctx.Err()
	}
}

func (sentiment *Sentiment) requestedCount() int {
	count := 0

	sentiment.requested.Range(func(key, value any) bool {
		count++
		return true
	})

	return count
}

func (sentiment *Sentiment) publishPulse() {
	tickerCount := 0

	sentiment.symbols.Range(func(key, value any) bool {
		state := value.(*symbolState)

		if state.snapshot().changePct != 0 {
			tickerCount++
		}

		return true
	})

	scanCap := max(config.System.MaxScanSymbols/8, 1)

	if symbols := sentiment.pendingBatch(scanCap, tickerCount); len(symbols) > 0 {
		for _, symbol := range symbols {
			sentiment.requested.Store(symbol, struct{}{})
		}

		sentiment.broadcasts["subscriptions"].Send(&qpool.QValue[any]{Value: symbols})
	}

	sentiment.publishMeasurements()
}

func (sentiment *Sentiment) queuePending(symbol string) {
	if symbol == "" {
		return
	}

	if _, queued := sentiment.pendingSeen.LoadOrStore(symbol, struct{}{}); queued {
		return
	}

	sentiment.pendingMu.Lock()
	defer sentiment.pendingMu.Unlock()

	sentiment.pending = append(sentiment.pending, symbol)
}

func (sentiment *Sentiment) pendingBatch(scanCap, tickerCount int) []string {
	requested := sentiment.requestedCount()

	if tickerCount >= 4 || requested >= scanCap {
		return nil
	}

	sentiment.pendingMu.Lock()
	defer sentiment.pendingMu.Unlock()

	if len(sentiment.pending) == 0 {
		return nil
	}

	remaining := scanCap - requested
	limit := min(min(config.System.SubscribeBatch, remaining), len(sentiment.pending))
	symbols := make([]string, 0, limit)
	pending := sentiment.pending[:0]

	for _, symbol := range sentiment.pending {
		if _, alreadyRequested := sentiment.requested.Load(symbol); alreadyRequested {
			continue
		}

		if len(symbols) >= limit {
			pending = append(pending, symbol)
			continue
		}
		symbols = append(symbols, symbol)
	}

	sentiment.pending = pending

	return symbols
}

func (sentiment *Sentiment) publishMeasurements() {
	breadth, leaders, topChange, ok := sentiment.breadthAndLeaders()

	if !ok {
		return
	}

	waiters := make([]chan *qpool.QValue[any], 0)

	sentiment.symbols.Range(func(key, value any) bool {
		symbol := key.(string)

		if _, subscribed := sentiment.requested.Load(symbol); !subscribed {
			return true
		}

		state := value.(*symbolState)

		snapshot := state.snapshot()

		if snapshot.changePct == 0 {
			return true
		}

		change := snapshot.changePct
		leaderSet := leaders

		if len(leaderSet) == 0 {
			leaderSet = map[string]float64{symbol: change}
		}

		waiters = append(
			waiters,
			sentiment.pool.ScheduleFast(sentiment.ctx, func(ctx context.Context) (any, error) {
				peakScore, err := sentiment.peakNext(
					change*breadth, leaderPeers(leaderSet, symbol)...,
				)

				if err != nil {
					return nil, err
				}

				confidence := state.calibratedConfidence(
					sentiment.sentimentConfidence(breadth, change, topChange, peakScore),
				)

				if confidence <= 0 {
					return nil, nil
				}

				return engine.Measurement{
					Type:       engine.Sentiment,
					Source:     sentimentSource,
					Regime:     "sentiment",
					Reason:     "breadth_leader",
					Pairs:      []asset.Pair{snapshot.pair},
					Confidence: confidence,
					Last:       snapshot.last,
					Bid:        snapshot.bid,
					Ask:        snapshot.ask,
				}, nil
			}),
		)

		return true
	})

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

		sentiment.broadcasts["measurements"].Send(&qpool.QValue[any]{
			Value: measurement,
		})
	}
}

func (sentiment *Sentiment) Close() error {
	sentiment.cancel()
	return nil
}

func (sentiment *Sentiment) Source() string { return sentimentSource }

func (sentiment *Sentiment) Measure() iter.Seq[engine.Measurement] {
	return func(yield func(engine.Measurement) bool) {
		breadth, leaders, topChange, ok := sentiment.breadthAndLeaders()

		if !ok {
			return
		}

		sentiment.symbols.Range(func(key, value any) bool {
			symbol := key.(string)
			state := value.(*symbolState)
			snapshot := state.snapshot()

			if snapshot.changePct == 0 {
				return true
			}

			change := snapshot.changePct
			leaderSet := leaders

			if len(leaderSet) == 0 {
				leaderSet = map[string]float64{symbol: change}
			}

			peakScore, err := sentiment.peakNext(
				change*breadth, leaderPeers(leaderSet, symbol)...,
			)

			if err != nil {
				errnie.Error(err)
				return true
			}

			confidence := state.calibratedConfidence(
				sentiment.sentimentConfidence(breadth, change, topChange, peakScore),
			)

			if confidence <= 0 {
				return true
			}

			if !yield(engine.Measurement{
				Type:       engine.Sentiment,
				Source:     sentimentSource,
				Regime:     "sentiment",
				Reason:     "breadth_leader",
				Pairs:      []asset.Pair{snapshot.pair},
				Confidence: confidence,
				Last:       snapshot.last,
				Bid:        snapshot.bid,
				Ask:        snapshot.ask,
			}) {
				return false
			}

			return true
		})
	}
}

func (sentiment *Sentiment) peakNext(score float64, peers ...float64) (float64, error) {
	sentiment.peakMu.Lock()
	defer sentiment.peakMu.Unlock()

	return sentiment.peak.Next(score, peers...)
}

func (sentiment *Sentiment) Feedback(feedback engine.PredictionFeedback) {
	if !engine.FeedbackIncludesSource(feedback, sentimentSource) || feedback.Symbol == "" || feedback.PredictedReturn <= 0 {
		return
	}

	raw, ok := sentiment.symbols.Load(feedback.Symbol)

	if !ok {
		return
	}

	state := raw.(*symbolState)

	if err := state.applyFeedback(feedback.PredictedReturn, feedback.ActualReturn); err != nil {
		errnie.Error(err)
	}
}
