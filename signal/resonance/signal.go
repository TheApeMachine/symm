package resonance

import (
	"context"
	"math"
	"sync"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/nomagique/learning"
	"github.com/theapemachine/qpool"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/logic"
	feed "github.com/theapemachine/symm/signal"
)

type Signal struct {
	ctx       context.Context
	cancel    context.CancelFunc
	err       error
	pool      *qpool.Q[any]
	manifolds sync.Map // Map of symbol string -> *learning.ResonanceManifold
	trade     *feed.Trade
	book      *feed.Book
	ticker    *feed.Ticker
	arch      []int   // e.g., []int{4, 8, 3} (4 inputs -> 8 hidden -> 3 latent)
	alpha     float64 // Learning rate for the manifold
}

func NewSignal(ctx context.Context, pool *qpool.Q[any], arch []int, alpha float64) *Signal {
	ctx, cancel := context.WithCancel(ctx)
	return &Signal{
		ctx:       ctx,
		cancel:    cancel,
		pool:      pool,
		manifolds: sync.Map{},
		trade:     feed.NewTrade(ctx),
		book:      feed.NewBook(ctx),
		ticker:    feed.NewTicker(ctx),
		arch:      arch,
		alpha:     alpha,
	}
}

// Update channels
func (signal *Signal) Update(artifact *datura.Artifact) error {
	switch artifact.Peek("role") {
	case "book":
		signal.book.Update(datura.As[[]*krakenmarket.BookUpdate](artifact)) // Adjust based on your types
	case "trade":
		signal.trade.Update(datura.As[[]*krakenmarket.TradeUpdate](artifact))
	case "ticker":
		signal.ticker.Update(datura.As[[]*krakenmarket.TickerUpdate](artifact))
	}
	return nil
}

// Measure processes incoming ticks, settles the manifold, and returns a measurement
func (signal *Signal) Measure(in *datura.Artifact) (logic.Measurement, error) {
	scope := in.Peek("scope")
	if scope == "" {
		return logic.Measurement{}, nil
	}

	// 1. Extract a cheap, lightweight feature vector
	tickerSnap := signal.ticker.Snapshot(scope)
	if tickerSnap.Last <= 0 {
		return logic.Measurement{}, nil
	}

	spread := signal.book.Spread(scope)
	volume := tickerSnap.Volume
	change := tickerSnap.ChangePct

	// Vector: [LastPrice, SpreadBPS, Volume, ChangePct]
	inputVector := []float64{
		tickerSnap.Last,
		spread,
		volume,
		change,
	}

	// 2. Load or initialize the ResonanceManifold for this symbol
	raw, _ := signal.manifolds.LoadOrStore(scope, func() any {
		m, err := learning.NewResonanceManifold(signal.arch, 0, signal.alpha)
		if err != nil {
			signal.err = err
			return nil
		}
		return m
	}())

	manifold, ok := raw.(*learning.ResonanceManifold)
	if !ok || manifold == nil {
		return logic.Measurement{}, signal.err
	}

	// 3. Settle and learn online from the feature stream
	// Settle performs the top-down/bottom-up prediction loop
	_ = manifold.Settle(inputVector, true)

	// Continuous unsupervised online learning
	manifold.Learn(nil)

	// 4. Retrieve metrics
	reconstructionError := manifold.ReconstructionError()
	latentState := manifold.LatentState()

	// Use peak latent neuron activation as a proxy for structural confidence
	peakActivation := 0.0
	for _, val := range latentState {
		peakActivation = math.Max(peakActivation, math.Abs(val))
	}

	observedAt := tickerSnap.Observed
	if observedAt.IsZero() {
		observedAt = time.Now()
	}

	return logic.Measurement{
		Source:     "resonance", // Define logic.SourceResonance if wanted
		Symbol:     scope,
		Price:      tickerSnap.Last,
		Strength:   peakActivation, // Representing structural weight
		Volume:     volume,
		Spread:     spread,
		Elapsed:    tickerSnap.Elapsed,
		Category:   logic.CategoryType(signal.determineCategory(latentState)),
		Confidence: 1.0 / (1.0 + reconstructionError), // Low reconstruction error = High confidence
		Surprise:   reconstructionError,               // High reconstruction error = High novelty/surprise
		ObservedAt: observedAt,
	}, nil
}

func (signal *Signal) determineCategory(latent []float64) string {
	// Simple mapping: project the most dominant latent neuron to a qualitative category
	if len(latent) == 0 {
		return "resonance_noise"
	}
	maxIdx := 0
	maxVal := 0.0
	for i, v := range latent {
		if math.Abs(v) > math.Abs(maxVal) {
			maxVal = v
			maxIdx = i
		}
	}
	switch maxIdx {
	case 0:
		return "laminar_resonance"
	case 1:
		return "turbulent_resonance"
	default:
		return "equilibrium"
	}
}
