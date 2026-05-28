package liquidity

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
	"github.com/theapemachine/symm/numeric/learned"
)

const (
	liquiditySource   = "liquidity"
	minLiquidityPeers = 2
)

type symbolState struct {
	mu            sync.RWMutex
	pair          asset.Pair
	dailyQuoteVol float64
	last          float64
	bid           float64
	ask           float64
	forecast      *learned.Forecast
}

type symbolSnapshot struct {
	pair          asset.Pair
	dailyQuoteVol float64
	last          float64
	bid           float64
	ask           float64
}

func newSymbolState(pair asset.Pair) *symbolState {
	return &symbolState{
		pair:     pair,
		forecast: learned.NewForecast(0),
	}
}

func (state *symbolState) observeTicker(row market.TickerRow) {
	state.mu.Lock()
	defer state.mu.Unlock()

	state.dailyQuoteVol = row.Volume * row.Last
	state.last = row.Last

	if row.Bid > 0 {
		state.bid = row.Bid
	}

	if row.Ask > 0 {
		state.ask = row.Ask
	}
}

func (state *symbolState) snapshot() symbolSnapshot {
	state.mu.RLock()
	defer state.mu.RUnlock()

	return symbolSnapshot{
		pair:          state.pair,
		dailyQuoteVol: state.dailyQuoteVol,
		last:          state.last,
		bid:           state.bid,
		ask:           state.ask,
	}
}

func (state *symbolState) applyFeedback(predictedReturn, actualReturn float64) error {
	state.mu.Lock()
	defer state.mu.Unlock()

	if state.forecast == nil {
		state.forecast = learned.NewForecast(0)
	}

	_, err := state.forecast.Next(0, predictedReturn, actualReturn)

	return err
}

/*
Liquidity ranks cross-section quote volume below the peer median.
*/
type Liquidity struct {
	ctx         context.Context
	cancel      context.CancelFunc
	pool        *qpool.Q
	broadcasts  map[string]*qpool.BroadcastGroup
	subscribers map[string]*qpool.Subscriber
	symbols     sync.Map
	adaptiveMu  sync.Mutex
	belowMedian *adaptive.BelowMedian
	peak        *adaptive.Peak
	pending     []string
	requested   sync.Map
}

func NewLiquidity(ctx context.Context, pool *qpool.Q) *Liquidity {
	ctx, cancel := context.WithCancel(ctx)

	liquidity := &Liquidity{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		broadcasts:  make(map[string]*qpool.BroadcastGroup),
		subscribers: make(map[string]*qpool.Subscriber),
		symbols:     sync.Map{},
		belowMedian: adaptive.NewBelowMedian(),
		peak:        adaptive.NewPeak(),
		requested:   sync.Map{},
	}

	for _, channel := range []string{"symbols", "tick", "feedback"} {
		group := pool.CreateBroadcastGroup(channel, 10*time.Millisecond)
		liquidity.subscribers[channel] = group.Subscribe("liquidity:"+channel, 128)
	}

	liquidity.broadcasts["measurements"] = pool.CreateBroadcastGroup("measurements", 10*time.Millisecond)
	liquidity.broadcasts["subscriptions"] = pool.CreateBroadcastGroup("subscriptions", 10*time.Millisecond)

	return liquidity
}

func (liquidity *Liquidity) Start() error        { return nil }
func (liquidity *Liquidity) State() engine.State { return engine.READY }

