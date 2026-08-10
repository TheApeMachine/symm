package liquidity

import (
	"context"
	"math"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"golang.org/x/sync/errgroup"

	"github.com/theapemachine/nomagique/statistic"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
)

/*
Signal is the Scarcity perspective, identifying opportunities where current
executable touch depth is thin relative to peers. Reported-volume notional is
retained as a separate turnover context and never mixed into the book-depth score.
*/
type Signal struct {
	status       atomic.Value
	ctx          context.Context
	cancel       context.CancelFunc
	api          *websocket.API
	ui           chan []byte
	thesis       *types.Thesis
	semaphore    chan struct{}
	observations *sync.Map
}

type liquidityObservation struct {
	at              time.Time
	bid             float64
	ask             float64
	bidQuantity     float64
	askQuantity     float64
	reportedPrice   float64
	reportedVolume  float64
	executableDepth float64
	quoteNotional   float64
	cadence         time.Duration
}

const minimumLiquidityCohort = 3

/*
NewSignal creates liquidity measurement state for central market cuts so each
tick can compare executable liquidity across the observed cohort.
*/
func NewSignal(
	ctx context.Context,
	api *websocket.API,
	ui chan []byte,
	thesis *types.Thesis,
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		ctx:          ctx,
		cancel:       cancel,
		api:          api,
		ui:           ui,
		thesis:       thesis,
		semaphore:    make(chan struct{}, 1),
		observations: &sync.Map{},
	}

	signal.status.Store(types.INITIALIZING)
	signal.thesis.Subscribe(types.SourceLiquidity, signal.semaphore)
	signal.status.Store(types.READY)
	signal.run()

	return signal
}

/*
Name returns the signal source identity.
*/
func (signal *Signal) Name() string {
	return string(types.SourceLiquidity)
}

func (signal *Signal) Status() types.Status {
	return signal.status.Load().(types.Status)
}

func (signal *Signal) run() {
	go func() {
		for {
			select {
			case <-signal.ctx.Done():
				return
			case <-signal.semaphore:
				signal.status.Store(types.BUSY)
				measurements := signal.Measure(signal.thesis)

				if len(measurements) > 0 {
					signal.thesis.AppendMeasurements(
						types.SourceLiquidity, measurements, true,
					)
				}

				signal.status.Store(types.READY)
			}
		}
	}()
}

