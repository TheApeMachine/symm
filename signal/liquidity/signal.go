package liquidity

import (
	"context"
	"math"
	"sort"

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
		"book":   {Topic: "thesis", Fn: signal.onBook},
		"trade":  {Topic: "thesis", Fn: signal.onTrade},
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
Initialize wires ticker, book, and trade ingress from Live.
*/
func (signal *Signal) Initialize(live *types.Actor, thesis *types.Thesis) {
	signal.thesis = thesis
	signal.Actor.Initialize(
		types.Topic{Name: "ticker", Actor: live},
		types.Topic{Name: "book", Actor: live},
		types.Topic{Name: "trade", Actor: live},
	)
}

func (signal *Signal) onTicker(message any) any {
	rows := message.(*kraken.Ticker).Data
	measurements, err := signal.Calculate(rows, nil, nil)

	if err != nil {
		errnie.Error(err)
		return nil
	}

	if len(measurements) == 0 {
		return nil
	}

	signal.thesis.Measurements = append(signal.thesis.Measurements, measurements...)

	return signal.thesis
}

func (signal *Signal) onBook(message any) any {
	rows := message.(*kraken.Book).Data
	measurements, err := signal.Calculate(nil, nil, rows)

	if err != nil {
		errnie.Error(err)
		return nil
	}

	if len(measurements) == 0 {
		return nil
	}

	signal.thesis.Measurements = append(signal.thesis.Measurements, measurements...)

	return signal.thesis
}

func (signal *Signal) onTrade(message any) any {
	rows := message.(*kraken.Trade).Data
	measurements, err := signal.Calculate(nil, rows, nil)

	if err != nil {
		errnie.Error(err)
		return nil
	}

	if len(measurements) == 0 {
		return nil
	}

	signal.thesis.Measurements = append(signal.thesis.Measurements, measurements...)

	return signal.thesis
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

	type measurementSpec struct {
		metric     types.MetricType
		unit       types.MeasurementUnit
		raw        float64
		normalized *float64
	}

	out := make([]*types.Measurement, 0, len(peers))

	for _, peer := range peers {
		executableDepth := peer.ExecutableDepth

		if executableDepth <= 0 {
			continue
		}

		validity := types.MeasurementValidity{
			State:     types.ValidityValid,
			Readiness: types.ReadinessObservation,
		}
		scale := types.ScaleReference{
			Kind:    types.ScaleObservationWindow,
			From:    peer.At,
			Through: peer.At,
		}
		specs := []measurementSpec{
			{types.MetricExecutableTouchDepth, types.UnitQuoteCurrency, executableDepth, nil},
		}

		if !peerReady {
			validity.State = types.ValidityProvisional
			validity.Reason = "peer executable-depth median unavailable"
		}

		if peerReady {
			relativeDepth := executableDepth / depthMedian
			scarcity := math.Max(0, 1-relativeDepth)
			specs = []measurementSpec{
				{
					types.MetricExecutableTouchDepth,
					types.UnitQuoteCurrency,
					executableDepth,
					types.NormalizeFinite(relativeDepth),
				},
				{
					types.MetricRelativeTouchDepth,
					types.UnitDimensionless,
					relativeDepth,
					types.NormalizeFinite(relativeDepth),
				},
				{
					types.MetricScarcityScore,
					types.UnitDimensionless,
					scarcity,
					types.NormalizeFinite(scarcity),
				},
				{
					types.MetricExecutableTouchDepthMedian,
					types.UnitQuoteCurrency,
					depthMedian,
					nil,
				},
			}
		}

		reportedNotional := peer.QuoteNotional

		if peerReady && reportedNotional > 0 && hasNotionalMedian && notionalMedian > 0 {
			specs = append(specs,
				measurementSpec{types.MetricReportedVolumeNotional, types.UnitQuoteCurrency, reportedNotional, nil},
				measurementSpec{types.MetricReportedVolumeNotionalMedian, types.UnitQuoteCurrency, notionalMedian, nil},
			)
		}

		for _, spec := range specs {
			out = append(out, &types.Measurement{
				Source:     types.SourceLiquidity,
				Stream:     types.Liquidity,
				Metric:     spec.metric,
				Subject:    types.SubjectPeerLiquidity,
				Symbol:     peer.Symbol,
				At:         peer.At,
				Unit:       spec.unit,
				Raw:        spec.raw,
				Normalized: spec.normalized,
				Maturity:   peerMaturity,
				Validity:   validity,
				Scale:      scale,
			})
		}
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
