package sentiment

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/market/perspectives/types"
	"github.com/theapemachine/symm/numeric"
	"github.com/theapemachine/symm/numeric/adaptive"
	"github.com/theapemachine/symm/rawdump"
	"github.com/theapemachine/symm/ring"
	signalpool "github.com/theapemachine/symm/signal"
)

const (
	sentimentBreadthHistory = 64
	rawSubscriberID         = "signal/sentiment:raw"
)

var sentimentDefaultBandEdges = []float64{-0.10, 0.10}

/*
Signal measures cross-section bullish breadth from ticker change percentages and
maps it onto the sentiment perspective. It is cross-asset: the verdict for one
symbol depends on how much of the universe is green. Confidence is classification
clarity — margin to the surge threshold or leadership boundary; SNR is how
surprising that clarity is versus the symbol's own recent baseline.

| Category        | Cross-section                                  |
|:----------------|:-----------------------------------------------|
| Risk-On Surge   | majority of the universe rising (>= 55%)       |
| Divergent Move  | this symbol leads while breadth is weak        |
| Systemic Slump  | breadth weak and this symbol is not a leader   |
*/
type Signal struct {
	ctx           context.Context
	cancel        context.CancelFunc
	pool          *qpool.Q[any]
	broadcasts    map[string]*qpool.BroadcastGroup
	subscribers   map[string]*qpool.BroadcastConsumer
	symbols       sync.Map // symbol -> float64 (change percent)
	tracked       sync.Map // symbol -> *types.Category
	breadthHist   ring.FloatRing
	surpriseField *types.CategorySurpriseField
	classifier    *adaptive.Classifier
	calibrator    *numeric.BandCalibrator
	rawDump       *rawdump.Writer
}

func NewSignal(ctx context.Context, pool *qpool.Q[any]) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	pooledCalibrator := numeric.NewSignalCalibrator(
		sentimentDefaultBandEdges,
		[]float64{0, 1, 2},
		[]string{"systemic_slump", "divergent_move", "risk_on_surge"},
		[]float64{0.25, 0.35, 0.40},
		numeric.DefaultCalibratorConfig("strength"),
		"sentiment",
	)

	surpriseField, err := types.NewCategorySurpriseField([]types.CategoryType{
		types.CategorySystemicSlump,
		types.CategoryDivergentMove,
		types.CategoryRiskOnSurge,
	}, types.DefaultCategorySurpriseAlpha)

	if err != nil {
		cancel()
		errnie.Error(err, "signal/sentiment")
		return nil
	}

	signal := &Signal{
		ctx:           ctx,
		cancel:        cancel,
		pool:          pool,
		broadcasts:    make(map[string]*qpool.BroadcastGroup),
		subscribers:   make(map[string]*qpool.BroadcastConsumer),
		breadthHist:   ring.NewFloatRing(sentimentBreadthHistory),
		surpriseField: surpriseField,
		classifier:    pooledCalibrator.Classifier,
		calibrator:    pooledCalibrator.Calibrator,
		rawDump:       rawdump.Open("sentiment"),
	}

	for _, channel := range []string{"raw"} {
		signal.broadcasts[channel] = pool.CreateBroadcastGroup(channel, 10*time.Millisecond)
		signal.subscribers[channel] = signal.broadcasts[channel].Subscribe(rawSubscriberID, 1024)
	}

	signal.broadcasts["measurements"] = pool.CreateBroadcastGroup("measurements", 10*time.Millisecond)
	signal.broadcasts["ui"] = pool.CreateBroadcastGroup("ui", 10*time.Millisecond)

	errnie.Info("signal/sentiment ready", "signal/sentiment")

	return signal
}

