package derivatives

import (
	"context"
	"sync"
	"unsafe"

	"github.com/theapemachine/errnie"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	nmderivatives "github.com/theapemachine/symm/nomagique/derivatives"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Ticker is the derivative/reference state measuring instrument. It holds no
state and no logic of its own: its entire behavior is one nomagique pipeline
over the measurement itself — every stage writes its facts into the
measurement where it computes them, and the workload's register owns the
measurement's lifetime.
*/
type Ticker struct {
	*runtime.System
	pipelines sync.Map
	ID        int
}

func NewTicker(ctx context.Context) *Ticker {
	ticker := &Ticker{}

	ticker.System = runtime.NewSystem(ctx, "derivatives:ticker", ticker)
	return ticker
}

/*
Step supplies the arriving measurement to the pipeline and returns it: the
measurement is the pipeline's state, enriched in place.
*/

func (ticker *Ticker) pipelineFor(symbol string) core.Primitive {
	if existing, ok := ticker.pipelines.Load(symbol); ok {
		return existing.(core.Primitive)
	}

	pipeline := nomagique.NewNumber(
		nmderivatives.NewGate(),
		nmderivatives.NewBasis(),
		transport.NewFan(
			data.NewAdapter(
				adaptive.NewBaseline(adaptive.NewWindow()),
				func(m *data.Measurement[float64]) float64 {
					if v, ok := m.LookupMetric("open_interest_growth"); ok {
						return v.Raw
					}
					return 0
				},
				func(m *data.Measurement[float64], out adaptive.BaselineReading) {
					if out.HasPrior {
						m.WriteStandardized("open_interest_growth_zscore", out.ZScore)
					}
				},
			),
			data.NewAdapter(
				adaptive.NewBaseline(adaptive.NewWindow()),
				func(m *data.Measurement[float64]) float64 {
					if v, ok := m.LookupMetric("return_gap"); ok {
						return v.Raw
					}
					return 0
				},
				func(m *data.Measurement[float64], out adaptive.BaselineReading) {
					if out.HasPrior {
						m.WriteStandardized("return_gap_zscore", out.ZScore)
					}
				},
			),
			data.NewAdapter(
				temporal.NewVelocity(),
				func(m *data.Measurement[float64]) temporal.Observation {
					if rate, ok := m.LookupMetric("open_interest_growth"); ok {
						return temporal.Observation{Value: rate.Raw, At: m.At.UnixNano()}
					}
					return temporal.Observation{Value: 0, At: m.At.UnixNano()}
				},
				func(m *data.Measurement[float64], out temporal.VelocityReading) {
					if out.Defined {
						m.WriteMetric("open_interest_growth_velocity", out.Rate)
					}
				},
			),
		),
		data.NewFinalizer[float64](),
	)

	actual, _ := ticker.pipelines.LoadOrStore(symbol, pipeline)
	return actual.(core.Primitive)
}

func (ticker *Ticker) Step(input *runtime.StageInput, output *data.Measurement[float64]) *data.Measurement[float64] {
	if ticker.Status() != runtime.READY {
		errnie.Warn(ticker.Name() + ": Step called before READY; dropping event")
		return nil
	}

	if input == nil || input.Ingress() == nil || input.Symbol() == "" {
		return nil
	}

	lastMetric, hasLast := input.IngressMetric("last")
	indexMetric, hasIndex := input.IngressMetric("index_price")
	markMetric, hasMark := input.IngressMetric("mark_price")
	oiMetric, hasOI := input.IngressMetric("open_interest")

	if !hasLast || !hasIndex || !hasMark || !hasOI {
		return nil
	}

	output.SetMetric("last", lastMetric)
	output.SetMetric("index_price", indexMetric)
	output.SetMetric("mark_price", markMetric)
	output.SetMetric("open_interest", oiMetric)

	if channel, hasCh := input.IngressProvenance("channel"); hasCh {
		output.SetProvenance("channel", channel)
	}

	data.StampInterval(output, input.At(), input.From())

	res := data.Read[*data.Measurement[float64]](ticker.pipelineFor(output.Label).Next(
		transport.NewOne(unsafe.Pointer(&output)).Next(nil),
	))

	if res == nil {
		return output
	}

	return res
}
