package depthflow

import (
	"context"
	"io"
	"math"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/symm/kraken/market"
	marketsection "github.com/theapemachine/symm/market"
	feed "github.com/theapemachine/symm/signal"
)

type BookSnapshot struct {
	Weighted        float64
	Level1          float64
	Flat            float64
	FlatOK          bool
	Mid             float64
	Spread          float64
	TouchDepth      float64
	Observed        time.Time
	Elapsed         float64
	WeightedHistory []float64
	Level1History   []float64
	FlatHistory     []float64
}

type Features struct {
	ctx          context.Context
	cancel       context.CancelFunc
	scope        string
	crossSection *marketsection.CrossSection
	book         *feed.Book
	trade        *feed.Trade
}

func NewFeatures(
	ctx context.Context,
	crossSection *marketsection.CrossSection,
	book *feed.Book,
	trade *feed.Trade,
) *Features {
	ctx, cancel := context.WithCancel(ctx)

	return &Features{
		ctx:          ctx,
		cancel:       cancel,
		crossSection: crossSection,
		book:         book,
		trade:        trade,
	}
}

func (features *Features) BookSnapshot(symbol string) BookSnapshot {
	var (
		weightedHistory []float64
		level1History   []float64
		flatHistory     []float64
		snapshot        *market.BookUpdate
		weighted        float64
		level1          float64
		flat            float64
		flatOK          bool
		mid             float64
		spread          float64
		touchDepth      float64
		first           *market.BookUpdate
		latest          *market.BookUpdate
	)

	features.book.Scan(symbol, func(frame *market.BookUpdate) {
		if frame == nil {
			return
		}

		if first == nil {
			first = frame
		}

		latest = frame

		if frame.Type == "snapshot" {
			snapshot = frame
		}

		if snapshot == nil {
			return
		}

		bids := frame.Bids
		asks := frame.Asks

		if len(bids) == 0 {
			bids = snapshot.Bids
		}

		if len(asks) == 0 {
			asks = snapshot.Asks
		}

		if len(bids) == 0 || len(asks) == 0 {
			return
		}

		touchMid := (bids[0].Price + asks[0].Price) / 2
		touchSpread := asks[0].Price - bids[0].Price

		frameWeighted, frameWeightedOK := weightedImbalance(
			snapshot.Bids, snapshot.Asks, touchMid, touchSpread,
		)
		frameLevel1, frameLevel1OK := level1Imbalance(bids, asks)
		frameFlat, frameFlatOK := flatImbalance(snapshot.Bids, snapshot.Asks)

		if frameWeightedOK {
			weightedHistory = append(weightedHistory, math.Abs(frameWeighted))
		}

		if frameLevel1OK {
			level1History = append(level1History, math.Abs(frameLevel1))
		}

		if frameFlatOK {
			flatHistory = append(flatHistory, math.Abs(frameFlat))
		}

		weighted = frameWeighted
		level1 = frameLevel1
		flat = frameFlat
		flatOK = frameFlatOK
		mid = touchMid
		spread = touchSpread
		touchDepth = bids[0].Qty + asks[0].Qty
	})

	if latest == nil || mid <= 0 || spread <= 0 || len(weightedHistory) == 0 || len(level1History) == 0 {
		return BookSnapshot{}
	}

	elapsed := 0.0

	if first != nil {
		elapsed = latest.Timestamp.Sub(first.Timestamp).Seconds()
	}

	quoteVol := mid * touchDepth

	if quoteVol > 0 {
		value := math.Abs(weighted) / mid
		row, rowErr := market.NewSymbolRow(symbol, mid, value, quoteVol, 0, latest.Timestamp)

		if rowErr == nil {
			_ = features.crossSection.Observe(row)
		}
	}

	return BookSnapshot{
		Weighted:        weighted,
		Level1:          level1,
		Flat:            flat,
		FlatOK:          flatOK,
		Mid:             mid,
		Spread:          spread,
		TouchDepth:      touchDepth,
		Observed:        latest.Timestamp,
		Elapsed:         elapsed,
		WeightedHistory: weightedHistory,
		Level1History:   level1History,
		FlatHistory:     flatHistory,
	}
}

