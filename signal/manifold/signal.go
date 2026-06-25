package manifold

import (
	"context"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/signal/compute"
	"github.com/theapemachine/symm/signal/dist"
)

/*
Manifold is the pilot-wave perspective on systemic field dynamics.

# Summary of Manifold Categories

| Category          | Dominant Score | Primary Input        | Market "Feel"           |
|:------------------|:---------------|:---------------------|:------------------------|
| Systemic Herd     | herdScore      | Coherence × Guidance | Superfluid Collapse     |
| Liquidity Shock   | shockScore     | PressureGradNorm     | Field Rupture           |
| Pilot-Wave Drift  | driftScore     | Guidance / Viscosity | Synchronized Drift      |
| Stochastic Noise  | noiseScore     | Viscosity × (1−Coh.) | Decoupled Noise         |
*/
/*
Signal classifies the 3D manifold state for one symbol from Field features.
See the struct comment block for category semantics.
*/
type Signal struct {
	ctx              context.Context
	cancel           context.CancelFunc
	err              error
	tree             *dmt.Tree
	field            *Field
	serial           *compute.SerialPool
	lastHydrateStamp int64
	featureCache     featureCacheEntry
}

type featureCacheEntry struct {
	scope      string
	eventStamp int64
	pressure   float64
	coherence  float64
	guidance   float64
	viscosity  float64
	ok         bool
}

func NewSignal(
	ctx context.Context,
	tree *dmt.Tree,
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	field := errnie.Does(func() (*Field, error) {
		return NewField()
	}).Or(func(err error) {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"manifold: failed to create field",
			err,
		))
	}).Value()

	serial := compute.NewSerialPool(
		ctx,
		ManifoldBatchCapacity(),
		ManifoldFlushInterval(),
	)

	if field != nil {
		field.bindSerial(serial)
	}

	return &Signal{
		ctx:    ctx,
		cancel: cancel,
		tree:   tree,
		field:  field,
		serial: serial,
	}
}

func (signal *Signal) IngestRoles() []string {
	return []string{"book", "trade", "ticker"}
}

func (signal *Signal) Measure(
	datapoint *datura.Artifact,
	crossSection *market.CrossSection,
) *datura.Artifact {
	if signal == nil || datapoint == nil {
		return nil
	}

	channel := datura.Peek[string](datapoint, "channel")

	switch channel {
	case "book":
		signal.observeBookArtifact(datapoint)

		return nil
	case "trade":
		signal.observeTradeArtifact(datapoint)

		return nil
	case "ticker":
		signal.observeTickerArtifact(datapoint)
	case "":
		// ponytail: measurement tick without ingest payload
	default:
		return nil
	}

	scope := datura.Peek[string](datapoint, "data", 0, "symbol")

	if scope == "" {
		scope, _ = datapoint.Scope()
	}

	if scope == "" {
		return nil
	}

	pressure, coherence, guidance, viscosity, ok := signal.resolveFeatures(
		scope,
		time.Unix(0, datapoint.Timestamp()),
	)

	if !ok {
		return nil
	}

	shares := signal.classify(pressure, coherence, guidance, viscosity)

	measurement := datura.Acquire("manifold", datura.APPJSON)
	measurement.WithRole("measurement")
	measurement.WithScope(scope)
	errnie.Error(measurement.SetOrigin(string(logic.SourceManifold)))
	measurement.SetTimestamp(datapoint.Timestamp())

	measurement.MergeOutput("pressureGradNorm", pressure)
	measurement.MergeOutput("coherenceMag2", coherence)
	measurement.MergeOutput("guidanceSpeed", guidance)
	measurement.MergeOutput("viscosityProxy", viscosity)
	confidence := dist.Write(measurement, shares)

	if confidence <= 0 {
		measurement.Release()

		return nil
	}

	measurement.Merge("classifier.category", manifoldClassifierIndex(shares))
	measurement.Merge("classifier.confidence", confidence)

	measurement.Merge("scope", scope)

	return measurement
}

func (signal *Signal) classify(
	pressure, coherence, guidance, viscosity float64,
) []dist.Share {
	herd := coherence * guidance
	shock := pressure
	drift := guidance / (1 + viscosity)
	noise := viscosity * (1 - coherence)

	return []dist.Share{
		{Key: "herdScore", Category: logic.CategorySystemicHerd, Mass: herd},
		{Key: "shockScore", Category: logic.CategoryLiquidityShock, Mass: shock},
		{Key: "driftScore", Category: logic.CategorySynchronizedDrift, Mass: drift},
		{Key: "noiseScore", Category: logic.CategoryStochasticNoise, Mass: noise},
	}
}

func manifoldClassifierIndex(shares []dist.Share) int {
	order := []logic.CategoryType{
		logic.CategorySystemicHerd,
		logic.CategoryLiquidityShock,
		logic.CategorySynchronizedDrift,
		logic.CategoryStochasticNoise,
	}

	bestIndex := 0
	bestMass := shares[0].Mass

	for index := range shares {
		if shares[index].Mass > bestMass {
			bestMass = shares[index].Mass
			bestIndex = index
		}
	}

	for index, category := range order {
		if shares[bestIndex].Category == category {
			return index + 1
		}
	}

	return 0
}

func (signal *Signal) FieldSnapshot(eventAt time.Time) (map[string]any, error) {
	if signal == nil || signal.field == nil {
		return nil, nil
	}

	type fieldSnapshotResult struct {
		payload map[string]any
		err     error
	}

	result := compute.Run(signal.serial, func() fieldSnapshotResult {
		if !signal.field.hasPublishableSnapshot() {
			return fieldSnapshotResult{}
		}

		payload, snapshotErr := signal.field.snapshotPayload(eventAt)

		return fieldSnapshotResult{payload: payload, err: snapshotErr}
	})

	return result.payload, result.err
}

func (signal *Signal) Error() error {
	return signal.err
}

func (signal *Signal) Close() (err error) {
	err = signal.err
	signal.cancel()

	if signal.field != nil {
		compute.Run(signal.serial, func() struct{} {
			signal.field.closeSolver()

			return struct{}{}
		})
	}

	if signal.serial != nil {
		signal.serial.Close()
	}

	return err
}
