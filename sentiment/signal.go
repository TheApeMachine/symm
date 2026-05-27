package sentiment

import (
	"context"
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

type symbolState struct {
	pair      asset.Pair
	changePct float64
	last      float64
	bid       float64
	ask       float64
}

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
	peak        *adaptive.Peak
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
		sentiment.subscribers[channel] = sentiment.broadcasts[channel].Subscribe("sentiment:"+channel, 128)
	}

	return sentiment
}

func (sentiment *Sentiment) Start() error        { return nil }
func (sentiment *Sentiment) State() engine.State { return engine.READY }

func (sentiment *Sentiment) Tick() error {
	select {
	case <-sentiment.ctx.Done():
		return sentiment.ctx.Err()
	case value := <-sentiment.subscribers["symbols"].Incoming:
		for symbol, pair := range value.Value.(map[string]*asset.Pair) {
			if pair == nil {
				continue
			}

			sentiment.symbols.Store(symbol, &symbolState{pair: *pair})

			if pair.Quote != config.System.QuoteCurrency {
				continue
			}

			if _, seen := sentiment.requested.Load(symbol); seen {
				continue
			}

			sentiment.pending = append(sentiment.pending, symbol)
		}

		sentiment.publishPulse()
	case value := <-sentiment.subscribers["tick"].Incoming:
		row := value.Value.(market.TickerRow)
		raw, ok := sentiment.symbols.Load(row.Symbol)

		if !ok || row.ChangePct == 0 {
			break
		}

		state := raw.(*symbolState)
		state.changePct = row.ChangePct

		if row.Last > 0 {
			state.last = row.Last
		}

		if row.Bid > 0 {
			state.bid = row.Bid
		}

		if row.Ask > 0 {
			state.ask = row.Ask
		}

		sentiment.publishPulse()
	case value := <-sentiment.subscribers["feedback"].Incoming:
		sentiment.Feedback(value.Value.(engine.PredictionFeedback))
		sentiment.publishPulse()
	default:
	}

	return nil
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

		if state.changePct != 0 {
			tickerCount++
		}

		return true
	})

	scanCap := max(config.System.MaxScanSymbols/8, 1)
	requested := sentiment.requestedCount()

	if len(sentiment.pending) > 0 && tickerCount < 4 && requested < scanCap {
		remaining := scanCap - requested
		batch := min(min(config.System.SubscribeBatch, remaining), len(sentiment.pending))

		symbols := sentiment.pending[:batch]
		sentiment.pending = sentiment.pending[batch:]

		for _, symbol := range symbols {
			sentiment.requested.Store(symbol, struct{}{})
		}

		sentiment.broadcasts["subscriptions"].Send(&qpool.QValue[any]{Value: symbols})
	}

	sentiment.publishMeasurements()
}

func (sentiment *Sentiment) breadthAndLeaders() (float64, map[string]float64, float64, bool) {
	positive := 0
	total := 0
	leaders := make(map[string]float64)
	topChange := 0.0

	sentiment.symbols.Range(func(key, value any) bool {
		state := value.(*symbolState)

		if state.changePct == 0 {
			return true
		}

		total++

		if state.changePct <= 0 {
			return true
		}

		positive++
		leaders[key.(string)] = state.changePct

		if state.changePct > topChange {
			topChange = state.changePct
		}

		return true
	})

	if total < 4 {
		return 0, nil, 0, false
	}

	breadth := float64(positive) / float64(total)

	if breadth < minBreadth || topChange <= 0 {
		return 0, nil, 0, false
	}

	return breadth, leaders, topChange, true
}

func (sentiment *Sentiment) publishMeasurements() {
	breadth, leaders, topChange, ok := sentiment.breadthAndLeaders()

	if !ok {
		return
	}

	waiters := make([]chan *qpool.QValue[any], 0, len(leaders))

	for symbol, change := range leaders {
		waiters = append(
			waiters,
			sentiment.pool.ScheduleFast(sentiment.ctx, func(ctx context.Context) (any, error) {
				peakScore, err := sentiment.peak.Next(change*breadth, leaderPeers(leaders, symbol)...)

				if err != nil {
					return nil, err
				}

				if peakScore <= 0 {
					return nil, nil
				}

				raw, loaded := sentiment.symbols.Load(symbol)

				if !loaded {
					return nil, nil
				}

				state := raw.(*symbolState)
				confidence := engine.AlignConfidence(breadth, change/topChange)

				if confidence <= 0 {
					return nil, nil
				}

				return engine.Measurement{
					Type:       engine.Sentiment,
					Source:     sentimentSource,
					Regime:     "sentiment",
					Reason:     "breadth_leader",
					Pairs:      []asset.Pair{state.pair},
					Confidence: confidence,
					Last:       state.last,
					Bid:        state.bid,
					Ask:        state.ask,
				}, nil
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

		for symbol, change := range leaders {
			peakScore, err := sentiment.peak.Next(change*breadth, leaderPeers(leaders, symbol)...)

			if err != nil {
				errnie.Error(err)
				continue
			}

			if peakScore <= 0 {
				continue
			}

			raw, loaded := sentiment.symbols.Load(symbol)

			if !loaded {
				continue
			}

			state := raw.(*symbolState)
			confidence := engine.AlignConfidence(breadth, change/topChange)

			if confidence <= 0 {
				continue
			}

			if !yield(engine.Measurement{
				Type:       engine.Sentiment,
				Source:     sentimentSource,
				Regime:     "sentiment",
				Reason:     "breadth_leader",
				Pairs:      []asset.Pair{state.pair},
				Confidence: confidence,
				Last:       state.last,
				Bid:        state.bid,
				Ask:        state.ask,
			}) {
				return
			}
		}
	}
}

func (sentiment *Sentiment) Feedback(feedback engine.PredictionFeedback) {
	if feedback.Source != sentimentSource {
		return
	}
}

func leaderPeers(leaders map[string]float64, skip string) []float64 {
	peers := make([]float64, 0, len(leaders)-1)

	for symbol, value := range leaders {
		if symbol == skip {
			continue
		}

		peers = append(peers, value)
	}

	return peers
}
