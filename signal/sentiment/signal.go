package sentiment

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/transport"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"
)

/*
Signal is the cross-sectional breadth and leadership perspective. It is ONLY a
living nomagique Number: one self-adapting numeric unit per symbol that maps
incoming ticker streams into cross-sectional cohort dynamics.
*/
type Signal struct {
	ctx    context.Context
	cancel context.CancelFunc
	err    error
	thesis *types.Thesis
	number *nomagique.Number[string]
	work   *transport.Consumer[*types.Symbol]
}

/*
NewSignal constructs the sentiment signal as one living nomagique.Number.
*/
func NewSignal(ctx context.Context, thesis *types.Thesis) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		ctx:    ctx,
		cancel: cancel,
		thesis: thesis,
		number: nomagique.NewNumber[string](temporal.Path),
	}
	signal.work = transport.NewConsumer[*types.Symbol](signal.Name(), signal.consume)
	thesis.Work(types.SourceSentiment).Register(signal.work)

	return signal
}

func (signal *Signal) Name() string           { return string(types.SourceSentiment) }
func (signal *Signal) Error() error           { return signal.err }
func (signal *Signal) Type() types.SourceType { return types.SourceSentiment }

func (signal *Signal) consume() {
	go func() {
		defer func() {
			signal.thesis.Fail(signal.err)
		}()

		for symbol := range signal.thesis.Work(types.SourceSentiment).Drain(
			signal.work, nil,
		) {
			select {
			case <-signal.ctx.Done():
				signal.err = signal.ctx.Err()
				return
			default:
			}

			if symbol == nil {
				continue
			}

			for ticker := range symbol.MarketTickers(
				symbol.TickerConsumers[types.TickerConsumerSentiment],
			) {
				if ticker.Last == nil || ticker.Last.Sign() <= 0 {
					continue
				}

				input := nmtypes.Frame{}
				input.Put(nmtypes.SampleValue, ticker.Last.Float64())
				input.Put(nmtypes.EventTimeSec, float64(ticker.Timestamp.Unix()))
				input.Put(nmtypes.EventTimeNsec, float64(ticker.Timestamp.Nanosecond()))

				_, err := signal.number.Step(symbol.Symbol, input)

				if err != nil {
					signal.err = errnie.Error(errnie.Err(
						errnie.Validation,
						"sentiment: path step failed for "+symbol.Symbol,
						err,
					))
					return
				}

				output, measured, err := algo.CohortSentiment(
					symbol.Symbol, signal.number,
				)

				if err != nil {
					signal.err = errnie.Error(errnie.Err(
						errnie.Validation,
						"sentiment: cohort evaluation failed for "+symbol.Symbol,
						err,
					))
					return
				}

				symbol.AppendMeasurement(signal.measurement(
					symbol.Symbol,
					ticker.Timestamp,
					output,
					measured,
				))
			}
		}
	}()
}

func (signal *Signal) measurement(
	symbol string,
	at time.Time,
	output nmtypes.Frame,
	measured bool,
) *nmtypes.Measurement {
	dimensionless := nmtypes.Descriptor{
		Unit:      nmtypes.UnitDimensionless,
		Timescale: nmtypes.TimescaleInstantaneous,
	}
	measurement := nmtypes.NewMeasurement(
		uuid.NewString(),
		signal.Name(),
		at.UnixNano(),
		at.UnixNano(),
	)

	path, found := signal.number.Project(symbol)

	if found {
		from, _, hasFrom := temporal.PathSample(&path, 0)

		if hasFrom {
			measurement.ObservedFrom = time.Unix(0, from)
			measurement.Horizon = at.Sub(measurement.ObservedFrom)
		}
	}

	change := metricValue(output, algo.SymbolChange, measured)
	breadth := metricValue(output, algo.SymbolBreadth, measured)
	leaderStrength := metricValue(output, algo.SymbolLeaderStrength, measured)
	leaderEvidence := metricValue(output, algo.SymbolLeaderEvidence, measured)
	relativeLead := metricValue(output, algo.SymbolRelativeLead, measured)
	surge := metricValue(output, algo.SymbolSurgeScore, measured)
	slump := metricValue(output, algo.SymbolSlumpScore, measured)
	divergence := metricValue(output, algo.SymbolDivergentScore, measured)
	strength := metricValue(output, algo.SymbolSentimentStrength, measured)
	peerCount := metricValue(output, algo.SymbolCohortPeerCount, measured)

	measurement.AddMetrics(
		nmtypes.NewMetric(string(types.MetricChange), change, dimensionless),
		nmtypes.NewMetric(string(types.MetricBreadth), breadth, dimensionless),
		nmtypes.NewMetric(string(types.MetricLeaderStrength), leaderStrength, dimensionless),
		nmtypes.NewMetric(string(types.MetricLeaderEvidence), leaderEvidence, dimensionless),
		nmtypes.NewMetric(string(types.MetricRelativeLead), relativeLead, dimensionless),
		nmtypes.NewNormalizedMetric(string(types.MetricSurgeScore), surge, surge, dimensionless),
		nmtypes.NewNormalizedMetric(string(types.MetricSlumpScore), slump, slump, dimensionless),
		nmtypes.NewNormalizedMetric(string(types.MetricDivergentScore), divergence, divergence, dimensionless),
		nmtypes.NewNormalizedMetric(string(types.MetricStrength), strength, strength, dimensionless),
	)
	measurement.StampQuality(strength, peerCount)

	return measurement
}

func metricValue(
	frame nmtypes.Frame,
	symbol nmtypes.Symbol,
	measured bool,
) float64 {
	if !measured {
		return 0
	}

	value, found := frame.Get(symbol)

	if found {
		return value
	}

	return 0
}

func (signal *Signal) Close() error {
	if signal.cancel != nil {
		signal.cancel()
	}

	return nil
}
