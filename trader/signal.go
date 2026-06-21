package trader

import (
	"context"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/signal/causal"
	"github.com/theapemachine/symm/signal/correlation"
	"github.com/theapemachine/symm/signal/cvd"
	"github.com/theapemachine/symm/signal/depthflow"
	"github.com/theapemachine/symm/signal/exhaust"
	"github.com/theapemachine/symm/signal/fluid"
	"github.com/theapemachine/symm/signal/hawkes"
	"github.com/theapemachine/symm/signal/leadlag"
	"github.com/theapemachine/symm/signal/liquidity"
	"github.com/theapemachine/symm/signal/manifold"
	"github.com/theapemachine/symm/signal/pumpdump"
	"github.com/theapemachine/symm/signal/sentiment"
	"github.com/theapemachine/symm/signal/toxicity"
)

/*
Signal runs every spectrum signal against the ingest roles it declares.
*/
type Signal struct {
	ctx      context.Context
	cancel   context.CancelFunc
	pool     *qpool.Q[any]
	tree     *dmt.Tree
	bindings []signalBinding
}

type signalBinding struct {
	signal market.Signal
	source logic.SourceType
}

func NewSignal(
	ctx context.Context,
	pool *qpool.Q[any],
	tree *dmt.Tree,
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	boundSignals := []market.Signal{
		causal.NewSignal(ctx, pool, tree),
		correlation.NewSignal(ctx, pool, tree),
		cvd.NewSignal(ctx, pool, tree),
		depthflow.NewSignal(ctx, pool, tree),
		exhaust.NewSignal(ctx, pool, tree),
		fluid.NewSignal(ctx, pool, tree),
		hawkes.NewSignal(ctx, pool, tree),
		leadlag.NewSignal(ctx, pool, tree),
		liquidity.NewSignal(ctx, pool, tree),
		manifold.NewSignal(ctx, pool, tree),
		pumpdump.NewSignal(ctx, pool, tree),
		sentiment.NewSignal(ctx, pool, tree),
		toxicity.NewSignal(ctx, pool, tree),
	}

	sources := []logic.SourceType{
		logic.SourceCausal,
		logic.SourceCorrelation,
		logic.SourceCVD,
		logic.SourceDepthFlow,
		logic.SourceExhaustion,
		logic.SourceFluid,
		logic.SourceHawkes,
		logic.SourceLeadLag,
		logic.SourceLiquidity,
		logic.SourceManifold,
		logic.SourcePumpDump,
		logic.SourceSentiment,
		logic.SourceToxicity,
	}

	bindings := make([]signalBinding, 0, len(boundSignals))

	for index, sig := range boundSignals {
		if sig == nil {
			continue
		}

		bindings = append(bindings, signalBinding{
			signal: sig,
			source: sources[index],
		})
	}

	return &Signal{
		ctx:      ctx,
		cancel:   cancel,
		pool:     pool,
		tree:     tree,
		bindings: bindings,
	}
}

func (signal *Signal) Measure() []*datura.Artifact {
	measurements := make([]*datura.Artifact, 0, len(signal.bindings))

	for _, binding := range signal.bindings {
		measurement := signal.measureBinding(binding)

		if measurement == nil {
			continue
		}

		tagMeasurementForUI(measurement, binding.source)
		measurements = append(measurements, measurement)
	}

	return measurements
}

func (signal *Signal) measureBinding(binding signalBinding) *datura.Artifact {
	roles := binding.signal.IngestRoles()

	if len(roles) == 0 {
		return nil
	}

	var result *datura.Artifact

	for _, role := range roles {
		for stored := range signal.tree.Seek([]byte(role + "/update")) {
			measurement := binding.signal.Measure(stored)

			if measurement == nil {
				stored.Release()

				continue
			}

			if measurement != stored {
				stored.Release()
			}

			if result != nil && result != measurement {
				result.Release()
			}

			result = measurement
		}
	}

	return result
}

func tagMeasurementForUI(measurement *datura.Artifact, source logic.SourceType) {
	if measurement == nil || source == logic.SourceNone {
		return
	}

	errnie.Error(measurement.SetOrigin(string(source)))
	measurement.WithRole("measurement")

	scope, _ := measurement.Scope()

	if scope != "update" && scope != "snapshot" && scope != "" {
		return
	}

	symbol := datura.Peek[string](measurement, "data", 0, "symbol")

	if symbol == "" {
		return
	}

	measurement.WithScope(symbol)
}

func latestIngest(tree *dmt.Tree, role string) *datura.Artifact {
	var latest *datura.Artifact

	for stored := range tree.Seek([]byte(role + "/update")) {
		if latest != nil {
			latest.Release()
		}

		latest = stored
	}

	return latest
}

func (signal *Signal) Close() error {
	signal.cancel()

	for _, binding := range signal.bindings {
		binding.signal.Close()
	}

	return nil
}