/*
Measure produces the Measurements for the liquidity signal.
*/
func (signal *Signal) Measure(thesis *types.Thesis) []*types.Measurement {
	tickers := thesis.MarketTickers(types.SourceLiquidity)

	if !signal.ingest(tickers) {
		return nil
	}

	peers, _, cadenceReady := signal.cohort()

	if len(peers) == 0 {
		return nil
	}

	sort.Slice(peers, func(left, right int) bool {
		return peers[left].symbol < peers[right].symbol
	})

	cohortDepthMedian, depthCohortReady := liquidityCohortMedian(peers, true)
	cohortNotionalMedian, notionalCohortReady := liquidityCohortMedian(peers, false)

	measurements := make([]*types.Measurement, len(peers))
	out := make([]*types.Measurement, 0)

	group, _ := errgroup.WithContext(signal.ctx)

	for index, peer := range peers {
		measurementIndex := index

		group.Go(func() error {
			updated := false

			for _, ticker := range tickers {
				if strings.TrimSpace(ticker.Symbol) == peer.symbol &&
					ticker.Timestamp.Equal(peer.observation.at) {
					updated = true
					break
				}
			}

			if !updated {
				return nil
			}

			executableDepth := peer.observation.executableDepth
			depthPeers, notionalPeers := leaveOneOutLiquidity(peer.symbol, peers)
			depthMedian, depthOK := statistic.MedianOf(depthPeers)
			peerReady := len(depthPeers) >= 2 && depthOK && depthMedian > 0
			notionalMedian, hasNotionalMedian := statistic.MedianOf(notionalPeers)
			reportedNotional := peer.observation.quoteNotional
			reportedReady := len(notionalPeers) >= 2 && hasNotionalMedian &&
				notionalMedian > 0 && reportedNotional > 0 && notionalCohortReady

			relativeDepth := 0.0
			scarcity := 0.0
			median := 0.0

			if peerReady && executableDepth > 0 {
				relativeDepth = executableDepth / depthMedian
				median = depthMedian
				deficit := math.Max(0, depthMedian-executableDepth)

				if deficit > 0 {
					deviations := absoluteDeviations(depthPeers, depthMedian)
					dispersion, _ := statistic.MedianOf(deviations)
					scarcity = deficit / (deficit + dispersion)
				}
			}

			reportedMedian := 0.0

			if hasNotionalMedian && notionalMedian > 0 {
				reportedMedian = notionalMedian
			}

			normalizedDepth := normalizedLiquidityRatio(executableDepth, depthMedian)
			normalizedRelativeDepth := normalizedRelativeLiquidity(relativeDepth)
			normalizedScarcity := normalizedLiquidityScore(scarcity)
			normalizedDepthMedian := normalizedLiquidityRatio(
				depthMedian,
				cohortDepthMedian,
			)
			normalizedReportedNotional := normalizedLiquidityRatio(
				reportedNotional,
				notionalMedian,
			)
			normalizedReportedMedian := normalizedLiquidityRatio(
				notionalMedian,
				cohortNotionalMedian,
			)
			maturity := 0.0

			if cadenceReady && peerReady && depthCohortReady && reportedReady {
				maturity = 1
			}

			measurement := &types.Measurement{
				ID:       uuid.NewString(),
				Source:   types.SourceLiquidity,
				Symbol:   peer.symbol,
				At:       peer.observation.at,
				Maturity: maturity,
				Metrics: map[string]types.MetricSample{
					types.MetricKey(types.MetricBestPrice, types.SideBuy): {
						Raw:  peer.observation.bid,
						Unit: types.UnitQuoteCurrency,
					},
					types.MetricKey(types.MetricBestPrice, types.SideSell): {
						Raw:  peer.observation.ask,
						Unit: types.UnitQuoteCurrency,
					},
					types.MetricKey(types.MetricTouchQuantity, types.SideBuy): {
						Raw:  peer.observation.bidQuantity,
						Unit: types.UnitBaseCurrency,
					},
					types.MetricKey(types.MetricTouchQuantity, types.SideSell): {
						Raw:  peer.observation.askQuantity,
						Unit: types.UnitBaseCurrency,
					},
					types.MetricKey(types.MetricMidpoint, types.SideNone): {
						Raw:  (peer.observation.bid + peer.observation.ask) / 2,
						Unit: types.UnitQuoteCurrency,
					},
					types.MetricKey(types.MetricVWAP, types.SideNone): {
						Raw:  peer.observation.reportedPrice,
						Unit: types.UnitQuoteCurrency,
					},
					types.MetricKey(types.MetricReportedVolume, types.SideNone): {
						Raw:  peer.observation.reportedVolume,
						Unit: types.UnitBaseCurrency,
					},
					types.MetricKey(types.MetricExecutableTouchDepth, types.SideNone): {
						Raw:        executableDepth,
						Normalized: normalizedDepth,
						Unit:       types.UnitQuoteCurrency,
					},
					types.MetricKey(types.MetricRelativeTouchDepth, types.SideNone): {
						Raw:        relativeDepth,
						Normalized: normalizedRelativeDepth,
						Unit:       types.UnitDimensionless,
					},
					types.MetricKey(types.MetricScarcityScore, types.SideNone): {
						Raw:        scarcity,
						Normalized: normalizedScarcity,
						Unit:       types.UnitDimensionless,
					},
					types.MetricKey(types.MetricExecutableTouchDepthMedian, types.SideNone): {
						Raw:        median,
						Normalized: normalizedDepthMedian,
						Unit:       types.UnitQuoteCurrency,
					},
					types.MetricKey(types.MetricReportedVolumeNotional, types.SideNone): {
						Raw:        reportedNotional,
						Normalized: normalizedReportedNotional,
						Unit:       types.UnitQuoteCurrency,
					},
					types.MetricKey(types.MetricReportedVolumeNotionalMedian, types.SideNone): {
						Raw:        reportedMedian,
						Normalized: normalizedReportedMedian,
						Unit:       types.UnitQuoteCurrency,
					},
				},
			}

			measurements[measurementIndex] = measurement

			return nil
		})
	}

	if err := group.Wait(); err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"liquidity: parallel measurement failed",
			err,
		))
		return nil
	}

	compacted := measurements[:0]

	for _, measurement := range measurements {
		if measurement == nil {
			continue
		}

		compacted = append(compacted, measurement)

		if measurement.Symbol == types.Focus() {
			out = append(out, measurement)
		}
	}
	measurements = compacted

	if len(out) > 0 {
		utils.Publish(signal.ui, datura.NewMap(
			"measurements", out,
		))
	}

	return measurements
}

