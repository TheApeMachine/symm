package manifold

import (
	"context"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/signal/compute"
	"github.com/theapemachine/symm/signal/dist"
)

const manifoldBatchCapacity = 8192

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
	ctx    context.Context
	cancel context.CancelFunc
	err    error
	tree   *dmt.Tree
	field  *Field
	serial *compute.SerialPool
}

func NewSignal(
	ctx context.Context,
	pool *qpool.Q[any],
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
		manifoldBatchCapacity,
		100*time.Millisecond,
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
	return []string{"ticker"}
}

func (signal *Signal) Measure(datapoint *datura.Artifact) *datura.Artifact {
	if signal == nil || datapoint == nil {
		return nil
	}

	scope, _ := datapoint.Scope()

	if scope == "" {
		return nil
	}

	pressure, coherence, guidance, viscosity := signal.resolveFeatures(
		scope,
		time.Unix(0, datapoint.Timestamp()),
	)

	if pressure == 0 && coherence == 0 && guidance == 0 && viscosity == 0 {
		return nil
	}

	herd := coherence * guidance
	shock := pressure
	drift := guidance / (1 + viscosity)
	noise := viscosity * (1 - coherence)

	shares := []dist.Share{
		{Key: "herdScore", Category: logic.CategorySystemicHerd, Mass: herd},
		{Key: "shockScore", Category: logic.CategoryLiquidityShock, Mass: shock},
		{Key: "driftScore", Category: logic.CategorySynchronizedDrift, Mass: drift},
		{Key: "noiseScore", Category: logic.CategoryStochasticNoise, Mass: noise},
	}

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

func manifoldFeatures(raw []byte) (pressure, coherence, guidance, viscosity float64) {
	pressure, coherence, guidance, viscosity, ok := decodeFeaturePayload(raw)

	if !ok {
		return 0, 0, 0, 0
	}

	return pressure, coherence, guidance, viscosity
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

	if !signal.field.hasPublishableSnapshot() {
		return nil, nil
	}

	return signal.field.snapshotPayload(eventAt)
}

func (signal *Signal) Error() error {
	return signal.err
}

func (signal *Signal) Close() (err error) {
	err = signal.err
	signal.cancel()

	if signal.serial != nil {
		signal.serial.Close()
	}

	if signal.field != nil {
		signal.field.Close()
	}

	return err
}