func (liquidity *Liquidity) Tick() error {
	errnie.Info("starting liquidity tick")

	var wg sync.WaitGroup

	wg.Go(func() {
		for {
			select {
			case <-liquidity.ctx.Done():
				return
			case value, ok := <-liquidity.subscribers["symbols"].Incoming:
				if !ok {
					errnie.Error(fmt.Errorf("liquidity symbols channel closed"))
					return
				}

				pairs, pairsOK := value.Value.(map[string]*asset.Pair)
				if !pairsOK {
					errnie.Error(fmt.Errorf("liquidity: invalid symbols payload: %T", value.Value))
					continue
				}

				for symbol, pair := range pairs {
					if pair == nil {
						continue
					}

					liquidity.symbols.Store(symbol, newSymbolState(*pair))

					if pair.Quote != config.System.QuoteCurrency {
						continue
					}

					if _, seen := liquidity.requested.Load(symbol); seen {
						continue
					}

					liquidity.pending = append(liquidity.pending, symbol)
				}

				liquidity.publishPulse()
			}
		}
	})

	wg.Go(func() {
		for {
			select {
			case <-liquidity.ctx.Done():
				return
			case value, ok := <-liquidity.subscribers["tick"].Incoming:
				if !ok {
					errnie.Error(fmt.Errorf("liquidity tick channel closed"))
					return
				}

				row, rowOK := value.Value.(market.TickerRow)
				if !rowOK {
					errnie.Error(fmt.Errorf("liquidity: invalid ticker payload: %T", value.Value))
					continue
				}

				raw, ok := liquidity.symbols.Load(row.Symbol)

				if ok && row.Last > 0 {
					state := raw.(*symbolState)
					state.observeTicker(row)

					liquidity.publishPulse()
				}

			}
		}
	})

	wg.Go(func() {
		for {
			select {
			case <-liquidity.ctx.Done():
				return
			case value, ok := <-liquidity.subscribers["feedback"].Incoming:
				if !ok {
					errnie.Error(fmt.Errorf("liquidity feedback channel closed"))
					return
				}

				fb, fbOK := value.Value.(engine.PredictionFeedback)
				if !fbOK {
					errnie.Error(fmt.Errorf("liquidity: invalid feedback payload: %T", value.Value))
					continue
				}

				liquidity.Feedback(fb)
				liquidity.publishPulse()
			}
		}
	})

	wg.Wait()
	return liquidity.ctx.Err()
}

func (liquidity *Liquidity) requestedCount() int {
	count := 0

	liquidity.requested.Range(func(key, value any) bool {
		count++
		return true
	})

	return count
}

func (liquidity *Liquidity) publishPulse() {
	scanCap := max(config.System.MaxScanSymbols/8, 1)
	requested := liquidity.requestedCount()

	if len(liquidity.pending) > 0 && requested < scanCap {
		remaining := scanCap - requested
		batch := min(min(config.System.SubscribeBatch, remaining), len(liquidity.pending))

		symbols := liquidity.pending[:batch]
		liquidity.pending = liquidity.pending[batch:]

		for _, symbol := range symbols {
			liquidity.requested.Store(symbol, struct{}{})
		}

		liquidity.broadcasts["subscriptions"].Send(&qpool.QValue[any]{Value: symbols})
	}

	liquidity.publishMeasurements()
}

func (liquidity *Liquidity) collectQuotes() map[string]float64 {
	quotes := make(map[string]float64)

	liquidity.symbols.Range(func(key, value any) bool {
		symbol := key.(string)
		state := value.(*symbolState)
		snapshot := state.snapshot()

		if snapshot.dailyQuoteVol > 0 {
			quotes[symbol] = snapshot.dailyQuoteVol
		}

		return true
	})

	return quotes
}

func (liquidity *Liquidity) collectCandidates(quotes map[string]float64) map[string]float64 {
	candidates := make(map[string]float64, len(quotes))

	for symbol, quoteVol := range quotes {
		peers := adaptive.PeerValues(quotes, symbol)

		if len(peers) < minLiquidityPeers {
			continue
		}

		liquid, err := liquidity.belowMedianNext(quoteVol, peers...)

		if err != nil {
			errnie.Error(err)
			continue
		}

		if liquid <= 0 {
			continue
		}

		score := adaptive.IlliquidityScore(quoteVol, peers)

		if score <= 0 {
			continue
		}

		candidates[symbol] = score
	}

	return candidates
}