/*
normalizedRelativeLiquidity validates a ratio that already carries its real
leave-one-out executable-depth denominator.
*/
func normalizedRelativeLiquidity(raw float64) *float64 {
	value := raw

	return &value
}

/*
liquidityCohortMedian derives the common cross-sectional scale used to make
each leave-one-out median itself comparable. It reads only the current cohort;
no normalization history is retained here.
*/
func liquidityCohortMedian(
	peers []liquidityPeer,
	depth bool,
) (float64, bool) {
	values := make([]float64, 0, len(peers))

	for _, peer := range peers {
		value := peer.observation.quoteNotional

		if depth {
			value = peer.observation.executableDepth
		}

		if value > 0 && !math.IsNaN(value) && !math.IsInf(value, 0) {
			values = append(values, value)
		}
	}

	median, ready := statistic.MedianOf(values)

	return median, ready && len(values) >= minimumLiquidityCohort && median > 0
}

/*
normalizedLiquidityRatio uses a positive executable cohort baseline. Zero is
accepted only as a measured numerator; a missing or malformed scale stays nil.
*/
func normalizedLiquidityRatio(raw, baseline float64) *float64 {
	value := 1.0

	if baseline != 0 {
		value = raw / baseline
	}

	return &value
}

func normalizedLiquidityScore(raw float64) *float64 {
	value := raw

	return &value
}

type liquidityPeer struct {
	symbol      string
	observation liquidityObservation
}

func (signal *Signal) ingest(rows []kraken.TickerData) bool {
	if signal.observations == nil {
		signal.observations = &sync.Map{}
	}

	rowBatches := make(map[string][]kraken.TickerData)
	symbols := make([]string, 0)

	for _, row := range rows {
		symbol := strings.TrimSpace(row.Symbol)

		if symbol == "" || row.Timestamp.IsZero() {
			continue
		}

		if _, exists := rowBatches[symbol]; !exists {
			symbols = append(symbols, symbol)
		}

		row.Symbol = symbol
		rowBatches[symbol] = append(rowBatches[symbol], row)
	}

	sort.Strings(symbols)
	changed := &sync.Map{}

	group, _ := errgroup.WithContext(signal.ctx)

	for _, symbol := range symbols {
		symbolRows := rowBatches[symbol]
		sort.SliceStable(symbolRows, func(leftIndex, rightIndex int) bool {
			return symbolRows[leftIndex].Timestamp.Before(symbolRows[rightIndex].Timestamp)
		})

		group.Go(func() error {
			for _, row := range symbolRows {
				raw, exists := signal.observations.Load(symbol)
				previous := liquidityObservation{}

				if exists {
					previous = raw.(liquidityObservation)
				}

				if exists && !row.Timestamp.After(previous.at) {
					continue
				}

				bid, ask, bidQuantity, askQuantity := tickerTouch(row)
				reportedPrice, reportedVolume := reportedTurnover(row)
				observation := liquidityObservation{
					at:              row.Timestamp,
					bid:             bid,
					ask:             ask,
					bidQuantity:     bidQuantity,
					askQuantity:     askQuantity,
					reportedPrice:   reportedPrice,
					reportedVolume:  reportedVolume,
					executableDepth: executableDepth(row),
					quoteNotional:   quoteNotional(row),
				}

				if exists {
					observation.cadence = row.Timestamp.Sub(previous.at)
				}

				signal.observations.Store(symbol, observation)
				changed.Store(symbol, struct{}{})
			}

			return nil
		})
	}

	if err := group.Wait(); err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"liquidity: parallel ingestion failed",
			err,
		))
		return false
	}

	anyChanged := false
	changed.Range(func(key, value any) bool {
		anyChanged = true
		return false
	})

	return anyChanged
}

