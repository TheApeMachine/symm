package liquidity

import (
	"context"
	"math"
	"sort"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"

	"github.com/theapemachine/nomagique/statistic"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

/*
Signal is the Scarcity perspective, identifying opportunities where current
executable touch depth is thin relative to peers. Reported-volume notional is
retained as a separate turnover context and never mixed into the book-depth score.
*/
type Signal struct {
	*types.Actor
	thesis       *types.Thesis
	ctx          context.Context
	cancel       context.CancelFunc
	ui           chan []byte
	crossSection *types.CrossSection
}

/*
NewSignal creates liquidity measurement state for central market cuts so each
tick can compare executable liquidity across the observed cohort.
*/
func NewSignal(ctx context.Context, ui chan []byte) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		ctx:          ctx,
		cancel:       cancel,
		ui:           ui,
		crossSection: types.NewCrossSection(),
	}

	signal.Actor = types.NewActor(ctx, map[string]types.Handler{
		"ticker": {Topic: "thesis", Fn: signal.onTicker},
	})

	return signal
}

/*
Name returns the signal source identity.
*/
func (signal *Signal) Name() string {
	return string(types.SourceLiquidity)
}

/*
Initialize wires ticker ingress from Live. Liquidity scores come from ticker
cross-section only; book and trade floods must not fill unused buffers.
*/
func (signal *Signal) Initialize(live *types.Actor, thesis *types.Thesis) {
	signal.thesis = thesis
	signal.Actor.Initialize(
		types.Topic{Name: "ticker", Actor: live},
	)
}

func (signal *Signal) onTicker(message any) any {
	rows := message.(*kraken.Ticker).Data
	measurements, err := signal.Calculate(rows, nil, nil)

	if err != nil {
		errnie.Error(err)
		return types.SignalResult{Source: types.SourceLiquidity, Status: types.SignalSkip}
	}

	if len(measurements) > 0 {
		signal.thesis.Publish(types.SourceLiquidity, measurements)
		return types.SignalResult{Source: types.SourceLiquidity, Measurements: measurements, Status: types.SignalReady}
	}

	return types.SignalResult{Source: types.SourceLiquidity, Status: types.SignalSkip}
}

func (signal *Signal) Calculate(
	tickers []kraken.TickerData,
	trades []kraken.TradeData,
	books []kraken.BookData,
) ([]*types.Measurement, error) {
	if len(tickers) == 0 {
		return nil, nil
	}

	// Retain the full observed cohort so an isolated single-symbol event still
	// reports every peer's latest executable liquidity in the same central cut.
	signal.crossSection.Measure(tickers)

	peers := make([]types.SymbolMetric, 0)
	notionalPeers := make([]float64, 0)
	depthPeers := make([]float64, 0)

	signal.crossSection.Metrics.Range(func(_, value any) bool {
		metric := value.(types.SymbolMetric)
		peers = append(peers, metric)

		if metric.QuoteNotional > 0 {
			notionalPeers = append(notionalPeers, metric.QuoteNotional)
		}

		if metric.ExecutableDepth > 0 {
			depthPeers = append(depthPeers, metric.ExecutableDepth)
		}

		return true
	})

	sort.Slice(peers, func(left, right int) bool {
		return peers[left].Symbol < peers[right].Symbol
	})

	depthMedian, depthOK := statistic.MedianOf(depthPeers)
	peerReady := len(depthPeers) >= 2 && depthOK && depthMedian > 0
	notionalMedian, hasNotionalMedian := statistic.MedianOf(notionalPeers)
	peerMaturity := float64(len(depthPeers)) / float64(len(depthPeers)+1)

	out := make([]*types.Measurement, 0, len(peers))

	for _, peer := range peers {
		executableDepth := peer.ExecutableDepth
		validity := types.MeasurementValidity{
			State:     types.ValidityValid,
			Readiness: types.ReadinessObservation,
		}
		scale := types.ScaleReference{
			Kind:    types.ScaleObservationWindow,
			From:    peer.At,
			Through: peer.At,
		}

		if !peerReady || executableDepth <= 0 {
			validity.State = types.ValidityProvisional

			if executableDepth <= 0 {
				validity.Reason = "executable touch depth unavailable"
			}

			if !peerReady {
				if validity.Reason != "" {
					validity.Reason += "; peer executable-depth median unavailable"
				}

				if validity.Reason == "" {
					validity.Reason = "peer executable-depth median unavailable"
				}
			}
		}

		relativeDepth := 0.0
		scarcity := 0.0
		median := 0.0

		if peerReady && executableDepth > 0 {
			relativeDepth = executableDepth / depthMedian
			scarcity = math.Max(0, 1-relativeDepth)
			median = depthMedian
		}

		reportedNotional := peer.QuoteNotional
		reportedMedian := 0.0

		if hasNotionalMedian && notionalMedian > 0 {
			reportedMedian = notionalMedian
		}

		measurement := &types.Measurement{
			Source:   types.SourceLiquidity,
			Symbol:   peer.Symbol,
			At:       peer.At,
			Maturity: peerMaturity,
			Validity: validity,
			Scale:    scale,
			Metrics:  make(map[string]types.MetricSample, 6),
		}

		measurement.Metrics[types.MetricKey(types.MetricExecutableTouchDepth, types.SideNone)] = types.MetricSample{
			Raw:        executableDepth,
			Normalized: types.NormalizeFinite(relativeDepth),
			Unit:       types.UnitQuoteCurrency,
		}
		measurement.Metrics[types.MetricKey(types.MetricRelativeTouchDepth, types.SideNone)] = types.MetricSample{
			Raw:        relativeDepth,
			Normalized: types.NormalizeFinite(relativeDepth),
			Unit:       types.UnitDimensionless,
		}
		measurement.Metrics[types.MetricKey(types.MetricScarcityScore, types.SideNone)] = types.MetricSample{
			Raw:        scarcity,
			Normalized: types.NormalizeFinite(scarcity),
			Unit:       types.UnitDimensionless,
		}
		measurement.Metrics[types.MetricKey(types.MetricExecutableTouchDepthMedian, types.SideNone)] = types.MetricSample{
			Raw:  median,
			Unit: types.UnitQuoteCurrency,
		}
		measurement.Metrics[types.MetricKey(types.MetricReportedVolumeNotional, types.SideNone)] = types.MetricSample{
			Raw:  reportedNotional,
			Unit: types.UnitQuoteCurrency,
		}
		measurement.Metrics[types.MetricKey(types.MetricReportedVolumeNotionalMedian, types.SideNone)] = types.MetricSample{
			Raw:  reportedMedian,
			Unit: types.UnitQuoteCurrency,
		}

		out = append(out, measurement)
	}

	select {
	case signal.ui <- datura.Map[any]{
		"measurements": out,
	}.Marshal():
	default:
		errnie.Error(errnie.Err(
			errnie.TooManyRequests,
			"wire: ui channel saturated; dropped measurements",
			nil,
		))
	}

	return out, nil
}

/*
Close releases the receiver's owned resources so shutdown does not leave
active market-data producers.
*/
func (signal *Signal) Close() error {
	signal.cancel()
	return nil
}
