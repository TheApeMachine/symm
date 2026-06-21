package manifold

import (
	"context"
	"encoding/binary"
	"io"
	"math"
	"sync"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/datura/transport"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/qpool"
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
	algo        io.ReadWriteCloser
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

	manifoldstate := equation.NewManifoldstate()

	return &Signal{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		subscribers: &sync.Map{},
		tree:        tree,
		field:       field,
		serial:      serial,
		algo: nomagique.Number(
			manifoldstate,
			probability.NewClassifier(
				datura.Acquire("manifold-classifier", datura.APPJSON).WithAttributes(datura.Map[any]{
					"inputs": []string{"herdScore", "shockScore", "driftScore", "noiseScore"},
				}),
			),
		),
	}
}

func (signal *Signal) Measure(datapoint *datura.Artifact) *datura.Artifact {
	if signal == nil || datapoint == nil || signal.algo == nil {
		return nil
	}

	scope, _ := datapoint.Scope()

	if scope == "" {
		return nil
	}

	featurePrefix := "features/" + scope
	featurePayload := []byte(nil)

	for inbound := range signal.tree.Seek([]byte(featurePrefix)) {
		if inbound == nil || !inbound.HasEncryptedPayload() {
			continue
		}

		payload := inbound.DecryptPayload()

		if len(payload) == 0 {
			continue
		}

		featurePayload = payload
	}

	featureWire := manifoldFeatureWire(featurePayload)

	if len(featureWire) == 0 {
		return nil
	}

	stored := datura.Acquire("manifold-features", datura.APPJSON)
	stored.WithRole("features")
	stored.WithScope(scope)
	stored.WithPayload(featureWire)

	if errnie.Error(transport.NewFlipFlop(
		stored, signal.algo,
	)) != nil {
		stored.Release()

		return nil
	}

	stored.WithRole("measurement")
	stored.WithScope(scope)
	stored.Merge("scope", scope)

	category := datura.Peek[float64](stored, "output", "category")
	confidence := datura.Peek[float64](stored, "output", "confidence")
	stored.Merge("classifier.category", category)
	stored.Merge("classifier.confidence", confidence)

	if signal.tree != nil {
		if wire := stored.Pack(); len(wire) > 0 {
			signal.tree.Insert(stored.Prefix(), wire)
		}
	}

	return stored
}

/*
FieldSnapshot builds the manifold dashboard payload from the last integrated field.
*/
func manifoldFeatureWire(raw []byte) []byte {
	if len(raw) == 0 {
		return nil
	}

	if raw[0] == '{' {
		return raw
	}

	fieldCount := len(raw) / 8
	samples := make([]float64, 0, fieldCount)

	for offset := 0; offset+8 <= len(raw); offset += 8 {
		bits := binary.BigEndian.Uint64(raw[offset : offset+8])
		sample := math.Float64frombits(bits)

		if math.IsNaN(sample) || math.IsInf(sample, 0) {
			continue
		}

		samples = append(samples, sample)
	}

	if len(samples) == 0 {
		return nil
	}

	return equation.MarshalFeaturesPayload(samples)
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
