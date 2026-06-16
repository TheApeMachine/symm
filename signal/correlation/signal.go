package correlation

import (
	"context"
	"io"
	"sync"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/nomagique"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/qpool"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/logic"
	feed "github.com/theapemachine/symm/signal"
	marketsection "github.com/theapemachine/symm/market"
)

/*
Signal measures how each symbol's return stream correlates with the cross-section median.

| Category          | Correlation | Energy  | Market "Feel"           |
|-------------------|-------------|---------|-------------------------|
| Systemic Herd     | High +      | High    | Beta / Crowded Move     |
| Decoupled Alpha   | Low         | High    | Idiosyncratic / Alpha   |
| Stochastic Noise  | Any         | Low     | Idle / Choppy           |
| Divergent Stress  | High -      | High    | Counter-Herd / Stress   |
*/
type Signal struct {
	ctx          context.Context
	cancel       context.CancelFunc
	err          error
	pool         *qpool.Q[any]
	subscribers  *sync.Map
	algo         io.ReadWriter
	cohort       *algorithm.Cohort
	classifier   *probability.Classifier
	CrossSection *marketsection.CrossSection
	trade        *feed.Trade
	ticker       *feed.Ticker
}

/*
NewSignal composes the cohort pipeline and subscribes to market channels.
*/
func NewSignal(
	ctx context.Context,
	pool *qpool.Q[any],
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	crossSection, crossSectionErr := marketsection.NewCrossSection(&marketsection.CrossSectionConfig{
		MatchWindow: time.Minute,
		ReturnCap:   16,
		MinBars:     4,
		BreadthHist: 16,
	})

	if crossSectionErr != nil {
		cancel()

		return nil
	}

	cohort := algorithm.NewCohort()
	classifier := probability.NewClassifier(
		cohort.HerdReading(),
		cohort.AlphaReading(),
		cohort.NoiseReading(),
		cohort.StressReading(),
	)

	tradeFeed := feed.NewTrade(ctx)
	tickerFeed := feed.NewTicker(ctx)
	tickerFeed.OnUpdate = func(update *krakenmarket.TickerUpdate) {
		if update == nil {
			return
		}

		row, rowErr := update.CompleteSymbol(1, update.Timestamp)

		if rowErr == nil {
			_ = crossSection.Observe(row)
		}
	}

	signal := &Signal{
		ctx:          ctx,
		cancel:       cancel,
		pool:         pool,
		subscribers:  &sync.Map{},
		cohort:       cohort,
		classifier:   classifier,
		CrossSection: crossSection,
		trade:        tradeFeed,
		ticker:       tickerFeed,
		algo: nomagique.Number(
			cohort,
			classifier,
			probability.NewTransitionSurprise(
				5, 1.0/float64(viper.GetInt("signals.feed_ring_capacity")),
			),
		),
	}

	return signal
}

func (signal *Signal) Update(artifact *datura.Artifact) error {
	switch artifact.Peek("role") {
	case "trade":
		signal.trade.Update(
			datura.As[krakenmarket.TradeUpdates](artifact),
		)
	case "ticker":
		signal.ticker.Update(
			datura.As[krakenmarket.TickerUpdates](artifact),
		)
	case "measurement":
		signal.Measure(artifact)
	}

	return nil
}

