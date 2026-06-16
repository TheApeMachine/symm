package liquidity

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
Signal identifies opportunities in thin markets by ranking quote volume against peers.

| Category         | Rank vs. Peers   | Volume   | Market "Feel"                |
|------------------|------------------|----------|------------------------------|
| Extreme Scarcity | Peak Illiquidity | Very Low | High Convexity / Fragile     |
| Median Depth     | Middle           | Normal   | Standard Efficiency          |
| Robust Liquidity | Bottom (Deep)    | High     | Efficient / Safe             |
*/
type Signal struct {
	ctx          context.Context
	cancel       context.CancelFunc
	err          error
	pool         *qpool.Q[any]
	subscribers  *sync.Map
	algo         io.ReadWriter
	depth        *algorithm.Depth
	classifier   *probability.Classifier
	CrossSection *marketsection.CrossSection
	Metrics      *Metrics
	features     *Features
	trade        *feed.Trade
	ticker       *feed.Ticker
	book         *feed.Book
}

/*
NewSignal composes the depth pipeline and subscribes to market channels.
*/
func NewSignal(
	ctx context.Context,
	pool *qpool.Q[any],
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	crossSection, crossSectionErr := marketsection.NewCrossSection(&marketsection.CrossSectionConfig{
		MatchWindow: time.Minute,
		ReturnCap:   64,
		MinBars:     8,
		BreadthHist: 64,
	})

	if crossSectionErr != nil {
		cancel()

		return nil
	}

	depth := algorithm.NewDepth()
	classifier := probability.NewClassifier(
		depth.ScarcityReading(),
		depth.MedianReading(),
		depth.RobustReading(),
	)

	metrics := NewMetrics()
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
	bookFeed := feed.NewBook(ctx)

	signal := &Signal{
		ctx:          ctx,
		cancel:       cancel,
		pool:         pool,
		subscribers:  &sync.Map{},
		depth:        depth,
		classifier:   classifier,
		CrossSection: crossSection,
		Metrics:      metrics,
		trade:        tradeFeed,
		ticker:       tickerFeed,
		book:         bookFeed,
		features:     NewFeatures(ctx, crossSection, metrics, tradeFeed, tickerFeed, bookFeed),
		algo: nomagique.Number(
			depth,
			classifier,
			probability.NewTransitionSurprise(
				4, 1.0/float64(viper.GetInt("signals.feed_ring_capacity")),
			),
		),
	}

	return signal
}

func (signal *Signal) Update(artifact *datura.Artifact) error {
	switch artifact.Peek("role") {
	case "book":
		signal.book.Update(
			datura.As[krakenmarket.BookUpdates](artifact),
		)
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

	signal.features.scope = scope

	snapshot := signal.features.Snapshot()
	at := snapshot.Observed

	if at.IsZero() {
		at = time.Now()
	}

	peers := signal.CrossSection.Volumes()

	if len(peers) < 2 {
		return logic.Measurement{
			Source:     logic.SourceLiquidity,
			Symbol:     scope,
			ObservedAt: at,
			BestEffort: true,
			GapReason:  "liquidity: peer universe is not ready",
		}, nil
	}

	frame := make([]byte, 8192)

	readCount, readErr := signal.features.Read(frame)

	if readCount == 0 {
		return logic.Measurement{}, nil
	}

	if readErr != nil && readErr != io.EOF {
		return logic.Measurement{}, readErr
	}

	if _, err := signal.algo.Write(frame[:readCount]); err != nil {
		return logic.Measurement{}, err
	}

	out := datura.Acquire("liquidity-out", datura.Artifact_Type_json)

	outCount, err := signal.algo.Read(frame)

	if err != nil && err != io.EOF {
		return logic.Measurement{}, err
	}

	if _, err := out.Write(frame[:outCount]); err != nil {
		return logic.Measurement{}, err
	}

	if !signal.depth.Outcome().Eligible {
		return logic.Measurement{}, nil
	}

	categoryIndex := signal.depth.Outcome().Category

	if categoryIndex == 0 {
		categoryIndex = signal.classifier.CategoryIndex()
	}

	if categoryIndex == 0 {
		return logic.Measurement{}, nil
	}

	confidence, confidenceErr := signal.classifier.Confidence(categoryIndex)

	if confidenceErr != nil {
		return logic.Measurement{}, confidenceErr
	}

	if snapshot.Spread <= 0 {
		return logic.Measurement{
			Source:     logic.SourceLiquidity,
			Symbol:     scope,
			ObservedAt: at,
			BestEffort: true,
			GapReason:  "liquidity: invalid spread",
		}, nil
	}

	return logic.Measurement{
		Source:     logic.SourceLiquidity,
		Symbol:     scope,
		Price:      snapshot.Price,
		Strength:   signal.depth.Outcome().Strength,
		Volume:     snapshot.Volume,
		Spread:     snapshot.Spread,
		Elapsed:    snapshot.Elapsed,
		Category:   liquidityCategory(categoryIndex),
		Regime:     logic.RegimeTypeNone,
		Position:   logic.PositionTypeNone,
		Confidence: confidence,
		Surprise:   datura.Peek[float64](out, "transition.surprise"),
		ObservedAt: at,
	}, nil
}

func liquidityCategory(categoryIndex int) logic.CategoryType {
	switch categoryIndex {
	case 1:
		return logic.CategoryExtremeScarcity
	case 2:
		return logic.CategoryMedianDepth
	case 3:
		return logic.CategoryRobustLiquidity
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
