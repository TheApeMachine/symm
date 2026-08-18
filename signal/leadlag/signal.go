package leadlag

import (
	"context"
	"iter"
	"math"
	"sort"

	"github.com/google/uuid"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

/*
Signal is the Anchor perspective, measuring temporal correlation between the
cross-section leader and each follower. Categories belong in logic; this signal
emits numerical scores only.
*/
type Signal struct {
	ctx     context.Context
	cancel  context.CancelFunc
	api     *websocket.API
	section *Section
	lag     *algorithm.Lag
}

/*
NewSignal creates lead-lag measurement state for central market cuts so
temporal relationships persist across Thesis ticks.
*/
func NewSignal(
	ctx context.Context,
	api *websocket.API,
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		ctx:     ctx,
		cancel:  cancel,
		api:     api,
		section: NewSection(),
		lag:     algorithm.NewLag(),
	}

	return signal
}

/*
Name returns the signal source identity.
*/
func (signal *Signal) Name() string {
	return string(types.SourceLeadLag)
}

func (signal *Signal) Type() types.SourceType {
	return types.SourceLeadLag
}

func (signal *Signal) Measure(
	symbol *types.Symbol,
	ticks ...int64,
) iter.Seq[*types.Measurement] {
	if symbol == nil {
		return func(yield func(*types.Measurement) bool) {}
	}

	return signal.MeasureCohort([]*types.Symbol{symbol}, ticks...)
}