func (signal *Signal) Measure(in *datura.Artifact) (logic.Measurement, error) {
	scope := in.Peek("scope")

	frame := make([]byte, 8192)

	readCount, readErr := signal.readFeatures(scope, frame)

	if readCount == 0 {
		return logic.Measurement{}, nil
	}

	if readErr != nil && readErr != io.EOF {
		return logic.Measurement{}, readErr
	}

	if _, err := signal.algo.Write(frame[:readCount]); err != nil {
		return logic.Measurement{}, err
	}

	out := datura.Acquire("correlation-out", datura.Artifact_Type_json)

	outCount, err := signal.algo.Read(frame)

	if err != nil && err != io.EOF {
		return logic.Measurement{}, err
	}

	if _, err := out.Write(frame[:outCount]); err != nil {
		return logic.Measurement{}, err
	}

	if !signal.cohort.Outcome().Eligible {
		return logic.Measurement{}, nil
	}

	categoryIndex := signal.classifier.CategoryIndex()

	if categoryIndex == 0 {
		return logic.Measurement{}, nil
	}

	confidence, confidenceErr := signal.classifier.Confidence(categoryIndex)

	if confidenceErr != nil {
		return logic.Measurement{}, confidenceErr
	}

	snapshot := signal.trade.Snapshot(scope)
	price := snapshot.Price

	if price <= 0 {
		window := signal.CrossSection.MinBarsRequired()
		returns := signal.CrossSection.SymbolReturns(scope, window)

		if len(returns) > 0 {
			price = 100 * (1 + returns[len(returns)-1])
		}
	}

	spread := medianAbsolute(signal.CrossSection.SymbolReturns(scope, signal.CrossSection.MinBarsRequired()))

	if spread <= 0 {
		spread = price * 0.0001
	}

	return logic.Measurement{
		Source:     logic.SourceCorrelation,
		Symbol:     scope,
		Price:      price,
		Strength:   signal.cohort.Outcome().Strength,
		Volume:     snapshot.Volume,
		Spread:     spread,
		Elapsed:    snapshot.Elapsed,
		Category:   correlationCategory(categoryIndex),
		Regime:     logic.RegimeTypeNone,
		Position:   logic.PositionTypeNone,
		Confidence: confidence,
		Surprise:   datura.Peek[float64](out, "transition.surprise"),
		ObservedAt: snapshot.Observed,
	}, nil
}

func (signal *Signal) readFeatures(scope string, buffer []byte) (int, error) {
	if scope != "" {
		signal.observeTrade(scope)
	}

	window := signal.CrossSection.MinBarsRequired()
	at := time.Now()
	snapshot := signal.CrossSection.PeerWindowSnapshot(window, at)
	symbolReturns := signal.CrossSection.SymbolReturns(scope, window)

	if len(symbolReturns) < window || len(snapshot.MarketReturns) < window {
		return 0, io.EOF
	}

	samples := []float64{float64(window)}
	samples = append(samples,
		float64(len(symbolReturns)),
		float64(len(snapshot.MarketReturns)),
		float64(len(snapshot.PeerCorrelations)),
		float64(len(snapshot.PeerEnergies)),
	)
	samples = append(samples, symbolReturns...)
	samples = append(samples, snapshot.MarketReturns...)
	samples = append(samples, snapshot.PeerCorrelations...)
	samples = append(samples, snapshot.PeerEnergies...)

	artifact := datura.Acquire("cohort-features", datura.Artifact_Type_json)
	artifact.WithRole("features")
	artifact.WithScope(scope)
	artifact.WithPayload(feed.EncodePayload(samples...))

	return artifact.Read(buffer)
}

func (signal *Signal) observeTrade(symbol string) {
	var (
		first    *krakenmarket.TradeUpdate
		latest   *krakenmarket.TradeUpdate
		quoteVol float64
		prices   []float64
	)

	signal.trade.Scan(symbol, func(update *krakenmarket.TradeUpdate) {
		if update == nil {
			return
		}

		if first == nil {
			first = update
		}

		latest = update
		quoteVol += update.Price * update.Qty
		prices = append(prices, update.Price)
	})

	if latest == nil || len(prices) < 2 || quoteVol <= 0 {
		return
	}

	row, rowErr := krakenmarket.SymbolRowFromPrices(
		symbol,
		prices,
		quoteVol,
		1,
		latest.Timestamp,
	)

	if rowErr == nil {
		_ = signal.CrossSection.Observe(row)
	}
}

func medianAbsolute(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	sum := 0.0

	for _, value := range values {
		sum += value

		if value < 0 {
			sum -= 2 * value
		}
	}

	return sum / float64(len(values))
}

func correlationCategory(categoryIndex int) logic.CategoryType {
	switch categoryIndex {
	case 1:
		return logic.CategorySystemicHerd
	case 2:
		return logic.CategoryDecoupledAlpha
	case 3:
		return logic.CategoryStochasticNoise
	case 4:
		return logic.CategoryDivergentStress
	default:
		return logic.CategoryTypeNone
	}
}

func (signal *Signal) Error() error {
	return signal.err
}

func (signal *Signal) Close() error {
	signal.cancel()

	return nil
}