func (signal *Signal) Tick() error {
	for {
		message := signal.subscribers["raw"].Poll()

		if message == nil {
			continue
		}

		errnie.Debug("signal/sentiment: Tick()", "type", message.Type)

		sm, ok := signalpool.SocketMessageFromValue(message.Value)

		if !ok {
			continue
		}

		switch sm.Channel {
		case public.TickerChannel:
			tickers := signalpool.GetTickers(sm)

			if err := signal.publishTickers(tickers); err != nil {
				errnie.Error(err, "sentiment: publish tickers")
			}
		}
	}
}

/*
sentimentSnapshot is one cross-section read shared by every symbol in a ticker
batch so breadth and leadership thresholds are not recomputed per coin.
*/
type sentimentSnapshot struct {
	breadth         float64
	universe        int
	surgeThreshold  float64
	leaderThreshold float64
}

func (signal *Signal) publishTickers(tickers []market.TickerUpdate) error {
	rows := make([]market.TickerUpdate, 0, len(tickers))

	for _, ticker := range tickers {
		if ticker.Last <= 0 {
			continue
		}

		signal.symbols.Store(ticker.Symbol, ticker.ChangePct)
		rows = append(rows, ticker)
	}

	if len(rows) == 0 {
		return nil
	}

	snapshot, ok := signal.snapshot()

	if !ok {
		return nil
	}

	signal.breadthHist.Push(snapshot.breadth)

	tasks := make([]*qpool.ResultWait[any], 0, len(rows))

	for _, row := range rows {
		tasks = append(tasks, signal.pool.Schedule(fmt.Sprintf("%s:parallel:%p", "sentiment", &tasks), func(ctx context.Context) (any, error) {
			measurement, standout, err := signal.measureFromSnapshot(
				row.Symbol, row.ChangePct, snapshot,
			)

			if err != nil {
				return nil, err
			}

			if measurement.Source == types.SourceNone {
				return nil, nil
			}

			measurement.Symbol = row.Symbol
			measurement.Last = row.Last

			telemetry, _ := numeric.ObserveGaugeTelemetry(
				signal.calibrator,
				signal.classifier,
				measurement.Strength,
				standout,
			)

			if err := types.AssignCategorySurpriseSNR(
				&measurement, signal.surpriseField, measurement.Category,
			); err != nil {
				return nil, err
			}

			if err := signal.rawDump.Write(rawRecord{
				Symbol:     measurement.Symbol,
				Category:   measurement.Category,
				Strength:   measurement.Strength,
				Confidence: measurement.Confidence,
				SNR:        measurement.SNR,
				Standout:   standout,
				Last:       measurement.Last,
				SpreadBPS:  measurement.SpreadBPS,
			}); err != nil {
				return nil, err
			}

			if err := measurement.Send(signal.pool); err != nil {
				return nil, err
			}

			if ui := signal.broadcasts["ui"]; ui != nil {
				ui.Send(&qpool.QValue[any]{
					Value: numeric.GaugePayload(
						measurement.Source.String(),
						measurement.Symbol,
						measurement.Category,
						measurement,
						telemetry,
					),
				})
			}

			return nil, nil
		}))
	}

	var err error

	for _, task := range tasks {
		value, getErr := task.Get(signal.ctx)
		if getErr != nil {
			err = errors.Join(err, getErr)
			continue
		}
		err = errors.Join(err, value.Error)
	}

	return err
}

// measure classifies one symbol against the live cross-section breadth.
func (signal *Signal) measure(symbol string, change float64) (types.Measurement, float64, error) {
	snapshot, ok := signal.snapshot()

	if !ok {
		return types.Measurement{}, 0, nil
	}

	signal.breadthHist.Push(snapshot.breadth)

	return signal.measureFromSnapshot(symbol, change, snapshot)
}

