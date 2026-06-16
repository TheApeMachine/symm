package liquidity

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/nomagique/adaptive"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/symm/kraken/market"
	feed "github.com/theapemachine/symm/signal"
	marketsection "github.com/theapemachine/symm/market"
)

type symbolBaseline struct {
	tracker *adaptive.TimeElasticMemory
}

type Metrics struct {
	baselineWindow time.Duration
	epsilon        float64
	baselines      sync.Map
}

func NewMetrics() *Metrics {
	return &Metrics{
		baselineWindow: time.Minute,
		epsilon:        0.000001,
	}
}

func (metrics *Metrics) SeedBaseline(symbol string, at time.Time, volume float64) error {
	state := metrics.ensure(symbol)

	if state == nil {
		return fmt.Errorf("liquidity: invalid baseline state")
	}

	_, err := state.tracker.Update(at, volume)

	return err
}

func (metrics *Metrics) ensure(symbol string) *symbolBaseline {
	raw, _ := metrics.baselines.LoadOrStore(symbol, &symbolBaseline{
		tracker: adaptive.NewTimeElasticMemory(metrics.baselineWindow, metrics.epsilon),
	})

	state, ok := raw.(*symbolBaseline)

	if !ok {
		return nil
	}

	return state
}

func (metrics *Metrics) observeVolume(
	symbol string,
	at time.Time,
	volume float64,
) (relative float64, ready bool, err error) {
	if volume <= 0 {
		return 0, false, nil
	}

	state := metrics.ensure(symbol)

	if state == nil {
		return 0, false, fmt.Errorf("liquidity: invalid baseline state")
	}

	ready = state.tracker.Initialized()
	relative, err = state.tracker.Update(at, volume)

	return relative, ready, err
}

type ScopeSnapshot struct {
	Price    float64
	Volume   float64
	Spread   float64
	Elapsed  float64
	Observed time.Time
}

type Features struct {
	ctx          context.Context
	cancel       context.CancelFunc
	scope        string
	crossSection *marketsection.CrossSection
	metrics      *Metrics
	trade        *feed.Trade
	ticker       *feed.Ticker
	book         *feed.Book
}

func NewFeatures(
	ctx context.Context,
	crossSection *marketsection.CrossSection,
	metrics *Metrics,
	trade *feed.Trade,
	ticker *feed.Ticker,
	book *feed.Book,
) *Features {
	ctx, cancel := context.WithCancel(ctx)

	return &Features{
		ctx:          ctx,
		cancel:       cancel,
		crossSection: crossSection,
		metrics:      metrics,
		trade:        trade,
		ticker:       ticker,
		book:         book,
	}
}

func (features *Features) Snapshot() ScopeSnapshot {
	tickerSnap := features.ticker.Snapshot(features.scope)

	if tickerSnap.Last > 0 && tickerSnap.Bid > 0 && tickerSnap.Ask > tickerSnap.Bid {
		volume := tickerSnap.Volume

		if volume <= 0 {
			volume = tickerSnap.Last
		}

		return ScopeSnapshot{
			Price:    tickerSnap.Last,
			Volume:   volume,
			Spread:   tickerSnap.Ask - tickerSnap.Bid,
			Observed: tickerSnap.Observed,
		}
	}

	tradeSnap := features.tradeScopeSnapshot()

	if tradeSnap.Price > 0 && tradeSnap.Volume > 0 {
		return tradeSnap
	}

	return features.bookScopeSnapshot()
}

func (features *Features) tradeScopeSnapshot() ScopeSnapshot {
	window, ok := features.trade.Window(features.scope)

	if !ok {
		return ScopeSnapshot{}
	}

	quoteVol := 0.0

	features.trade.Scan(features.scope, func(update *market.TradeUpdate) {
		if update != nil && update.Price > 0 && update.Qty > 0 {
			quoteVol += update.Price * update.Qty
		}
	})

	if quoteVol <= 0 {
		return ScopeSnapshot{}
	}

	spread, spreadOK := feed.TouchSpread(window.Prices)

	if !spreadOK {
		return ScopeSnapshot{}
	}

	row, rowErr := market.SymbolRowFromPrices(
		features.scope,
		window.Prices,
		quoteVol,
		1,
		window.Latest.Timestamp,
	)

	if rowErr == nil {
		_ = features.crossSection.Observe(row)
	}

	return ScopeSnapshot{
		Price:    window.Latest.Price,
		Volume:   quoteVol,
		Spread:   spread,
		Elapsed:  window.Elapsed,
		Observed: window.Latest.Timestamp,
	}
}

func (features *Features) bookScopeSnapshot() ScopeSnapshot {
	window, ok := features.book.Window(features.scope)

	if !ok {
		return ScopeSnapshot{}
	}

	price := window.Prices[len(window.Prices)-1]
	spread := window.Spreads[len(window.Spreads)-1]
	depth := window.Latest.Bids[0].Qty + window.Latest.Asks[0].Qty
	quoteVol := depth * price

	if quoteVol <= 0 {
		return ScopeSnapshot{}
	}

	row, rowErr := market.SymbolRowFromPrices(
		features.scope,
		window.Prices,
		quoteVol,
		1,
		window.Latest.Timestamp,
	)

	if rowErr == nil {
		_ = features.crossSection.Observe(row)
	}

	return ScopeSnapshot{
		Price:    price,
		Volume:   quoteVol,
		Spread:   spread,
		Observed: window.Latest.Timestamp,
	}
}

func (features *Features) Read(p []byte) (int, error) {
	snapshot := features.Snapshot()

	if snapshot.Price <= 0 || snapshot.Volume <= 0 {
		return 0, io.EOF
	}

	at := snapshot.Observed

	if at.IsZero() {
		at = time.Now()
	}

	row, rowErr := market.NewSymbolRow(
		features.scope,
		snapshot.Price,
		0,
		snapshot.Volume,
		1,
		at,
	)

	if rowErr == nil {
		_ = features.crossSection.Observe(row)
	}

	peers := features.crossSection.Volumes()

	if len(peers) < 2 {
		return 0, io.EOF
	}

	relativeVolume, baselineReady, baselineErr := features.metrics.observeVolume(
		features.scope,
		at,
		snapshot.Volume,
	)

	if baselineErr != nil {
		return 0, baselineErr
	}

	scaledQuoteVol, scaledPeers := algorithm.AbsoluteScaledVolumes(
		snapshot.Volume,
		peers,
		relativeVolume,
		baselineReady,
	)

	samples := []float64{scaledQuoteVol, float64(len(scaledPeers))}
	samples = append(samples, scaledPeers...)
	samples = append(samples, relativeVolume)

	baselineFlag := 0.0

	if baselineReady {
		baselineFlag = 1
	}

	samples = append(samples, baselineFlag)

	artifact := datura.Acquire("depth-features", datura.Artifact_Type_json)
	artifact.WithRole("features")
	artifact.WithScope(features.scope)
	artifact.WithPayload(feed.EncodePayload(samples...))

	return artifact.Read(p)
}

func (features *Features) Close() error {
	features.cancel()

	return nil
}