/*
MeasureCohort keeps the lead-lag section under one owner and merges the dirty
symbol queues into a deterministic event-time order. The anchor is selected
from strictly prior section state, then the complete arrival is applied.
*/
func (signal *Signal) MeasureCohort(
	symbols []*types.Symbol,
	ticks ...int64,
) iter.Seq[*types.Measurement] {
	return func(yield func(*types.Measurement) bool) {
		anchor := signal.section.CausalAnchor()

		if anchor == "" {
			signal.section.ClearAnchor()
		}

		if anchor != "" {
			signal.section.SetAnchor(anchor)
		}

		ordered := make([]*types.Symbol, 0, len(symbols))

		for _, symbol := range symbols {
			if symbol != nil && symbol.Symbol != "" {
				ordered = append(ordered, symbol)
			}
		}

		sort.Slice(ordered, func(left, right int) bool {
			return ordered[left].Symbol < ordered[right].Symbol
		})

		tickers := make([]kraken.TickerData, 0)

		for _, symbol := range ordered {
			for ticker := range symbol.MarketTickers(types.SourceLeadLag) {
				if ticker.Timestamp.IsZero() || ticker.Symbol == "" || ticker.Last == nil {
					continue
				}

				if ticker.Last.Float64() <= 0 {
					continue
				}

				tickers = append(tickers, ticker)
			}
		}

		tick := int64(0)

		if len(ticks) > 0 {
			tick = ticks[0]
		}

		sort.SliceStable(tickers, func(leftIndex, rightIndex int) bool {
			left := tickers[leftIndex]
			right := tickers[rightIndex]

			if left.Timestamp.Equal(right.Timestamp) {
				return left.Symbol < right.Symbol
			}

			return left.Timestamp.Before(right.Timestamp)
		})

		for _, ticker := range tickers {
			if !signal.section.ObservePrice(ticker.Symbol, ticker.Last.Float64(), ticker.Timestamp) {
				continue
			}

			features := signal.section.Features(ticker.Symbol)

			if features.Price <= 0 {
				continue
			}

			if features.IsAnchor && (!features.MoveReady || features.MoveMoved || features.StallMargin <= 0) {
				continue
			}

			if !features.IsAnchor && features.SampleCount <= 0 {
				continue
			}

			if !features.IsAnchor && !features.MoveReady && !features.ContempOK {
				continue
			}

			if !features.IsAnchor && features.MoveReady && features.MoveMoved &&
				!features.LagOK && !features.ContempOK {
				continue
			}

			if !features.IsAnchor && features.MoveReady && !features.MoveMoved &&
				!features.ContempOK && features.StallMargin <= 0 {
				continue
			}

			outcome, err := signal.lag.Measure(algorithm.LagInput{
				IsAnchor:    features.IsAnchor,
				Price:       features.Price,
				MoveReady:   features.MoveReady,
				MoveMoved:   features.MoveMoved,
				StallMargin: features.StallMargin,
				LagOK:       features.LagOK,
				LagBars:     features.LagBars,
				LagCorr:     features.LagCorr,
				ContempOK:   features.ContempOK,
				ContempCorr: features.ContempCorr,
				SampleCount: features.SampleCount,
			})

			if err != nil {
				errnie.Error(errnie.Err(
					errnie.UnprocessableContent,
					"leadlag: failed to measure lag",
					err,
				))

				continue
			}

			peer := ""

			if signal.section.AnchorSymbol() != "" && signal.section.AnchorSymbol() != ticker.Symbol {
				peer = signal.section.AnchorSymbol()
			}

			contempCorrelation := 0.0

			if features.ContempOK {
				contempCorrelation = features.ContempCorr
			}

			lagCorrelation := 0.0
			lagFraction := 0.0
			lagDirection := 0.0

			if features.LagOK {
				lagCorrelation = features.LagCorr
				maxLagBars := signal.section.maxLagBars(features.SampleCount)

				if maxLagBars > 0 {
					lagFraction = math.Abs(float64(features.LagBars)) / float64(maxLagBars)
				}

				if features.LagBars > 0 {
					lagDirection = 1
				}

				if features.LagBars < 0 {
					lagDirection = -1
				}
			}

			signedCorrelation := contempCorrelation

			if outcome.InefficientScore > 0 {
				signedCorrelation = lagCorrelation
			}

			correlation := math.Abs(signedCorrelation)
			sampleCount := float64(features.SampleCount)

			metrics := map[string]types.MetricSample{
				types.MetricKey(types.MetricCorrelation, types.SideNone): {
					Raw:        correlation,
					Normalized: &correlation,
					Unit:       types.UnitDimensionless,
				},
				types.MetricKey(types.MetricSignedCorrelation, types.SideNone): {
					Raw:        signedCorrelation,
					Normalized: &signedCorrelation,
					Unit:       types.UnitDimensionless,
				},
				types.MetricKey(types.MetricSignedContempCorrelation, types.SideNone): {
					Raw:        contempCorrelation,
					Normalized: &contempCorrelation,
					Unit:       types.UnitDimensionless,
				},
				types.MetricKey(types.MetricSignedLagCorrelation, types.SideNone): {
					Raw:        lagCorrelation,
					Normalized: &lagCorrelation,
					Unit:       types.UnitDimensionless,
				},
				types.MetricKey(types.MetricLagFraction, types.SideNone): {
					Raw:        lagFraction,
					Normalized: &lagFraction,
					Unit:       types.UnitDimensionless,
				},
				types.MetricKey(types.MetricSignedLagDirection, types.SideNone): {
					Raw:        lagDirection,
					Normalized: &lagDirection,
					Unit:       types.UnitDimensionless,
				},
				types.MetricKey(types.MetricSampleCount, types.SideNone): {
					Raw:  sampleCount,
					Unit: types.UnitCount,
				},
				types.MetricKey(types.MetricInefficient, types.SideNone): {
					Raw:        outcome.InefficientScore,
					Normalized: &outcome.InefficientScore,
					Unit:       types.UnitDimensionless,
				},
				types.MetricKey(types.MetricSync, types.SideNone): {
					Raw:        outcome.SyncScore,
					Normalized: &outcome.SyncScore,
					Unit:       types.UnitDimensionless,
				},
				types.MetricKey(types.MetricDecoupled, types.SideNone): {
					Raw:        outcome.DecoupledScore,
					Normalized: &outcome.DecoupledScore,
					Unit:       types.UnitDimensionless,
				},
				types.MetricKey(types.MetricStall, types.SideNone): {
					Raw:        outcome.StallScore,
					Normalized: &outcome.StallScore,
					Unit:       types.UnitDimensionless,
				},
				types.MetricKey(types.MetricStrength, types.SideNone): {
					Raw:        outcome.Strength,
					Normalized: &outcome.Strength,
					Unit:       types.UnitDimensionless,
				},
				types.MetricKey(types.MetricLastPrice, types.SideNone): {
					Raw:  ticker.Last.Float64(),
					Unit: types.UnitQuoteCurrency,
				},
			}

			separation, separationReady := types.MeasurementHypothesisSeparation(
				types.SourceLeadLag,
				metrics,
			)
			separationSample := types.MetricSample{
				Raw:  separation,
				Unit: types.UnitDimensionless,
			}

			if separationReady {
				separationSample.Normalized = &separation
			}

			metrics[types.MetricKey(types.MetricHypothesisSeparation, types.SideNone)] = separationSample
			measurement := &types.Measurement{
				ID:     uuid.NewString(),
				Source: types.SourceLeadLag,
				Symbol: ticker.Symbol,
				Peer:   peer,
				Tick:   tick,
				At:     ticker.Timestamp,
				Metadata: map[string]float64{
					"last_price": ticker.Last.Float64(),
				},
				Metrics: metrics,
			}

			if !features.ObservedFrom.IsZero() {
				if features.ObservedFrom.After(ticker.Timestamp) {
					panic("leadlag: observation interval runs backward")
				}

				measurement.ObservedFrom = features.ObservedFrom
				measurement.Horizon = ticker.Timestamp.Sub(features.ObservedFrom)
			}

			if measurement.Peer != "" {
				if features.PeerPrice <= 0 || features.PeerAt.IsZero() ||
					features.PeerFrom.IsZero() ||
					features.PeerFrom.After(features.PeerAt) {
					panic("leadlag: peer observation is incomplete")
				}

				measurement.PeerAt = features.PeerAt
				measurement.PeerObservedFrom = features.PeerFrom
				measurement.PutMarketValue("peer_last_price", features.PeerPrice)
				measurement.PutMetric(
					types.MetricPeerLastPrice,
					types.SideNone,
					types.MetricSample{
						Raw:  features.PeerPrice,
						Unit: types.UnitQuoteCurrency,
					},
				)
			}

			if !yield(measurement) {
				return
			}
		}
	}
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
