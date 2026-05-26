package leadlag

import (
	"context"
	"iter"
	"time"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/numeric/adaptive"
)

const (
	leadlagSource  = "leadlag"
	anchorSymbol   = "BTC/EUR"
	minAnchorMove  = 0.05
	minLagFraction = 0.35
)

type symbolState struct {
	pair      asset.Pair
	changePct float64
}

/*
LeadLag detects altcoins lagging a moving anchor pair.
*/
type LeadLag struct {
	ctx         context.Context
	cancel      context.CancelFunc
	pool        *qpool.Q
	broadcasts  map[string]*qpool.BroadcastGroup
	subscribers map[string]*qpool.Subscriber
	symbols     map[string]*symbolState
	peak        *adaptive.Peak
	pending     []string
	requested   map[string]struct{}
}

func NewLeadLag(ctx context.Context, pool *qpool.Q) *LeadLag {
	ctx, cancel := context.WithCancel(ctx)

	leadlag := &LeadLag{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		broadcasts:  make(map[string]*qpool.BroadcastGroup),
		subscribers: make(map[string]*qpool.Subscriber),
		symbols:     make(map[string]*symbolState),
		peak:        adaptive.NewPeak(),
		requested:   make(map[string]struct{}),
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
	select {
	case <-leadlag.ctx.Done():
		return leadlag.ctx.Err()
	case value := <-leadlag.subscribers["symbols"].Incoming:
		for symbol, pair := range value.Value.(map[string]*asset.Pair) {
			if pair == nil {
				continue
			}

			leadlag.symbols[symbol] = &symbolState{pair: *pair}

			if pair.Quote != config.System.QuoteCurrency {
				continue
			}

			if _, seen := leadlag.requested[symbol]; seen {
				continue
			}

			if symbol == anchorSymbol {
				leadlag.requested[symbol] = struct{}{}
				leadlag.broadcasts["subscriptions"].Send(&qpool.QValue[any]{Value: []string{symbol}})
				continue
			}

			leadlag.pending = append(leadlag.pending, symbol)
		}
	case value := <-leadlag.subscribers["tick"].Incoming:
		row := value.Value.(market.TickerRow)
		state := leadlag.symbols[row.Symbol]

		if state == nil || row.ChangePct == 0 {
			break
		}

		state.changePct = row.ChangePct

		anchor := leadlag.symbols[anchorSymbol]

		if anchor == nil || anchor.changePct < minAnchorMove || len(leadlag.pending) == 0 {
			break
		}

		scanCap := config.System.MaxScanSymbols / 8

		if scanCap < 1 {
			scanCap = 1
		}

		if len(leadlag.requested) >= scanCap {
			break
		}

		remaining := scanCap - len(leadlag.requested)
		batch := config.System.SubscribeBatch

		if batch > remaining {
			batch = remaining
		}

		if batch > len(leadlag.pending) {
			batch = len(leadlag.pending)
		}

		symbols := leadlag.pending[:batch]
		leadlag.pending = leadlag.pending[batch:]

		for _, symbol := range symbols {
			leadlag.requested[symbol] = struct{}{}
		}

		leadlag.broadcasts["subscriptions"].Send(&qpool.QValue[any]{Value: symbols})
	case value := <-leadlag.subscribers["feedback"].Incoming:
		leadlag.Feedback(value.Value.(engine.PredictionFeedback))
	default:
	}

	leadlag.publishPulse()

	return nil
}

func (leadlag *LeadLag) publishPulse() {
	for measurement := range leadlag.Measure() {
		leadlag.broadcasts["measurements"].Send(&qpool.QValue[any]{Value: measurement})
	}
}

func (leadlag *LeadLag) Close() error {
	leadlag.cancel()
	return nil
}

func (leadlag *LeadLag) Source() string { return leadlagSource }

func (leadlag *LeadLag) Measure() iter.Seq[engine.Measurement] {
	return func(yield func(engine.Measurement) bool) {
		anchor := leadlag.symbols[anchorSymbol]

		if anchor == nil || anchor.changePct < minAnchorMove {
			return
		}

		lags := make(map[string]float64, len(leadlag.symbols))

		for symbol, state := range leadlag.symbols {
			if symbol == anchorSymbol || state.changePct <= 0 {
				continue
			}

			lag := anchor.changePct - state.changePct

			if lag <= anchor.changePct*minLagFraction {
				continue
			}

			lags[symbol] = lag / anchor.changePct
		}

		for symbol, lagRatio := range lags {
			peakLag, err := leadlag.peak.Next(lagRatio, peerValues(lags, symbol)...)

			if err != nil || peakLag <= 0 {
				continue
			}

			state := leadlag.symbols[symbol]
			anchorStrength := engine.ExcessRatio(anchor.changePct / minAnchorMove)
			confidence := engine.AlignConfidence(peakLag, anchorStrength)

			if confidence <= 0 {
				continue
			}

			if !yield(engine.Measurement{
				Type:       engine.LeadLag,
				Source:     leadlagSource,
				Regime:     "cross_asset",
				Reason:     "anchor_lag",
				Pairs:      []asset.Pair{state.pair},
				Confidence: confidence,
			}) {
				return
			}
		}
	}
}

func (leadlag *LeadLag) Feedback(feedback engine.PredictionFeedback) {
	if feedback.Source != leadlagSource {
		return
	}
}

func peerValues(values map[string]float64, skip string) []float64 {
	peers := make([]float64, 0, len(values)-1)

	for symbol, value := range values {
		if symbol == skip {
			continue
		}

		peers = append(peers, value)
	}

	return peers
}
