package manifold

import (
	"context"
	"encoding/json"
	"io"
	"sync"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/datura/transport"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/logic"
	feed "github.com/theapemachine/symm/signal"
	"github.com/theapemachine/symm/signal/compute"
)

const manifoldBatchCapacity = 8192

/*
Signal classifies the 3D manifold state for one symbol.

PressureGradNorm captures cross-axis basis and beta dislocations.
CoherenceMag2 captures systemic herding / superfluid collapse.
GuidanceSpeed is the pilot-wave trend velocity from aligned Ψ.
ViscosityProxy inverts divergence — laminar when large, turbulent when small.
*/
type Signal struct {
	ctx         context.Context
	cancel      context.CancelFunc
	err         error
	pool        *qpool.Q[any]
	subscribers *sync.Map
	algo        io.ReadWriter
	tree        *dmt.Tree
	field       *Field
	serial      *compute.SerialPool
}

/*
NewSignal composes the manifold-state pipeline and subscribes to market channels.
*/
func NewSignal(
	ctx context.Context,
	pool *qpool.Q[any],
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

	manifoldstate := algorithm.NewManifoldstate()

	return &Signal{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		subscribers: &sync.Map{},
		tree:        dmt.NewTree(""),
		field:       field,
		serial:      serial,
		algo: nomagique.Number(
			manifoldstate,
			probability.NewClassifier(
				manifoldstate.HerdReading(),
				manifoldstate.ShockReading(),
				manifoldstate.DriftReading(),
				manifoldstate.NoiseReading(),
			),
		),
	}
}

func (signal *Signal) Update(artifact *datura.Artifact) error {
	switch datura.Peek[string](artifact, "role") {
	case "book", "trade", "ticker":
	case "measurement":
		if artifact != nil {
			signal.Measure(*artifact)
		}
	}

	return nil
}

func (signal *Signal) Measure(query datura.Artifact) *datura.Artifact {
	scope, _ := query.Scope()

	signal.publishFeatures(scope)

	var measurement *datura.Artifact

	prefix := "features/" + scope

	for inbound := range signal.tree.Seek([]byte(prefix)) {
		processed := datura.Acquire("manifold", datura.APPJSON)

		if processed == nil {
			continue
		}

		payload, payloadOK := inbound.PayloadQuiet()

		if !payloadOK {
			processed.Release()
			continue
		}

		if processed.WithPayload(payload) == nil {
			processed.Release()
			continue
		}

		if flipErr := transport.NewFlipFlop(processed, signal.algo); flipErr != nil {
			_ = processed.WithError(flipErr)
		}

		if datura.Peek[int](processed, "classifier.category") <= 0 {
			processed.Release()
			continue
		}

		if datura.Peek[float64](processed, "classifier.confidence") <= 0 {
			processed.Release()
			continue
		}

		processed.WithRole("measurement")
		processed.WithScope(scope)

		measurement = processed
	}

	if measurement != nil {
		feed.InsertMeasurement(signal.tree, measurement)
	}

	return measurement
}

func (signal *Signal) publishFeatures(scope string) {
	artifact := signal.featureArtifact(scope)

	if artifact == nil || signal.tree == nil {
		return
	}

	feed.InsertTreeArtifact(signal.tree, artifact)
	artifact.Release()
}

func (signal *Signal) featureArtifact(scope string) *datura.Artifact {
	if signal == nil || signal.field == nil {
		return nil
	}

	reading, price, _, ok := signal.field.Reading(scope)

	if !ok || !reading.IsFinite() {
		return nil
	}

	samples := []float64{
		reading.PressureGradNorm,
		reading.CoherenceMag2,
		reading.GuidanceSpeed,
		reading.ViscosityProxy,
		price,
	}

	payload, err := json.Marshal(samples)

	if err != nil {
		return nil
	}

	artifact := datura.Acquire("manifold-features", datura.Artifact_Type_json)
	artifact.WithRole("features")
	artifact.WithScope(scope)
	artifact.WithPayload(payload)

	return artifact
}

func manifoldCategory(categoryIndex int) logic.CategoryType {
	switch categoryIndex {
	case 1:
		return logic.CategorySystemicHerd
	case 2:
		return logic.CategoryLiquidityShock
	case 3:
		return logic.CategorySynchronizedDrift
	case 4:
		return logic.CategoryStochasticNoise
	default:
		return logic.CategoryTypeNone
	}
}

/*
FieldSnapshot builds the manifold dashboard payload from the last integrated field.
*/
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
