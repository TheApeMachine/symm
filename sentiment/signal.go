package sentiment

import (
	"context"
	"iter"
	"time"

	"github.com/theapemachine/qpool"
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
	symbols     map[string]*symbolState
	peak        *adaptive.Peak
}

var (
	_ engine.System = (*Sentiment)(nil)
	_ engine.Signal = (*Sentiment)(nil)
)

func NewSentiment(ctx context.Context, pool *qpool.Q) *Sentiment {
	ctx, cancel := context.WithCancel(ctx)

	sentiment := &Sentiment{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		broadcasts:  make(map[string]*qpool.BroadcastGroup),
		subscribers: make(map[string]*qpool.Subscriber),
		symbols:     make(map[string]*symbolState),
		peak:        adaptive.NewPeak(),
	}

	for _, channel := range []string{"symbols", "tick", "feedback"} {
		group := pool.CreateBroadcastGroup(channel, 10*time.Millisecond)
		sentiment.subscribers[channel] = group.Subscribe("sentiment:"+channel, 128)
	}

	sentiment.broadcasts["measurements"] = pool.CreateBroadcastGroup("measurements", 10*time.Millisecond)

	return sentiment
}

func (sentiment *Sentiment) Start() error  { return nil }
func (sentiment *Sentiment) State() engine.State { return engine.READY }

func (sentiment *Sentiment) Tick() error {
	select {
	case <-sentiment.ctx.Done():
		return sentiment.ctx.Err()
	case value := <-sentiment.subscribers["symbols"].Incoming:
		for symbol, pair := range value.Value.(map[string]*asset.Pair) {
			if pair != nil {
				sentiment.symbols[symbol] = &symbolState{pair: *pair}
			}
		}
	case value := <-sentiment.subscribers["tick"].Incoming:
		row := value.Value.(market.TickerRow)
		state := sentiment.symbols[row.Symbol]

		if state == nil || row.ChangePct == 0 {
			return nil
		}

		state.changePct = row.ChangePct
	case value := <-sentiment.subscribers["feedback"].Incoming:
		sentiment.Feedback(value.Value.(engine.PredictionFeedback))
	default:
		for measurement := range sentiment.Measure() {
			sentiment.broadcasts["measurements"].Send(&qpool.QValue[any]{Value: measurement})
		}
	}

	return nil
}

func (sentiment *Sentiment) Close() error {
	sentiment.cancel()
	return nil
}

func (sentiment *Sentiment) Source() string { return sentimentSource }

func (sentiment *Sentiment) Measure() iter.Seq[engine.Measurement] {
	return func(yield func(engine.Measurement) bool) {
		positive := 0
		total := 0
		leaders := make(map[string]float64, len(sentiment.symbols))

		for symbol, state := range sentiment.symbols {
			if state.changePct == 0 {
				continue
			}

			total++

			if state.changePct <= 0 {
				continue
			}

			positive++
			leaders[symbol] = state.changePct
		}

		if total < 4 {
			return
		}

		breadth := float64(positive) / float64(total)

		if breadth < minBreadth {
			return
		}

		for symbol, change := range leaders {
			confidence, err := sentiment.peak.Next(change*breadth, leaderPeers(leaders, symbol)...)

			if err != nil || confidence <= 0 {
				continue
			}

			state := sentiment.symbols[symbol]

			if !yield(engine.Measurement{
				Type:       engine.Sentiment,
				Source:     sentimentSource,
				Regime:     "sentiment",
				Reason:     "breadth_leader",
				Pairs:      []asset.Pair{state.pair},
				Confidence: confidence,
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
