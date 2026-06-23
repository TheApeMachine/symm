package trader

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"time"

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

type wiredSignal struct {
	measurer market.Signal
	origin   logic.SourceType
}

/*
Signal runs every spectrum signal against the ingest roles it declares.
Each signal advances its own tree cursor so Measure only replays new frames.
*/
type Signal struct {
	ctx            context.Context
	cancel         context.CancelFunc
	pool           *qpool.Q[any]
	tree           *dmt.Tree
	signals        []wiredSignal
	measureCursors sync.Map
}

func NewSignal(
	ctx context.Context,
	pool *qpool.Q[any],
	tree *dmt.Tree,
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	return &Signal{
		ctx:    ctx,
		cancel: cancel,
		pool:   pool,
		tree:   tree,
		signals: []wiredSignal{
			{causal.NewSignal(ctx, pool, tree), logic.SourceCausal},
			{correlation.NewSignal(ctx, pool, tree), logic.SourceCorrelation},
			{cvd.NewSignal(ctx, pool, tree), logic.SourceCVD},
			{depthflow.NewSignal(ctx, pool, tree), logic.SourceDepthFlow},
			{exhaust.NewSignal(ctx, pool, tree), logic.SourceExhaustion},
			{fluid.NewSignal(ctx, pool, tree), logic.SourceFluid},
			{hawkes.NewSignal(ctx, pool, tree), logic.SourceHawkes},
			{leadlag.NewSignal(ctx, pool, tree), logic.SourceLeadLag},
			{liquidity.NewSignal(ctx, pool, tree), logic.SourceLiquidity},
			{manifold.NewSignal(ctx, pool, tree), logic.SourceManifold},
			{pumpdump.NewSignal(ctx, pool, tree), logic.SourcePumpDump},
			{sentiment.NewSignal(ctx, pool, tree), logic.SourceSentiment},
			{toxicity.NewSignal(ctx, pool, tree), logic.SourceToxicity},
		},
	}
}

func measureCursorKey(measurer market.Signal, role string) string {
	return fmt.Sprintf("%p:%s", measurer, role)
}

func (signal *Signal) Measure() []*datura.Artifact {
	measurements := make([]*datura.Artifact, 0, len(signal.signals))

	for _, wired := range signal.signals {
		for _, role := range wired.measurer.IngestRoles() {
			measurements = append(
				measurements,
				signal.measureRole(wired, role)...,
			)
		}
	}

	return measurements
}

/*
FieldSnapshots returns live fluid and manifold dashboard payloads when available.
*/
func (signal *Signal) FieldSnapshots(eventAt time.Time) []map[string]any {
	if signal == nil || eventAt.IsZero() {
		return nil
	}

	snapshots := make([]map[string]any, 0, 2)

	for _, wired := range signal.signals {
		snapshotter, ok := wired.measurer.(fieldSnapshotter)

		if !ok {
			continue
		}

		payload, err := snapshotter.FieldSnapshot(eventAt)

		if err != nil || len(payload) == 0 {
			continue
		}

		snapshots = append(snapshots, payload)
	}

	return snapshots
}