func (liquidity *Liquidity) publishMeasurements() {
	quotes := liquidity.collectQuotes()
	candidates := liquidity.collectCandidates(quotes)
	waiters := make([]chan *qpool.QValue[any], 0)

	for symbol, rawScore := range candidates {
		raw, ok := liquidity.symbols.Load(symbol)

		if !ok {
			continue
		}

		state := raw.(*symbolState)
		snapshot := state.snapshot()
		score := rawScore

		waiters = append(
			waiters,
			liquidity.pool.ScheduleFast(liquidity.ctx, func(ctx context.Context) (any, error) {
				peakScore, err := liquidity.peakNext(
					score, adaptive.PeerValues(candidates, symbol)...,
				)

				if err != nil {
					return nil, err
				}

				if peakScore <= 0 {
					return nil, nil
				}

				confidence := liquidity.confidenceFromScore(
					peakScore, adaptive.PeerValues(candidates, symbol),
				)

				if confidence <= 0 {
					return nil, nil
				}

				return engine.Measurement{
					Type:       engine.Liquidity,
					Source:     liquiditySource,
					Regime:     "liquidity",
					Reason:     "below_median",
					Pairs:      []asset.Pair{snapshot.pair},
					Confidence: confidence,
					Last:       snapshot.last,
					Bid:        snapshot.bid,
					Ask:        snapshot.ask,
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

		liquidity.broadcasts["measurements"].Send(&qpool.QValue[any]{
			Value: measurement,
		})
	}
}

func (liquidity *Liquidity) Close() error {
	liquidity.cancel()
	return nil
}

func (liquidity *Liquidity) Source() string { return liquiditySource }

func (liquidity *Liquidity) Measure() iter.Seq[engine.Measurement] {
	return func(yield func(engine.Measurement) bool) {
		quotes := liquidity.collectQuotes()
		candidates := liquidity.collectCandidates(quotes)

		for symbol, rawScore := range candidates {
			raw, ok := liquidity.symbols.Load(symbol)

			if !ok {
				continue
			}

			state := raw.(*symbolState)
			snapshot := state.snapshot()
			peakScore, err := liquidity.peakNext(
				rawScore, adaptive.PeerValues(candidates, symbol)...,
			)

			if err != nil {
				errnie.Error(err)
				continue
			}

			if peakScore <= 0 {
				continue
			}

			confidence := liquidity.confidenceFromScore(
				peakScore, adaptive.PeerValues(candidates, symbol),
			)

			if confidence <= 0 {
				continue
			}

			if !yield(engine.Measurement{
				Type:       engine.Liquidity,
				Source:     liquiditySource,
				Regime:     "liquidity",
				Reason:     "below_median",
				Pairs:      []asset.Pair{snapshot.pair},
				Confidence: confidence,
				Last:       snapshot.last,
				Bid:        snapshot.bid,
				Ask:        snapshot.ask,
			}) {
				return
			}
		}
	}
}

func (liquidity *Liquidity) belowMedianNext(
	quoteVol float64,
	peers ...float64,
) (float64, error) {
	liquidity.adaptiveMu.Lock()
	defer liquidity.adaptiveMu.Unlock()

	return liquidity.belowMedian.Next(quoteVol, peers...)
}

func (liquidity *Liquidity) peakNext(
	score float64,
	peers ...float64,
) (float64, error) {
	liquidity.adaptiveMu.Lock()
	defer liquidity.adaptiveMu.Unlock()

	return liquidity.peak.Next(score, peers...)
}

func (liquidity *Liquidity) Feedback(feedback engine.PredictionFeedback) {
	if !engine.FeedbackIncludesSource(feedback, liquiditySource) || feedback.Symbol == "" || feedback.PredictedReturn <= 0 {
		return
	}

	raw, ok := liquidity.symbols.Load(feedback.Symbol)

	if !ok {
		return
	}

	state := raw.(*symbolState)

	if err := state.applyFeedback(feedback.PredictedReturn, feedback.ActualReturn); err != nil {
		errnie.Error(err)
	}
}

/*
confidenceFromScore scores how illiquid the current symbol is versus peers.
With peer context it combines illiquidity depth and cross-section lead; alone it
uses the illiquidity score directly so a single reading is not pinned at 50%.
*/
func (liquidity *Liquidity) confidenceFromScore(score float64, peers []float64) float64 {
	if score <= 0 {
		return 0
	}

	if len(peers) > 0 {
		maxPeer := 0.0

		for _, peer := range peers {
			if peer > maxPeer {
				maxPeer = peer
			}
		}

		if score > maxPeer {
			margin := (score - maxPeer) / score

			return engine.AlignConfidence(score, margin)
		}
	}

	return engine.ConfidenceFromScore(score)
}
