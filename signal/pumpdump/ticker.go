package pumpdump

import (
	"context"
	"fmt"
	"github.com/theapemachine/errnie"
	"iter"
	"math"
	"strconv"
	"unsafe"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/transport"
)

type tickerInput struct {
	Bid float64
	Ask float64
}

type tickerResult struct {
	Bid            float64
	Ask            float64
	Midpoint       float64
	Spread         float64
	RelativeSpread float64
	SpreadRatio    float64
	Divergence     float64
	Reading        adaptive.BaselineReading
}

type tickerPipeline struct {
	*core.PrimitiveError
	baseline core.Primitive
	out      tickerResult
}

func newTickerPipeline() core.Primitive {
	return &tickerPipeline{
		PrimitiveError: core.NewPrimitiveError(),
		baseline:       adaptive.NewBaseline(adaptive.NewWindow()),
	}
}

func (op *tickerPipeline) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			input := (*tickerInput)(arriving)

			midpoint := (input.Bid + input.Ask) / 2.0
			spread := input.Ask - input.Bid
			relativeSpread := 0.0

			if midpoint > 0 {
				relativeSpread = spread / midpoint
			}

			var reading adaptive.BaselineReading
			for rPtr := range op.baseline.Next(transport.NewOne(unsafe.Pointer(&relativeSpread)).Next(nil)) {
				reading = *(*adaptive.BaselineReading)(rPtr)
			}

			spreadRatio := 1.0
			divergence := 0.0

			if reading.Baseline > 0 {
				spreadRatio = relativeSpread / reading.Baseline
				if relativeSpread > 0 {
					divergence = math.Log(relativeSpread / reading.Baseline)
				}
			}

			op.out = tickerResult{
				Bid:            input.Bid,
				Ask:            input.Ask,
				Midpoint:       midpoint,
				Spread:         spread,
				RelativeSpread: relativeSpread,
				SpreadRatio:    spreadRatio,
				Divergence:     divergence,
				Reading:        reading,
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

/*
Ticker is the executable-touch market entity. It holds no state and no logic of
its own: its entire behavior is one nomagique pipeline over the measurement itself —
every stage writes its facts into the measurement where it computes them, and the
workload's register owns the measurement's lifetime.
*/
type Ticker struct {
	*runtime.System
	pipeline core.Primitive
	ID       int
}

func NewTicker(ctx context.Context) *Ticker {
	ticker := &Ticker{
		pipeline: nomagique.NewNumber(newTickerPipeline()),
	}

	ticker.System = runtime.NewSystem(ctx, "pumpdump:ticker", ticker)
	return ticker
}

/*
Step supplies the arriving measurement to the pipeline and returns it: the
measurement is the pipeline's state, enriched in place.
*/
func (ticker *Ticker) Step(m *data.Measurement[float64]) *data.Measurement[float64] {
	if ticker.Status() != runtime.READY {
		errnie.Warn(ticker.Name() + ": Step called before READY; dropping event")
		return m
	}

	if m == nil {
		return nil
	}

	if m.Err != nil {
		return m
	}

	source := m

	if len(m.Peers) > 0 {
		peer := m.FindPeer(func(candidate *data.Measurement[float64]) bool {
			if candidate.Label == "" {
				return false
			}

			bid := candidate.Metrics["best_bid"].Raw

			if bid == 0 {
				bid = candidate.Metrics["bid"].Raw
			}

			ask := candidate.Metrics["best_ask"].Raw

			if ask == 0 {
				ask = candidate.Metrics["ask"].Raw
			}

			return bid > 0 && ask > 0
		})

		if peer == nil {
			return m
		}

		source = peer
	}

	bid := source.Metrics["best_bid"].Raw

	if bid == 0 {
		bid = source.Metrics["bid"].Raw
	}

	ask := source.Metrics["best_ask"].Raw

	if ask == 0 {
		ask = source.Metrics["ask"].Raw
	}

	if bid <= 0 || ask <= 0 {
		return m
	}

	if bid >= ask {
		m.Err = fmt.Errorf("pumpdump: crossed touch (%f >= %f)", bid, ask)
		return m
	}

	if m.Metadata == nil {
		m.Metadata = make(map[string]string)
	}

	pipeInput := tickerInput{Bid: bid, Ask: ask}

	for out := range ticker.pipeline.Next(transport.NewOne(unsafe.Pointer(&pipeInput)).Next(nil)) {
		res := (*tickerResult)(out)

		m.Metrics["best_bid"] = m.Metrics["best_bid"].Write(res.Bid)
		m.Metrics["best_ask"] = m.Metrics["best_ask"].Write(res.Ask)
		m.Metrics["midpoint"] = m.Metrics["midpoint"].Write(res.Midpoint)
		m.Metrics["spread"] = m.Metrics["spread"].Write(res.Spread)
		m.Metrics["relative_spread"] = m.Metrics["relative_spread"].Write(res.RelativeSpread)
		m.Metrics["relative_spread_baseline"] = m.Metrics["relative_spread_baseline"].Write(res.Reading.Baseline)
		m.Metrics["spread_ratio"] = m.Metrics["spread_ratio"].Write(res.SpreadRatio)

		m.Metadata[data.MetadataSupport] = strconv.FormatFloat(res.Reading.Count, 'f', -1, 64)

		if res.Reading.HasPrior {
			m.Metrics["spread_divergence"] = m.Metrics["spread_divergence"].Write(res.Divergence)
			m.Metrics["spread_zscore"] = m.Metrics["spread_zscore"].Write(res.Reading.ZScore)
			m.Metadata[data.MetadataDivergence] = strconv.FormatFloat(res.Divergence, 'f', -1, 64)

			if res.Reading.VarianceDefined {
				m.Metadata[data.MetadataNoiseVariance] = strconv.FormatFloat(res.Reading.Variance, 'f', -1, 64)
			}
		}
	}

	m.Label = source.Label
	m.At = source.At
	m.Finalize()
	return m
}

/*
Register returns the measurement declaring this entity's full metric schema.
Values are empty; the workload uses this at startup to allocate the metric
schema before feeding streaming records.
*/
func (ticker *Ticker) Register() *data.Measurement[float64] {
	m := data.NewMeasurement[float64]("pumpdump:ticker", map[string]data.Metric[float64]{
		"best_bid":                 data.NewMetric[float64]("best_bid", data.UnitRate, data.TimescaleInstantaneous, 0, 1),
		"best_ask":                 data.NewMetric[float64]("best_ask", data.UnitRate, data.TimescaleInstantaneous, 0, 1),
		"midpoint":                 data.NewMetric[float64]("midpoint", data.UnitRate, data.TimescaleInstantaneous, 0, 1),
		"spread":                   data.NewMetric[float64]("spread", data.UnitRate, data.TimescaleInstantaneous, 0, 1),
		"relative_spread":          data.NewMetric[float64]("relative_spread", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1),
		"relative_spread_baseline": data.NewMetric[float64]("relative_spread_baseline", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1),
		"spread_ratio":             data.NewMetric[float64]("spread_ratio", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1),
		"spread_divergence":        data.NewMetric[float64]("spread_divergence", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1),
		"spread_zscore":            data.NewMetric[float64]("spread_zscore", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1),
	})
	m.Metadata["peer-interest"] = "*"
	return m
}