func (signal *Signal) measureRole(
	wired wiredSignal,
	role string,
) []*datura.Artifact {
	if signal == nil || signal.tree == nil || wired.measurer == nil {
		return nil
	}

	prefix := []byte(role + "/")
	cursorKey := measureCursorKey(wired.measurer, role)
	lastKey, _ := signal.measureCursors.Load(cursorKey)

	lastPrefix, _ := lastKey.([]byte)

	measurements := make([]*datura.Artifact, 0)
	advanced := lastPrefix
	var deferredProbe *datura.Artifact

	signal.tree.WalkPrefix(prefix, func(key, value []byte) bool {
		if len(lastPrefix) > 0 && bytes.Compare(key, lastPrefix) <= 0 {
			return true
		}

		inbound := datura.Acquire("trader-measure", datura.APPJSON)

		if _, err := inbound.Unpack(value); err != nil {
			inbound.Release()

			errnie.Error(errnie.Err(
				errnie.Validation,
				"trader: failed to unpack ingest artifact",
				err,
			))

			return true
		}

		measurement := wired.measurer.Measure(inbound)

		if measurement == nil {
			if probe := calibrationProbe(inbound, wired.origin); probe != nil {
				if deferredProbe != nil {
					deferredProbe.Release()
				}

				deferredProbe = probe
			} else if warmup := warmupProbe(inbound, wired.origin); warmup != nil {
				if deferredProbe != nil {
					deferredProbe.Release()
				}

				deferredProbe = warmup
			}

			inbound.Release()
			advanced = append([]byte(nil), key...)

			return true
		}

		inbound.Release()

		measurement.WithRole("measurement")

		if err := measurement.SetOrigin(string(wired.origin)); err != nil {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"trader: measurement origin failed",
				err,
			))

			return true
		}

		measurement.WithDestination("ui")

		symbol := datura.Peek[string](measurement, "data", 0, "symbol")

		if symbol != "" {
			measurement.WithScope(symbol)
		}

		if datura.Peek[float64](measurement, "output", "confidence") > 0 {
			measurement.Merge("calibrated", true)
		}

		measurements = append(measurements, measurement)

		advanced = append([]byte(nil), key...)

		return true
	})

	if deferredProbe != nil {
		measurements = append(measurements, deferredProbe)
	}

	if len(advanced) > 0 && !bytes.Equal(advanced, lastPrefix) {
		signal.measureCursors.Store(cursorKey, advanced)
	}

	return measurements
}

func warmupProbe(
	inbound *datura.Artifact,
	origin logic.SourceType,
) *datura.Artifact {
	if inbound == nil || origin == "" {
		return nil
	}

	probe := datura.Acquire("trader-warmup", datura.APPJSON)
	probe.Merge("calibrating", true)

	symbol := datura.Peek[string](inbound, "data", 0, "symbol")

	if symbol != "" {
		probe.Merge("symbol", symbol)
	}

	samples := datura.Peek[float64](inbound, "samples")

	if samples > 0 {
		probe.Merge("samples", samples)
	}

	minSamples := datura.Peek[float64](inbound, "min_samples")

	if minSamples > 0 {
		probe.Merge("min_samples", minSamples)
	}

	probe.WithRole("measurement")

	if err := probe.SetOrigin(string(origin)); err != nil {
		probe.Release()

		errnie.Error(errnie.Err(
			errnie.Validation,
			"trader: warmup probe origin failed",
			err,
		))

		return nil
	}

	if symbol != "" {
		probe.WithScope(symbol)
	}

	probe.WithDestination("ui")

	return probe
}

func calibrationProbe(
	inbound *datura.Artifact,
	origin logic.SourceType,
) *datura.Artifact {
	if inbound == nil || origin == "" {
		return nil
	}

	confidence := datura.Peek[float64](inbound, "output", "confidence")
	samples := datura.Peek[float64](inbound, "samples")
	minSamples := datura.Peek[float64](inbound, "min_samples")

	if confidence <= 0 && samples <= 0 && minSamples <= 0 {
		return nil
	}

	probe := datura.Acquire("trader-calibration", datura.APPJSON)
	probe.Merge("calibrating", true)

	if confidence > 0 {
		probe.MergeOutput("confidence", confidence)
	}

	if samples > 0 {
		probe.Merge("samples", samples)
	}

	if minSamples > 0 {
		probe.Merge("min_samples", minSamples)
	}

	symbol := datura.Peek[string](inbound, "data", 0, "symbol")

	if symbol != "" {
		probe.Merge("symbol", symbol)
	}

	probe.WithRole("measurement")

	if err := probe.SetOrigin(string(origin)); err != nil {
		probe.Release()

		errnie.Error(errnie.Err(
			errnie.Validation,
			"trader: calibration probe origin failed",
			err,
		))

		return nil
	}

	if symbol != "" {
		probe.WithScope(symbol)
	}

	probe.WithDestination("ui")

	return probe
}

func (signal *Signal) Close() error {
	signal.cancel()

	for _, wired := range signal.signals {
		wired.measurer.Close()
	}

	return nil
}