func (signal *Signal) measureFromSnapshot(
	symbol string,
	change float64,
	snapshot sentimentSnapshot,
) (types.Measurement, float64, error) {
	category, confidence := sentimentReading(
		snapshot.breadth,
		change,
		snapshot.surgeThreshold,
		math.Abs(change) >= snapshot.leaderThreshold && change != 0,
	)

	// confidence is which category the breadth selects and how decisively; standout is
	// the strength of the cross-sectional sentiment itself — how far breadth swings
	// from a balanced 0.5 — which SNR scores against this symbol's own history. They
	// are different questions: a lopsided tape can still sit cleanly inside one band,
	// and a near-balanced one can sit right on a boundary. Neutral breadth (0.5) is
	// genuinely no signal, so its standout is 0.
	standout := math.Min(1, math.Abs(snapshot.breadth-0.5)*2)

	trackedRaw, _ := signal.tracked.LoadOrStore(
		symbol,
		types.NewCategory(types.CategoryTypeNone),
	)
	tracked := trackedRaw.(*types.Category)

	if err := tracked.Observe(category, confidence); err != nil {
		return types.Measurement{}, 0, err
	}

	return types.Measurement{
		Source:     types.SourceSentiment,
		Category:   category,
		Strength:   signal.breadthOdds(snapshot.breadth),
		Confidence: confidence,
	}, standout, nil
}

func (signal *Signal) snapshot() (sentimentSnapshot, bool) {
	breadth, _, universe, ok := signal.breadth()

	if !ok {
		return sentimentSnapshot{}, false
	}

	magnitudes := make([]float64, 0, 16)

	signal.symbols.Range(func(_, value any) bool {
		magnitudes = append(magnitudes, math.Abs(value.(float64)))

		return true
	})

	leaderThreshold := 0.0

	if len(magnitudes) >= 2 {
		leaderThreshold = numeric.PercentileSorted(numeric.CopySorted(magnitudes), 0.90)
	}

	return sentimentSnapshot{
		breadth:         breadth,
		universe:        universe,
		surgeThreshold:  signal.surgeThreshold(universe),
		leaderThreshold: leaderThreshold,
	}, true
}

// breadth returns the fraction of the universe that is rising and the strongest
// positive change observed.
func (signal *Signal) breadth() (fraction, topChange float64, universe int, ok bool) {
	positive := 0
	total := 0

	signal.symbols.Range(func(_, value any) bool {
		change := value.(float64)

		if change == 0 {
			return true
		}

		total++

		if change > topChange {
			topChange = change
		}

		if change > 0 {
			positive++
		}

		return true
	})

	if total == 0 {
		return 0, 0, 0, false
	}

	return float64(positive) / float64(total), topChange, total, true
}

// category maps breadth and this symbol's leadership onto the sentiment perspective.
func (signal *Signal) category(breadth, change float64, universe int) types.CategoryType {
	signal.breadthHist.Push(breadth)
	category, _ := sentimentReading(
		breadth, change, signal.surgeThreshold(universe), signal.isLeader(change),
	)

	return category
}

func (signal *Signal) surgeThreshold(universe int) float64 {
	samples := signal.breadthHist.Ordered()

	if len(samples) >= 8 {
		return numeric.PercentileSorted(numeric.CopySorted(samples), 0.75)
	}

	if universe <= 0 {
		return 0.5
	}

	return 0.5 + 0.5/float64(universe)
}

func (signal *Signal) isLeader(change float64) bool {
	if change == 0 {
		return false
	}

	magnitudes := make([]float64, 0, 16)

	signal.symbols.Range(func(_, value any) bool {
		magnitudes = append(magnitudes, math.Abs(value.(float64)))

		return true
	})

	if len(magnitudes) < 2 {
		return math.Abs(change) > 0
	}

	threshold := numeric.PercentileSorted(numeric.CopySorted(magnitudes), 0.90)

	return math.Abs(change) >= threshold
}

// breadthOdds is the decisiveness of the breadth split — its odds away from 50/50.
func (signal *Signal) breadthOdds(breadth float64) float64 {
	if breadth <= 0 || breadth >= 1 {
		return 1
	}

	if breadth >= 0.5 {
		return breadth / (1 - breadth)
	}

	return (1 - breadth) / breadth
}

func (signal *Signal) Close() error {
	signal.cancel()
	return signal.rawDump.Close()
}