func (features *Features) observeTrades(symbol string) {
	var (
		buyVolume  float64
		sellVolume float64
		prices     []float64
		latest     *market.TradeUpdate
	)

	features.trade.Scan(symbol, func(update *market.TradeUpdate) {
		if update == nil || update.Price <= 0 || update.Qty <= 0 {
			return
		}

		prices = append(prices, update.Price)
		latest = update

		if update.Side == "buy" {
			buyVolume += update.Qty
		}

		if update.Side == "sell" {
			sellVolume += update.Qty
		}
	})

	gross := buyVolume + sellVolume

	if len(prices) < 2 || gross <= 0 || latest == nil {
		return
	}

	pressure := (buyVolume - sellVolume) / gross

	if pressure == 0 {
		return
	}

	quoteVol := gross * prices[len(prices)-1]

	row, rowErr := market.SymbolRowFromPrices(symbol, prices, quoteVol, pressure, latest.Timestamp)

	if rowErr != nil {
		return
	}

	_ = features.crossSection.Observe(row)
}

func (features *Features) Artifact() *datura.Artifact {
	features.observeTrades(features.scope)

	snapshot := features.BookSnapshot(features.scope)

	if snapshot.Mid <= 0 || len(snapshot.WeightedHistory) == 0 || len(snapshot.Level1History) == 0 {
		return nil
	}

	tradePressure := features.crossSection.TradePressure(features.scope)

	flatOK := 0.0

	if snapshot.FlatOK {
		flatOK = 1
	}

	const depthflowHeaderFloats = 11

	maxFloats := feed.MaxFeatureFloats(
		"bookflow-features",
		"features",
		features.scope,
		depthflowHeaderFloats,
	)
	maxVariableFloats := maxFloats - depthflowHeaderFloats

	weightedHistory := snapshot.WeightedHistory
	level1History := snapshot.Level1History
	flatHistory := snapshot.FlatHistory

	if maxVariableFloats > 0 {
		trimmed := feed.TrimHistoryTails(
			[][]float64{weightedHistory, level1History, flatHistory},
			maxVariableFloats,
		)

		weightedHistory = trimmed[0]
		level1History = trimmed[1]
		flatHistory = trimmed[2]
	}

	samples := []float64{
		snapshot.Weighted,
		snapshot.Level1,
		snapshot.Flat,
		flatOK,
		snapshot.Mid,
		snapshot.Spread,
		snapshot.TouchDepth,
		tradePressure,
		float64(len(weightedHistory)),
		float64(len(level1History)),
		float64(len(flatHistory)),
	}

	samples = append(samples, weightedHistory...)
	samples = append(samples, level1History...)
	samples = append(samples, flatHistory...)

	artifact := datura.Acquire("bookflow-features", datura.Artifact_Type_json)
	artifact.WithRole("features")
	artifact.WithScope(features.scope)
	artifact.WithPayload(feed.EncodePayload(samples...))

	return artifact
}

func (features *Features) Read(p []byte) (int, error) {
	artifact := features.Artifact()

	if artifact == nil {
		return 0, io.EOF
	}

	return feed.ReadFeatureArtifact(p, artifact)
}

func (features *Features) Close() error {
	features.cancel()

	return nil
}

func weightedImbalance(
	bids, asks []market.BookLevel,
	mid, spread float64,
) (float64, bool) {
	if mid <= 0 || spread <= 0 || len(bids) == 0 || len(asks) == 0 {
		return 0, false
	}

	weightedBid := 0.0
	weightedAsk := 0.0

	for _, level := range bids {
		weight := math.Exp(-math.Abs(level.Price-mid) / spread)
		weightedBid += level.Qty * weight
	}

	for _, level := range asks {
		weight := math.Exp(-math.Abs(level.Price-mid) / spread)
		weightedAsk += level.Qty * weight
	}

	total := weightedBid + weightedAsk

	if total <= 0 {
		return 0, false
	}

	return (weightedBid - weightedAsk) / total, true
}

func level1Imbalance(bids, asks []market.BookLevel) (float64, bool) {
	if len(bids) == 0 || len(asks) == 0 {
		return 0, false
	}

	total := bids[0].Qty + asks[0].Qty

	if total <= 0 {
		return 0, false
	}

	return (bids[0].Qty - asks[0].Qty) / total, true
}

func flatImbalance(bids, asks []market.BookLevel) (float64, bool) {
	bidVolume := 0.0
	askVolume := 0.0

	for _, level := range bids {
		bidVolume += level.Qty
	}

	for _, level := range asks {
		askVolume += level.Qty
	}

	total := bidVolume + askVolume

	if total <= 0 {
		return 0, false
	}

	return (bidVolume - askVolume) / total, true
}