func (signal *Signal) cohort() ([]liquidityPeer, time.Duration, bool) {
	latest := time.Time{}
	cadences := make([]float64, 0)

	signal.observations.Range(func(key, value any) bool {
		observation := value.(liquidityObservation)

		if observation.at.After(latest) {
			latest = observation.at
		}

		if observation.cadence > 0 {
			cadences = append(cadences, float64(observation.cadence))
		}

		return true
	})

	medianCadence, cadenceReady := statistic.MedianOf(cadences)
	freshness := time.Duration(medianCadence)
	peers := make([]liquidityPeer, 0)

	signal.observations.Range(func(key, value any) bool {
		symbol := key.(string)
		observation := value.(liquidityObservation)

		if cadenceReady && freshness > 0 && latest.Sub(observation.at) > freshness {
			return true
		}

		peers = append(peers, liquidityPeer{symbol: symbol, observation: observation})
		return true
	})

	return peers, freshness, cadenceReady && freshness > 0
}

func leaveOneOutLiquidity(
	symbol string,
	peers []liquidityPeer,
) ([]float64, []float64) {
	depths := make([]float64, 0, len(peers)-1)
	notionals := make([]float64, 0, len(peers)-1)

	for _, peer := range peers {
		if peer.symbol == symbol {
			continue
		}

		if peer.observation.executableDepth > 0 {
			depths = append(depths, peer.observation.executableDepth)
		}

		if peer.observation.quoteNotional > 0 {
			notionals = append(notionals, peer.observation.quoteNotional)
		}
	}

	return depths, notionals
}

func absoluteDeviations(values []float64, center float64) []float64 {
	deviations := make([]float64, 0, len(values))

	for _, value := range values {
		deviations = append(deviations, math.Abs(value-center))
	}

	return deviations
}

func executableDepth(row kraken.TickerData) float64 {
	if row.Bid == nil || row.Ask == nil || row.BidQty <= 0 || row.AskQty <= 0 {
		return 0
	}

	bid := row.Bid.Float64()
	ask := row.Ask.Float64()

	if bid <= 0 || ask <= bid || math.IsNaN(bid) || math.IsNaN(ask) ||
		math.IsInf(bid, 0) || math.IsInf(ask, 0) {
		return 0
	}

	return math.Min(row.BidQty, row.AskQty) * (bid + ask) / 2
}

func quoteNotional(row kraken.TickerData) float64 {
	price, volume := reportedTurnover(row)

	return price * volume
}

func tickerTouch(row kraken.TickerData) (float64, float64, float64, float64) {
	if row.Bid == nil || row.Ask == nil || row.BidQty <= 0 || row.AskQty <= 0 {
		return 0, 0, 0, 0
	}

	return row.Bid.Float64(), row.Ask.Float64(), row.BidQty, row.AskQty
}

func reportedTurnover(row kraken.TickerData) (float64, float64) {
	price := row.Vwap

	if price <= 0 && row.Last != nil {
		price = row.Last.Float64()
	}

	if price <= 0 || row.Volume <= 0 || math.IsNaN(price) || math.IsNaN(row.Volume) ||
		math.IsInf(price, 0) || math.IsInf(row.Volume, 0) {
		return 0, 0
	}

	return price, row.Volume
}

/*
Close releases the receiver's owned resources so shutdown does not leave
active market-data producers.
*/
func (signal *Signal) Close() error {
	if signal.cancel != nil {
		signal.cancel()
	}

	return nil
}
